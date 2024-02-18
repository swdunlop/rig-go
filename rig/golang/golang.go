// Package golang provides a rig option that will build and run a Go package to handle requests as a subprocess.
//
// Since this includes the Go toolchain, it can be a bit heavy to use for development -- you may want to omit this option
// in production builds.
package golang

import "github.com/swdunlop/rig-go/rig"

// Rig returns a rig option that configures a rig to proxy any unhandled requests to a subprocess running the given Go package.
// This subprocess should listen for Unix domain socket connections on the path specified by the RIG_SOCKET environment variable.
// When any file in the package changes, the subprocess will be rebuilt and restarted.
func Rig(pkg string) rig.Option {
	panic(`TODO`)
}
