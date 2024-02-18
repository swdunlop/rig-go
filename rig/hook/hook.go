// Package hook defines interfaces that the rig.Hook option recognizes and will apply at various stages of setting up
// a new rig.
package hook

import (
	"net"
	"net/http"
	"sort"
)

// Listener hooks are called when the rig is setting up a new listener.
type Listener interface {
	RigListener(*net.ListenConfig)
}

// Server hooks are called when the rig is setting up a new HTTP server.
type Server interface {
	RigServer(*http.Server)
}

// Mux hooks are called when the rig is setting up a new HTTP multiplexer.
type Mux interface {
	RigMux(*http.ServeMux)
}

// Order will return the provided hooks in the order they were provided with adjustments made so that all dependent
// hooks are run after their dependencies.  Note that cyclic dependencies will not produce an error, the order will
// simply be best effort.
func Order(hooks ...interface{}) []any {
	dependencies := make(map[string][]int, len(hooks))
	for i, hook := range hooks {
		if dependency, ok := hook.(Provider); ok {
			for _, name := range dependency.Provides() {
				dependencies[name] = append(dependencies[name], i)
			}
		}
	}
	order := make([]interface{}, 0, len(hooks))
	placed := make([]bool, len(hooks))
	var place func(int)
	place = func(i int) {
		if placed[i] {
			return
		}
		placed[i] = true
		if dependent, ok := hooks[i].(Dependent); ok {
			names := dependent.DependsOn()
			items := make([]int, 0, len(names))
			for _, name := range names {
				items = append(items, dependencies[name]...)
			}
			sort.Ints(items) // try to preserve the original order as much as possible
			for _, j := range items {
				place(j)
			}
		}
		order = append(order, hooks[i])
	}
	for i := range hooks {
		place(i)
	}
	return order
}

// A Provider provides a name so that it can be referenced by a Dependent.
type Provider interface {
	Provides() []string
}

// A Dependent hook will not be called until all of its dependencies have been provided.
type Dependent interface {
	DependsOn() []string
}
