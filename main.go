// Copyright 2020 Alexey Krivonogov. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"time"

	"eonza/script"

	"github.com/kataras/golog"
	"github.com/labstack/echo/v4"
)

var stopchan = make(chan os.Signal)

func main() {
	var e *echo.Echo

	golog.SetTimeFormat("2006/01/02 15:04:05")
	flag.StringVar(&cfg.path, "cfg", "", "The path of the `config file`")
	flag.Parse()
	script.InitWorkspace()

	fi, err := os.Stdin.Stat()
	if err != nil {
		golog.Fatal(err)
	}
	if fi.Mode()&os.ModeNamedPipe != 0 {
		IsScript = true
		scriptTask, err := script.Decode()
		if err != nil {
			golog.Fatal(err)
		}
		if err = LoadCustomAsset(scriptTask.Header.AssetsDir, scriptTask.Header.HTTP.Theme); err != nil {
			golog.Fatal(err)
		}
		e = RunServer(WebSettings{
			Port: scriptTask.Header.HTTP.Port,
			Open: true,
			Lang: scriptTask.Header.Lang,
		})
		scriptTask.Run()
	} else {
		LoadConfig()
		LoadStorage()
		defer CloseLog()
		if err = LoadCustomAsset(cfg.AssetsDir, cfg.HTTP.Theme); err != nil {
			golog.Fatal(err)
		}
		InitScripts()
		e = RunServer(WebSettings{
			Port: cfg.HTTP.Port,
			Open: cfg.HTTP.Open,
			Lang: storage.Settings.Lang,
		})
	}
	signal.Notify(stopchan, os.Kill, os.Interrupt)
	<-stopchan

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	e.Shutdown(ctx)
}
