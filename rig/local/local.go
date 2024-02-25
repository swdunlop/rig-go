package local

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/swdunlop/rig-go/rig"
	"github.com/swdunlop/rig-go/rig/hook"
)

// Rig returns a rig.Option that configures a network listener.
func Rig(options ...Option) rig.Option {
	return func(r *rig.Config) error {
		var cfg config
		for _, option := range options {
			err := option(&cfg)
			if err != nil {
				return err
			}
		}
		return cfg.rig(r)
	}
}

// An Option is a function that configures a local listener.
type Option func(*config) error

type config struct {
	listen struct {
		network string
		address string
		config  net.ListenConfig
	}
}

// TCP returns an Option that sets the listener to a TCP socket on the provided address.
func TCP(address string) Option {
	return Listen("tcp", address)
}

// Unix returns an Option that sets the listener to a Unix socket on the provided path.
func Unix(path string) Option {
	return Listen("unix", path)
}

// Listen returns an Option that sets the listener to the provided network and address.
func Listen(network, address string) Option {
	return func(cfg *config) error {
		cfg.listen.network = network
		cfg.listen.address = address
		return nil
	}
}

// Listen implements hook.Listen by returning a net.Listener for the configured network and address.
func (cfg *config) Listen(ctx context.Context) (net.Listener, error) {
	return cfg.listen.config.Listen(ctx, cfg.listen.network, cfg.listen.address)
}

// KeepAlive specifies the keepalive duration for connections accepted by the listener.
func (cfg *config) KeepAlive(keepalive time.Duration) Option {
	return func(cfg *config) error {
		cfg.listen.config.KeepAlive = keepalive
		return nil
	}
}

// ListenConfig returns a rig.Option that adjusts the net.ListenConfig used to create local listeners.
func ListenConfig(options ...func(*net.ListenConfig)) Option {
	return func(cfg *config) error {
		for _, option := range options {
			option(&cfg.listen.config)
		}
		return nil
	}

}

var _ hook.Listen = (*config)(nil)

func (cfg *config) rig(r *rig.Config) error {
	if cfg.listen.network == `` || cfg.listen.address == `` {
		return errors.New(`local listeners must configure both network and address`)
	}
	r.Hook(cfg)
	return nil
}
