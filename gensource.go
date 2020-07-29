// Copyright 2020 Alexey Krivonogov. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"eonza/lib"
	"fmt"
	"hash/crc64"
	"reflect"
	"strings"

	es "eonza/script"

	"gopkg.in/yaml.v2"
)

type Source struct {
	Linked      map[string]bool
	Strings     []string
	CRCTable    *crc64.Table
	HashStrings map[uint64]int
	Header      *es.Header
	Counter     int
	Funcs       string
}

type Param struct {
	Value string
	Type  string
	Name  string
}

func (src *Source) Tree(tree []scriptTree) (string, error) {
	var (
		body, tmp string
		err       error
	)
	for _, child := range tree {
		if child.Disable {
			continue
		}
		if tmp, err = src.Script(child); err != nil {
			return ``, err
		}
		body += tmp
	}
	return body, nil
}

func (src *Source) FindStrConst(value string) string {
	var (
		id int
		ok bool
	)
	crc := crc64.Checksum([]byte(value), src.CRCTable)
	if id, ok = src.HashStrings[crc]; !ok {
		id = len(src.Strings)
		src.HashStrings[crc] = id
		src.Strings = append(src.Strings, value)
	}
	return fmt.Sprintf(`STR%d`, id)
}

func (src *Source) ScriptValues(script *Script, node scriptTree) ([]Param, error) {
	values := make([]Param, 0, len(script.Params))
	errField := func(field string) error {
		glob := langRes[langsId[src.Header.Lang]]
		return fmt.Errorf(langRes[langsId[src.Header.Lang]][`errfield`],
			es.ReplaceVars(field, script.Langs[src.Header.Lang], &glob),
			es.ReplaceVars(script.Settings.Title, script.Langs[src.Header.Lang], &glob))
	}
	for _, par := range script.Params {
		var (
			ptype, value string
			isMacro      bool
		)
		val := node.Values[par.Name]
		if val != nil {
			value = strings.TrimSpace(fmt.Sprint(val))
		}
		switch par.Type {
		case PCheckbox:
			ptype = `bool`
			if value == `false` || value == `0` || len(value) == 0 {
				value = `false`
			} else {
				value = `true`
			}
		case PTextarea, PSingleText:
			ptype = `str`
			if len(value) == 0 {
				if par.Options.Required {
					return nil, errField(par.Title)
				}
				value = par.Options.Default
			}
			if script.Settings.Name != SourceCode {
				isMacro = strings.ContainsRune(value, es.VarChar)
				value = src.FindStrConst(value)
				if isMacro {
					value = fmt.Sprintf("Macro(%s)", value)
				}
			}
		case PSelect:
			if len(par.Options.Type) > 0 {
				ptype = par.Options.Type
			} else {
				ptype = `str`
				value = src.FindStrConst(value)
			}
		case PNumber:
			ptype = `int`
			if len(value) == 0 {
				if par.Options.Required {
					return nil, errField(par.Title)
				}
				value = par.Options.Default
			}
		case PList:
			ptype = `str`
			if reflect.TypeOf(val).Kind() == reflect.Slice && reflect.ValueOf(val).Len() > 0 {
				out, err := json.Marshal(val)
				if err != nil {
					return nil, err
				}
				value = src.FindStrConst(string(out))
			} else {
				if par.Options.Required {
					return nil, errField(par.Title)
				}
				value = src.FindStrConst(`[]`)
			}
		}
		values = append(values, Param{
			Value: value,
			Type:  ptype,
			Name:  par.Name,
		})
	}
	return values, nil
}

func (src *Source) Predefined(script *Script) (ret string, err error) {
	if len(script.Langs[LangDefCode]) > 0 {
		var data []byte
		predef := make(map[string]string)

		for name, value := range script.Langs[LangDefCode] {
			if !strings.HasPrefix(name, `_`) {
				predef[name] = value
			}
		}
		if src.Header.Lang != LangDefCode {
			for name, value := range script.Langs[src.Header.Lang] {
				if !strings.HasPrefix(name, `_`) {
					predef[name] = value
				}
			}
		}
		data, err = yaml.Marshal(predef)
		if err != nil {
			return
		}
		ret = `SetYamlVars(` + src.FindStrConst(string(data)) + ")\r\n"
	}
	return
}

func (src *Source) Script(node scriptTree) (string, error) {
	script := getScript(node.Name)
	if script == nil {
		return ``, fmt.Errorf(Lang(DefLang, `erropen`), node.Name)
	}
	idname := lib.IdName(script.Settings.Name)
	values, err := src.ScriptValues(script, node)
	if err != nil {
		return ``, err
	}
	var params []string
	if !src.Linked[idname] || script.Settings.Name == SourceCode {
		src.Linked[idname] = true

		tmp, err := src.Tree(node.Children)
		if err != nil {
			return ``, err
		}
		var (
			code, predef string
		)
		if predef, err = src.Predefined(script); err != nil {
			return ``, err
		}
		if script.Settings.Name == SourceCode {
			code = values[1].Value
		} else {
			code = script.Code
		}
		code = strings.ReplaceAll(code, `%body%`, tmp)
		if script.Settings.Name == SourceCode {
			if values[0].Value == `true` {
				src.Funcs += code + "\r\n"
				return ``, nil
			} else {
				idname = fmt.Sprintf("%s%d", idname, src.Counter)
				src.Counter++
			}
		}
		code = strings.TrimRight(code, "\r\n")
		var parNames string
		if script.Settings.Name != SourceCode {
			var vars []string
			for _, par := range values {
				params = append(params, fmt.Sprintf("%s %s", par.Type, par.Name))
				parNames += `,` + par.Name
				vars = append(vars, fmt.Sprintf(`"%s", %[1]s`, par.Name))
			}
			if len(script.Tree) > 0 {
				code += "\r\ninit(" + strings.Join(vars, `,`) + ")\r\n" + predef
				tmp, err = src.Tree(script.Tree)
				if err != nil {
					return ``, err
				}
				code += "\r\n" + tmp
				code += "\r\ndeinit()"
			}
		}
		var prefix, suffix, initcmd string
		if script.Settings.LogLevel < es.LOG_INHERIT {
			prefix = fmt.Sprintf("int prevLog = SetLogLevel(%d)\r\n", script.Settings.LogLevel)
			suffix = "\r\nSetLogLevel(prevLog)"
		}
		initcmd = fmt.Sprintf("initcmd(`%s`%s)\r\n", script.Settings.Name, parNames)
		/*		if len(script.Tree) > 0 || len(predef) > 0 {
				initcmd += "init()\r\n" + predef
				code += "\r\ndeinit()"
			}*/
		code = initcmd + code
		src.Funcs += fmt.Sprintf("func %s(%s) {\r\n", idname, strings.Join(params, `,`)) +
			prefix + code + suffix + "\r\n}\r\n"
	}
	params = params[:0]
	if script.Settings.Name != SourceCode {
		for _, par := range values {
			params = append(params, par.Value)
		}
	}
	return fmt.Sprintf("   %s(%s)\r\n", idname, strings.Join(params, `,`)), nil
}

func ValToStr(input string) string {
	var out string

	if strings.ContainsAny(input, "`%$") {
		out = strings.ReplaceAll(input, `\`, `\\`)
		out = `"` + strings.ReplaceAll(out, `"`, `\"`) + `"`
	} else {
		out = "`" + input + "`"
	}
	return out
}

func GenSource(script *Script, header *es.Header) (string, error) {
	var params string
	src := &Source{
		Linked:      make(map[string]bool),
		CRCTable:    crc64.MakeTable(crc64.ISO),
		HashStrings: make(map[uint64]int),
		Header:      header,
	}
	values, err := src.ScriptValues(script, scriptTree{})
	if err != nil {
		return ``, err
	}
	for _, par := range values {
		val := par.Value
		if par.Type == `str` {
			val = ValToStr(val)
		}
		params += fmt.Sprintf("%s %s = %s\r\n", par.Type, par.Name, val)
	}
	level := storage.Settings.LogLevel
	if script.Settings.LogLevel < es.LOG_INHERIT {
		level = script.Settings.LogLevel
	}
	params += fmt.Sprintf("SetLogLevel(%d)\r\ninit()\r\n", level)
	code := strings.TrimSpace(strings.ReplaceAll(script.Code, `%body%`, ``))
	if len(code) > 0 {
		code += "\r\n"
	}
	if predef, err := src.Predefined(script); err != nil {
		return ``, err
	} else {
		code = predef + code
	}
	body, err := src.Tree(script.Tree)
	if err != nil {
		return ``, err
	}
	var constStr string
	if len(src.Strings) > 0 {
		constStr = "const {\r\n"
		for i, val := range src.Strings {
			constStr += fmt.Sprintf("STR%d = %s\r\n", i, ValToStr(val))
		}
		constStr += "}\r\n"
	}
	constStr += `const IOTA { LOG_DISABLE
	LOG_ERROR LOG_WARN LOG_FORM LOG_INFO LOG_DEBUG }
`
	return fmt.Sprintf("%s%s\r\nrun {\r\n%s%s%s\r\ndeinit()}", constStr, src.Funcs, params,
		code, body), nil
}
