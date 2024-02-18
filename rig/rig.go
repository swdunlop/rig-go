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

// Main is intended to be used as your main function and will run a rig with the given options.  If it observes the special environment variable
// RIG_SOCKET, it will ignore most of its options and simply listen to that socket for requests and handle them.
func Main(listenAddress string, options ...Option) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	cfg, err := New(options...)
	if err != nil {
		return err
	}
	if socket := os.Getenv(`RIG_SOCKET`); socket != `` {
		return cfg.Serve(ctx, socket)
	}
	return cfg.Run(ctx, listenAddress)
}

// Run will either Serve the rig if RIG_SOCKET is set, or will start a child process with RIG_SOCKET to serve the rig
// that may be rebuilt when its inputs change.
func Run(ctx context.Context, listenAddress string, options ...Option) error {
	cfg, err := New(options...)
	if err != nil {
		return err
	}
	return cfg.Run(ctx, listenAddress)
}

// Serve will serve the configured rig at the specified address.  Unlike Run, this will not use a child process to
// serve the rig, and therefore will not rebuild the server and restart it when its inputs change.
func Serve(ctx context.Context, address string, options ...Option) error {
	cfg, err := New(options...)
	if err != nil {
		return err
	}
	return cfg.Serve(ctx, address)
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
func (cfg *Config) Run(ctx context.Context, listenAddress string) error {
	address := os.Getenv(`RIG_SOCKET`)
	if address != `` {
		return cfg.Serve(ctx, address)
	}
	panic(`TODO`)
}

// Serve will run the configured rig as a server listening to the provided address until the context is cancelled.  If
// the address starts with "." or "/", it will be interpreted as a Unix domain socket.  Otherwise, it will be interpreted
// as a TCP address.
func (cfg *Config) Serve(ctx context.Context, address string) error {
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
	svr.Handler = &mux
	for _, it := range cfg.hooks {
		if impl, ok := it.(hook.Server); ok {
			impl.RigServer(&svr)
		}
	}

	var lcf net.ListenConfig
	for _, it := range cfg.hooks {
		if impl, ok := it.(hook.Listener); ok {
			impl.RigListener(&lcf)
		}
	}

	lr, err := lcf.Listen(ctx, `tcp`, address)
	if err != nil {
		return err
	}
	// no need to defer lr.Close, svr.Shutdown will close it

	go func() {
		<-ctx.Done()
		svr.Shutdown(context.Background())
	}()

	hog.From(ctx).Info().Str(`address`, address).Msg(`starting HTTP service`)
	err = svr.Serve(lr)
	hog.From(ctx).Info().Err(err).Msg(`HTTP service stopped`)
	if err == http.ErrServerClosed {
		return nil
	}
	_ = lr.Close() // just in case, since we did not have a shutdown or server close.
	return err
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
