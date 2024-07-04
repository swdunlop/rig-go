//go:build !deploy
// +build !deploy

package main

import (
	"os"

	"github.com/swdunlop/rig-go/rig"
	"github.com/swdunlop/rig-go/rig/esbuild"
)

var wwwFS = os.DirFS(`www`)
var rigExtras = rig.Apply(
	esbuild.Rig(
		esbuild.Output(`www`),
		esbuild.EntryPoint(`example.ts`),
		esbuild.Bundle(true),
	),
)
