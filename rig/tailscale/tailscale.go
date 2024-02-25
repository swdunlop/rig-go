package tailscale

import (
	"context"
	"errors"
	"net"

	"github.com/swdunlop/rig-go/rig"
	"github.com/swdunlop/rig-go/rig/hook"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

// Rig returns a rig.Option that configures a Tailscale server.
func Rig(address string, options ...Option) rig.Option {
	return func(r *rig.Config) error {
		var cfg config
		cfg.listen = address
		for _, option := range options {
			err := option(&cfg)
			if err != nil {
				return err
			}
		}
		return cfg.rig(r)
	}
}

type config struct {
	tsnet   tsnet.Server
	funnel  bool
	noTLS   bool
	upHooks []func(*tsnet.Server, *ipnstate.Status) error
	listen  string
}

func (cfg *config) rig(r *rig.Config) error {
	if cfg.funnel && cfg.noTLS {
		return errors.New("funnels are required to use TLS by Tailscale")
	}
	r.Hook(cfg)
	return nil
}

func (cfg *config) Listen(ctx context.Context) (net.Listener, error) {
	status, err := cfg.tsnet.Up(ctx)
	if err != nil {
		return nil, err
	}
	for _, fn := range cfg.upHooks {
		err = fn(&cfg.tsnet, status)
		if err != nil {
			_ = cfg.tsnet.Close()
			return nil, err
		}
	}
	switch {
	case cfg.funnel:
		return cfg.tsnet.ListenFunnel(`tcp`, cfg.listen)
	case cfg.noTLS:
		return cfg.tsnet.Listen(`tcp`, cfg.listen)
	default:
		return cfg.tsnet.ListenTLS(`tcp`, cfg.listen)
	}
}

var _ hook.Listen = (*config)(nil)

type Option func(*config) error

func Dir(dir string) Option {
	return func(cfg *config) error {
		cfg.tsnet.Dir = dir
		return nil
	}
}

// Hostname specifies the name of your Tailscale host.  Defaults to the system hostname.
func Hostname(hostname string) Option {
	return func(cfg *config) error {
		cfg.tsnet.Hostname = hostname
		return nil
	}
}

// Funnel tells Tailscale to allow public IPs to connect to your service.
func Funnel() Option {
	return func(cfg *config) error {
		cfg.funnel = true
		return nil
	}
}

// NoTLS tells Tailscale to not use TLS.  This is incompatible with Funnel.
func NoTLS() Option {
	return func(cfg *config) error {
		cfg.noTLS = true
		return nil
	}
}

// Logf sets the logging function for the Tailscale server.  Tailscale is EXTREMELY chatty.
// The default is to log to the standard logger.
func Logf(f func(format string, args ...interface{})) Option {
	return func(cfg *config) error {
		cfg.tsnet.Logf = f
		return nil
	}
}

// HookUp adds a function that will be called when the Tailscale connection is established and authorized.  This
// is particularly useful for getting the public IP address of the Tailscale server and its FQDN.  If the hook
// returns an error, the Tailscale connection will be closed.
func HookUp(fn func(*tsnet.Server, *ipnstate.Status) error) Option {
	return func(cfg *config) error {
		cfg.upHooks = append(cfg.upHooks, fn)
		return nil
	}
}
