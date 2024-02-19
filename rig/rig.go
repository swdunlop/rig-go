// Package rig manages a configuration of HTTP handlers rigged together in a way that will rebuild them when their inputs change.
// Web applications can observe when a restart has occurred by subscribing to server sent events at /_rig/restart.
package rig

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
	"github.com/swdunlop/html-go/hog"
	"github.com/swdunlop/rig-go/rig/hook"
)

func init() {
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: `2006-01-02 15:04:05`}).With().Timestamp().Logger()
	zlog.Logger = log
	zerolog.DefaultContextLogger = &log
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

}

// Main is intended to be used as your main function and will run a rig with the given options.
func Main(options ...Option) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	cfg, err := New(options...)
	if err != nil {
		return err
	}
	return cfg.Run(ctx)
}

// Run will either Serve the rig if RIG_SOCKET is set, or will start a child process with RIG_SOCKET to serve the rig
// that may be rebuilt when its inputs change.
func Run(ctx context.Context, options ...Option) error {
	cfg, err := New(options...)
	if err != nil {
		return err
	}
	return cfg.Run(ctx)
}

// Serve will serve the configured rig at the specified address.  Unlike Run, this will not use a child process to
// serve the rig, and therefore will not rebuild the server and restart it when its inputs change.
func Serve(ctx context.Context, options ...Option) error {
	cfg, err := New(options...)
	if err != nil {
		return err
	}
	return cfg.Serve(ctx)
}

// New returns a new rig configuration.
func New(options ...Option) (*Config, error) {
	cfg := new(Config)
	err := cfg.Apply(options...)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// A Config is a rig configuration.
type Config struct {
	serve   bool            // true once Serve has been called
	serving bool            // true after Serve has been called and before it returns
	hooks   []any           // hooks to apply
	done    <-chan struct{} // closed when the rig starts to shut down
	watch   []watch
}

type watch struct {
	dir      string
	patterns []string
}

// Done returns a channel that will be closed when the rig starts to shut down.  This is nil unless the rig has been
// started with a context.
func (cfg *Config) Done() <-chan struct{} {
	return cfg.done
}

// Hook adds hooks to the configuration, see the hook package for interfaces that hooks can implement.  This is
// normally done by various options.
func (cfg *Config) Hook(hooks ...any) {
	cfg.hooks = append(cfg.hooks, hooks...)
}

// Apply applies the given options to the config; should not be called after Run.
func (cfg *Config) Apply(options ...Option) error {
	if cfg.serving {
		return errors.New(`cannot apply options while a rig is running`)
	} else if cfg.serve {
		return errors.New(`cannot apply options after a rig has been run`)
	}

	for _, option := range options {
		err := option(cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

// Run will either Serve the rig if RIG_SOCKET is set, or will start a child process with RIG_SOCKET to serve the rig.
func (cfg *Config) Run(ctx context.Context) error {
	panic(`TODO`)
}

// Serve will run the configured rig as a server listening to the provided address until the context is cancelled.  If
// the address starts with "." or "/", it will be interpreted as a Unix domain socket.  Otherwise, it will be interpreted
// as a TCP address.
func (cfg *Config) Serve(ctx context.Context) error {
	cfg.serve = true
	cfg.serving = true

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cfg.done = ctx.Done()
	defer func() { cfg.done, cfg.serving = nil, false }()

	var mux http.ServeMux
	for _, it := range cfg.hooks {
		if impl, ok := it.(hook.Mux); ok {
			impl.RigMux(&mux)
		}
	}

	var svr http.Server
	svr.BaseContext = func(l net.Listener) context.Context { return ctx }
	svr.Handler = &mux
	for _, it := range cfg.hooks {
		if impl, ok := it.(hook.Server); ok {
			impl.RigServer(&svr)
		}
	}

	var listeners []net.Listener
	for _, it := range cfg.hooks {
		impl, ok := it.(hook.Listen)
		if !ok {
			continue
		}
		listener, err := impl.Listen(ctx)
		if err != nil {
			for _, lr := range listeners {
				err := lr.Close()
				if err != nil {
					hog.From(ctx).Error().Err(err).Msg(`failed to close listener`)
				}
			}
			return err
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) < 1 {
		lr, err := net.Listen(`tcp`, `localhost:`)
		if err != nil {
			return err
		}
		listeners = append(listeners, lr)
	}

	// no need to defer lr.Close, svr.Shutdown will close it
	go func() {
		<-ctx.Done()
		svr.Shutdown(context.Background())
	}()

	var wg sync.WaitGroup
	wg.Add(len(listeners))
	defer wg.Wait()

	for _, lr := range listeners {
		go func(lr net.Listener) {
			defer wg.Done()
			addr := lr.Addr().String()
			err := svr.Serve(lr)
			switch err {
			case nil, http.ErrServerClosed:
				hog.From(ctx).Info().Str(`listener`, addr).Msg(`HTTP service stopped`)
			default:
				hog.From(ctx).Info().Str(`listener`, addr).Err(err).Msg(`HTTP service error`)
			}
		}(lr)
	}
	return nil
}

// Watch will trigger notifying clients watching "/_rig/build" when any file in the given directory changes that
// matches the given glob patterns.  This is normally done by various options like esbuild.
//
// If nothing is being watched, the "/_rig/build" endpoint will not be registered.
func (cfg *Config) Watch(dir string, patterns ...string) error {
	cfg.watch = append(cfg.watch, watch{dir, patterns})
	return nil
}

// An Option is a function that modifies a Config before it is Run.
type Option func(*Config) error
