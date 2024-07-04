package esbuild

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/swdunlop/rig-go/rig"
)

// Rig returns a rig option that configures a rig to build the given esbuild file when it changes.
func Rig(options ...Option) rig.Option {
	var cfg config
	cfg.build.LogLevel = esbuild.LogLevelInfo
	cfg.build.Bundle = true
	cfg.build.Write = true
	for _, option := range options {
		option(&cfg)
	}
	return cfg.rigOption
}

// Option is a function that can manipulate the esbuild API build options structure.
type Option func(*config)

type config struct {
	build esbuild.BuildOptions
	watch esbuild.WatchOptions
}

func (cfg *config) rigOption(r *rig.Config) error {
	// BUG: this option should exit if RIG_SOCKET is not set, only the server process should run.
	if cfg.build.Outdir == "" && cfg.build.Outfile == "" {
		return fmt.Errorf(`esbuild: no output directory or file specified`)
	}
	if len(cfg.build.EntryPoints) == 0 {
		return fmt.Errorf(`esbuild: no entry points specified`)
	}
	doneCh := r.Done()
	errCh := make(chan error)
	go cfg.buildAndWatch(errCh, doneCh)
	startErr := <-errCh
	if startErr != nil {
		return startErr
	}
	if cfg.build.Outdir != `` {
		err := r.Watch(cfg.build.Outdir, `*.html`, `*.css`, `*.js`)
		if err != nil {
			return err
		}
	}
	if cfg.build.Outfile != `` {
		err := r.Watch(filepath.Dir(cfg.build.Outfile), filepath.Base(cfg.build.Outfile))
		if err != nil {
			return err
		}
	}
	return nil
}

func (cfg *config) buildAndWatch(errCh chan<- error, doneCh <-chan struct{}) {
	var err error
	ctx, ctxErr := esbuild.Context(cfg.build)
	if ctxErr != nil {
		printErrors(ctxErr.Errors)
	}
	if ctxErr != nil && len(ctxErr.Errors) > 0 {
		errCh <- fmt.Errorf(`esbuild failed to start`)
		return
	}
	errCh <- nil
	defer ctx.Dispose()
	ret := esbuild.Build(cfg.build)
	printErrors(ret.Errors)
	err = ctx.Watch(cfg.watch)
	if err != nil {
		panic(err)
	}
	<-doneCh
}

func printErrors(errors []esbuild.Message) {
	var buf bytes.Buffer
	for i, err := range errors {
		if i == 0 {
			fmt.Fprintf(&buf, "!! esbuild: ")
		} else {
			fmt.Fprintf(&buf, "   esbuild: ")
		}
		fmt.Fprintf(&buf, "%s\n", strings.ReplaceAll(err.Text, "\n", "\n            "))
	}
	if buf.Len() > 0 {
		os.Stderr.Write(buf.Bytes())
	}
}

// Output returns a rig option that sets the output directory for the esbuild build.
func Output(outdir string) Option {
	return func(cfg *config) { cfg.build.Outdir = outdir }
}

// EntryPoint appends entry points to the esbuild build options.
func EntryPoint(entryPoints ...string) Option {
	return func(cfg *config) { cfg.build.EntryPoints = append(cfg.build.EntryPoints, entryPoints...) }
}

// Bundle returns a rig option that configures esbuild to bundle the output if true, otherwise it will not bundle.
func Bundle(ok bool) Option {
	return func(cfg *config) { cfg.build.Bundle = ok }
}

// BuildOption returns a rig option that can manipulate the esbuild API build options structure.
// See https://esbuild.github.io/api for information on how to use esbuild options.
func BuildOption(fn func(*esbuild.BuildOptions)) Option {
	return func(cfg *config) { fn(&cfg.build) }
}

// WatchOption returns a rig option that can manipulate the esbuild API watch options structure.
// See https://esbuild.github.io/api for information on how to use esbuild options.
func WatchOption(fn func(*esbuild.WatchOptions)) Option {
	return func(cfg *config) { fn(&cfg.watch) }
}
