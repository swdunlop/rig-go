package www

import "github.com/swdunlop/rig-go/rig"

// Rig returns a rig option that configures a rig to serve static files from the given directory.  The files will have an entity tag associated with
// them that is invalidated when the file changes.
func Rig(dir string) rig.Option {
	panic(`TODO`)
}
