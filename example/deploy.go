//go:build deploy
// +build deploy

package main

import (
	"embed"

	"github.com/swdunlop/rig-go/rig"
)

//go:embed www
var wwwFS embed.FS
var rigExtras = rig.Apply()
