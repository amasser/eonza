// Copyright 2020 Alexey Krivonogov. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package script

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gentee/gentee"
)

const (
	LOG_DISABLE = iota
	LOG_ERROR
	LOG_WARN
	LOG_INFO
	LOG_DEBUG
	LOG_INHERIT

	VarChar   = '#'
	VarLength = 32
	VarDeep   = 16

	ErrVarLoop = `%s variable refers to itself`
	ErrVarDeep = `maximum depth reached`
)

type Data struct {
	LogLevel int64
	Vars     []map[string]string
	Mutex    sync.Mutex
	chLogout chan string
}

var (
	dataScript Data
	customLib  = []gentee.EmbedItem{
		{Prototype: `init()`, Object: Init},
		{Prototype: `initcmd(str)`, Object: InitCmd},
		{Prototype: `deinit()`, Object: Deinit},
		{Prototype: `LogOutput(int,str)`, Object: LogOutput},
		{Prototype: `macro(str) str`, Object: Macro},
		{Prototype: `SetLogLevel(int) int`, Object: SetLogLevel},
		{Prototype: `SetVariable(str,str)`, Object: SetVariable},
	}
)

func Deinit() {
	dataScript.Mutex.Lock()
	defer dataScript.Mutex.Unlock()
	dataScript.Vars = dataScript.Vars[:len(dataScript.Vars)-1]
}

func Init() {
	dataScript.Mutex.Lock()
	defer dataScript.Mutex.Unlock()
	dataScript.Vars = append(dataScript.Vars, make(map[string]string))
}

func InitCmd(name string, pars ...interface{}) bool {
	params := make([]string, len(pars))
	for i, par := range pars {
		switch par.(type) {
		case string:
			params[i] = `"` + fmt.Sprint(par) + `"`
		default:
			params[i] = fmt.Sprint(par)
		}
	}
	LogOutput(LOG_DEBUG, fmt.Sprintf("=> %s(%s)", name, strings.Join(params, `, `)))
	return true
}

func LogOutput(level int64, message string) {
	var mode = []string{``, `ERROR`, `WARN`, `INFO`, `DEBUG`}
	if level < LOG_ERROR || level > LOG_DEBUG {
		return
	}
	dataScript.Mutex.Lock()
	defer dataScript.Mutex.Unlock()
	if level > dataScript.LogLevel {
		return
	}
	dataScript.chLogout <- fmt.Sprintf("[%s] %s %s",
		mode[level], time.Now().Format(`2006/01/02 15:04:05`), message)
}

func replace(values map[string]string, input []rune, stack *[]string) ([]rune, error) {
	if len(input) == 0 || strings.IndexRune(string(input), VarChar) == -1 {
		return input, nil
	}
	var (
		err        error
		isName, ok bool
		value      string
		tmp        []rune
	)
	result := make([]rune, 0, len(input))
	name := make([]rune, 0, VarLength+1)

	for i := 0; i < len(input); i++ {
		r := input[i]
		if r != VarChar {
			if isName {
				name = append(name, r)
				if len(name) > VarLength {
					result = append(append(result, VarChar), name...)
					isName = false
					name = name[:0]
				}
			} else {
				result = append(result, r)
			}
			continue
		}
		if isName {
			value, ok = values[string(name)]
			if ok {
				if len(*stack) < VarDeep {
					for _, item := range *stack {
						if item == string(name) {
							return result, fmt.Errorf(ErrVarLoop, item)
						}
					}
				} else {
					return result, fmt.Errorf(ErrVarDeep)
				}
				*stack = append(*stack, string(name))
				if tmp, err = replace(values, []rune(value), stack); err != nil {
					return result, err
				}
				*stack = (*stack)[:len(*stack)-1]
				result = append(result, tmp...)
			} else {
				result = append(append(result, VarChar), name...)
				i--
			}
			name = name[:0]
		}
		isName = !isName
	}
	if isName {
		result = append(append(result, VarChar), name...)
	}
	return result, nil
}

func Macro(in string) (string, error) {
	dataScript.Mutex.Lock()
	defer dataScript.Mutex.Unlock()
	stack := make([]string, 0)
	out, err := replace(dataScript.Vars[len(dataScript.Vars)-1], []rune(in), &stack)
	return string(out), err
}

func SetLogLevel(level int64) int64 {
	dataScript.Mutex.Lock()
	defer dataScript.Mutex.Unlock()
	ret := dataScript.LogLevel
	if level >= LOG_DISABLE && level < LOG_INHERIT {
		dataScript.LogLevel = level
	}
	return ret
}

func SetVariable(name, value string) {
	dataScript.Mutex.Lock()
	defer dataScript.Mutex.Unlock()
	id := len(dataScript.Vars) - 1
	dataScript.Vars[id][name] = value
}

func InitData(chLogout chan string) {
	dataScript.Vars = make([]map[string]string, 0, 8)
	dataScript.chLogout = chLogout
}

func InitEngine() error {
	return gentee.Customize(&gentee.Custom{
		Embedded: customLib,
	})
}