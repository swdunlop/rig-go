package main

import (
	"context"
	"errors"

	"github.com/swdunlop/rig-go/rig"
	"github.com/swdunlop/rig-go/rig/esbuild"
	"github.com/swdunlop/rig-go/rig/golang"
	"github.com/swdunlop/rig-go/rig/www"
	"github.com/swdunlop/zugzug-go"
	"github.com/swdunlop/zugzug-go/zug/parser"
)

func init() {
	tasks = append(tasks, zugzug.Tasks{
		{Name: "run", Use: "Runs a rigged service", Fn: runRig, Parser: parser.New(
			parser.String(&wwwDir, "www", "d", "The directory to serve for static files"),
			parser.String(&golangPkg, "pkg", "g", "The Go package to serve for API requests"),
			parser.String(&esbuildFile, "ui", "u", "The esbuild file to build for the UI"),
		)},
	}...)
}

func runRig(ctx context.Context) error {
	var options []rig.Option
	if esbuildFile != "" {
		if wwwDir == "" {
			return errors.New("esbuild requires a www directory")
		}
		options = append(options, esbuild.Rig(
			esbuild.EntryPoint(esbuildFile),
			esbuild.OutDir(wwwDir),
		))
	}
	if wwwDir != "" {
		options = append(options, www.Rig(wwwDir))
	}
	if golangPkg != "" {
		options = append(options, golang.Rig(golangPkg))
	}
	return rig.Run(ctx, options...)
}

var (
	wwwDir      string
	golangPkg   string
	esbuildFile string
)
