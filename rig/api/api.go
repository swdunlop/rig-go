package api

import (
	"io/fs"
	"net/http"

	"github.com/swdunlop/rig-go/rig"
)

// Rig returns a rig option that configures a rig to serve an API from the given package.
func Rig(options ...Option) rig.Option {
	var cfg config
	cfg.apply(options...)
	return cfg.rigOption
}

// FS returns an option that serves the given file system at any of the given patterns.
func FS(filesystem fs.FS, patterns ...string) Option {
	return func(cfg *config) error {
		fs := http.FileServer(http.FS(filesystem))
		for _, pattern := range patterns {
			cfg.patternHandlers = append(cfg.patternHandlers, patternHandler{pattern, fs})
		}
		return nil
	}
}

// Use returns an option that applies the given middleware to all subsequent handlers.  You can stack middleware multiple times, the
// earliest middleware added will be the outermost layer and therefore will be run first.
//
// Any Go middleware that takes a http.Handler and returns a http.Handler can be used with this function, such as those commonly used
// with Chi.
func Use(fn func(http.Handler) http.Handler) Option {
	return func(cfg *config) error {
		cfg.middleware = append(cfg.middleware, fn)
		return nil
	}
}

// HandleFunc accepts a http.ServeMux pattern and a handler function.
func HandleFunc(pattern string, fn func(w http.ResponseWriter, r *http.Request)) Option {
	var handler http.Handler = http.HandlerFunc(fn)
	return Handle(pattern, handler)
}

// Handle accepts a http.ServeMux pattern and a http.Handler.
func Handle(pattern string, handler http.Handler) Option {
	return func(cfg *config) error {
		for i := len(cfg.middleware) - 1; i >= 0; i-- {
			handler = cfg.middleware[i](handler)
		}
		cfg.patternHandlers = append(cfg.patternHandlers, patternHandler{
			pattern: pattern,
			handler: handler,
		})
		return nil
	}
}

// Group organizes a group of options into a single option.  This is useful for isolating a set of handlers and middleware so that
// the middleware does not affect handlers outside of the group.
func Group(options ...Option) Option {
	return func(cfg *config) error {
		old := struct {
			middleware []func(http.Handler) http.Handler
		}{cfg.middleware}
		defer func() { cfg.middleware = old.middleware }()
		for _, option := range options {
			err := option(cfg)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

type Option func(*config) error

type config struct {
	middleware      []func(http.Handler) http.Handler
	patternHandlers []patternHandler
	err             error
}

// RigMux adds the configured handlers to the provided ServeMux, implementing the hook.Mux interface.
func (cfg *config) RigMux(mux *http.ServeMux) {
	for _, it := range cfg.patternHandlers {
		mux.Handle(it.pattern, it.handler)
	}
}

type patternHandler struct {
	pattern string
	handler http.Handler
}

func (cfg *config) apply(options ...Option) {
	for _, option := range options {
		if cfg.err != nil {
			return
		}
		cfg.err = option(cfg)
	}
}

func (cfg *config) rigOption(r *rig.Config) error {
	r.Hook(cfg)
	return nil
}
