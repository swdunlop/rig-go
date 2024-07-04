// Package rig manages a configuration of HTTP handlers rigged together in a way that will rebuild them when their inputs change.
// Web applications can observe when a restart has occurred by subscribing to server sent events at /_rig/restart.
package rig

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
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

// Apply defines an option that applies a set of options.
func Apply(options ...Option) Option {
	return func(cfg *Config) error {
		for _, opt := range options {
			err := opt(cfg)
			if err != nil {
				return err
			}
		}
		return nil
	}
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

// Handler returns an http.Handler that will serve the configured rig.
func Handler(options ...Option) http.Handler {
	cfg, err := New(options...)
	if err != nil {
		panic(err)
	}
	return cfg.Handler()
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
	done    <-chan struct{}
	serve   bool  // true once Serve has been called
	serving bool  // true after Serve has been called and before it returns
	worker  bool  // true if Run with RIG_SOCKET in the environment
	hooks   []any // hooks to apply
	watch   []watch
}

type watch struct {
	dir      string
	patterns []string
}

// Done returns a channel that will be closed when the rig is done serving.
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
	// Attempt to reorder the hooks.
	cfg.hooks = hook.Order(cfg.hooks...)
	return nil
}

// Run will either Serve the rig if RIG_SOCKET is set, or will start a child process with RIG_SOCKET to serve the rig.
func (cfg *Config) Run(ctx context.Context) error {
	socket := os.Getenv(`RIG_SOCKET`)
	if socket != `` {
		return cfg.runWorker(ctx, socket)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return cfg.Spawn(ctx, executable, os.Args[1:]...)
}

// Spawn will run a rig as a child process.  This is identical to Run but allows specifying the path to the child
// executable and arguments to pass to it.
func (cfg *Config) Spawn(ctx context.Context, executable string, args ...string) error {
	dir, err := os.MkdirTemp(``, `rig-socket`)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	addr := dir + `/socket`
	worker, err := startWorker(ctx, addr, executable, args)
	if err != nil {
		return err
	}
	defer worker.Wait()
	go func() {
		defer worker.Kill()
		<-ctx.Done()
	}()
	defer cancel() // Note that this is a duplicate that ensures the worker is interrupted if the supervisor is interrupted.

	listeners, err := cfg.listen(ctx)
	if err != nil {
		return err
	}

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: `http`, Host: `rig`})
	proxy.Transport = &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return net.Dial(`unix`, addr)
	}}
	// The supervisor applies only listener and server hooks.
	return cfg.serveListeners(ctx, cfg.Server(ctx, proxy), listeners...)
}

// runWorker will serve the rig at the given unix address.
func (cfg *Config) runWorker(ctx context.Context, addr string) error {
	var lcf net.ListenConfig
	listener, err := lcf.Listen(ctx, `unix`, addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	cfg.worker = true
	// We do not apply server or listener hooks to workers.
	server := &http.Server{
		BaseContext: func(net.Listener) context.Context { return ctx },
		Handler:     cfg.Handler(),
	}
	// Nor do we use the listener hooks.
	return cfg.serveListeners(ctx, server, listener)
}

// Serve will run the configured rig as a server listening to the provided address until the context is cancelled.  If
// the address starts with "." or "/", it will be interpreted as a Unix domain socket.  Otherwise, it will be interpreted
// as a TCP address.
func (cfg *Config) Serve(ctx context.Context) error {
	listeners, err := cfg.listen(ctx)
	if err != nil {
		return err
	}
	return cfg.serveListeners(ctx, cfg.Server(ctx, cfg.Handler()), listeners...)
}

// Server returns an http.Server with the server hooks applied.
func (cfg *Config) Server(ctx context.Context, handler http.Handler) *http.Server {
	server := new(http.Server)
	server.BaseContext = func(net.Listener) context.Context { return ctx }
	server.Handler = handler
	for _, it := range cfg.hooks {
		if impl, ok := it.(hook.Server); ok {
			impl.RigServer(server)
		}
	}
	return server
}

func (cfg *Config) serveListeners(ctx context.Context, server *http.Server, listeners ...net.Listener) error {
	if cfg.serving {
		return fmt.Errorf(`you are already serving this rig`)
	}
	if cfg.serve {
		return fmt.Errorf(`cannot serve a rig more than once`)
	}
	if len(listeners) == 0 {
		return fmt.Errorf(`no listeners for service`)
	}
	// used to spot use of options after service has started
	cfg.serve = true
	cfg.serving = true
	defer func() { cfg.serving = false }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()
	cfg.done = ctx.Done()

	var wg sync.WaitGroup
	wg.Add(len(listeners))
	defer wg.Wait()

	for _, lr := range listeners {
		go func(lr net.Listener) {
			defer wg.Done()
			addr := lr.Addr().String()
			err := server.Serve(lr)
			// Do not babble about shutdown if we are a worker
			if cfg.worker {
				return
			}
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

// listen will return a list of listeners for the configured addresses.
func (cfg *Config) listen(ctx context.Context) ([]net.Listener, error) {
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
			return nil, err
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) < 1 {
		lr, err := net.Listen(`tcp`, `localhost:`)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, lr)
	}
	return listeners, nil
}

// Handler returns an http.Handler that will serve the configured rig.
func (cfg *Config) Handler() http.Handler {
	var mux http.ServeMux
	for _, it := range cfg.hooks {
		if impl, ok := it.(hook.Mux); ok {
			impl.RigMux(&mux)
		}
	}
	return &mux
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

// startWorker will start a child process with RIG_SOCKET set to the given address.
func startWorker(ctx context.Context, addr string, executable string, args []string) (*os.Process, error) {
	// TODO: watch for changes in the directory and restart the worker
	worker := exec.CommandContext(ctx, executable, args...)
	worker.Env = append(os.Environ(), `RIG_SOCKET=`+addr)
	worker.Stdout = os.Stdout
	worker.Stderr = os.Stderr
	worker.Stdin = os.Stdin
	err := worker.Start()
	if err != nil {
		return nil, err
	}
	return worker.Process, nil
}
