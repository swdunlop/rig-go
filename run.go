package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/swdunlop/rig-go/rig"
	"github.com/swdunlop/rig-go/rig/esbuild"
	"github.com/swdunlop/rig-go/rig/local"
	"github.com/swdunlop/rig-go/rig/tailscale"
	"github.com/swdunlop/zugzug-go"
	"github.com/swdunlop/zugzug-go/zug/parser"
)

func init() {
	tasks = append(tasks, zugzug.Tasks{
		{Name: "run", Use: "Runs a rigged service", Fn: runRig, Parser: parser.New(
			parser.String(&wwwDir, "www", "d", "The directory to serve for static files"),
			parser.String(&esbuildFile, "ui", "u", "The esbuild file to build for the UI"),
		), Settings: zugzug.Settings{
			{Var: &listenNetwork, Name: `LISTEN_NETWORK`,
				Use: "Listening network for the address (default: \"tcp\" if Tailscale not used)"},
			{Var: &listenAddress, Name: `LISTEN_ADDRESS`,
				Use: "Listening address for the service  (default: localhost:8080 if TCP used)"},

			{Var: &tailscaleHostname, Name: `TAILSCALE_HOSTNAME`,
				Use: "Specifies the hostname on your Tailscale network"},
			{Var: &tailscaleFunnel, Name: `TAILSCALE_FUNNEL`,
				Use: "Enables internet access via a Tailscale funnel"},
			{Var: &tailscaleListen, Name: `TAILSCALE_LISTEN`,
				Use: "Listening address for clients from your Tailscale network (default: \":443\" or \":80\")"},
			{Var: &tailscaleDir, Name: `TAILSCALE_DIR`,
				Use: "State directory for Tailscale"},
			{Var: &noTailscaleTLS, Name: `NO_TAILSCALE_TLS`,
				Use: "Disables TLS for Tailscale"},
		}},
	}...)
}

func runRig(ctx context.Context) error {
	args := parser.Args(ctx)
	if len(args) < 1 {
		return errors.New("no Go package specified")
	}
	pkg, args := args[0], args[1:]

	var options []rig.Option
	if esbuildFile != "" {
		if wwwDir == "" {
			return errors.New("esbuild requires a www directory")
		}
		options = append(options, esbuild.Rig(
			esbuild.EntryPoint(esbuildFile),
		))
	}

	var tailscaleOptions []tailscale.Option
	useTailscale := false
	if tailscaleFunnel {
		if noTailscaleTLS {
			return errors.New("Tailscale funnel requires TLS")
		}
		if tailscaleListen != `` {
			return errors.New("You cannot combine TAILSCALE_FUNNEL with TAILSCALE_LISTEN")
		}
		tailscaleListen = `:443`

		useTailscale = true
		tailscaleOptions = append(tailscaleOptions, tailscale.Funnel())
	} else if tailscaleListen != "" {
		useTailscale = true
	} else if noTailscaleTLS {
		tailscaleListen = `:80`
	} else {
		tailscaleListen = `:443`
	}
	if tailscaleHostname != `` {
		useTailscale = true
		tailscaleOptions = append(tailscaleOptions, tailscale.Hostname(tailscaleHostname))
	}
	if noTailscaleTLS {
		tailscaleListen = `:80`
		tailscaleOptions = append(tailscaleOptions, tailscale.NoTLS())
	}
	if tailscaleDir != `` {
		tailscaleOptions = append(tailscaleOptions, tailscale.Dir(tailscaleDir))
	}

	if useTailscale {
		// TODO: use tailscale.HookUp to suppress tailscale logs after registration completes.
		options = append(options, tailscale.Rig(tailscaleListen, tailscaleOptions...))
	} else if listenNetwork == `` {
		listenNetwork = `tcp`
	}

	if listenNetwork != `` {
		if listenAddress != `` {
			// all good.
		} else if listenNetwork == `tcp` {
			listenAddress = `localhost:8080`
		} else {
			return fmt.Errorf(`LISTEN_ADDRESS must be specified for LISTEN_NETWORK other than "tcp"`)
		}
		options = append(options, local.Rig(local.Listen(listenNetwork, listenAddress)))
	}

	cfg, err := rig.New(options...)
	if err != nil {
		return err
	}
	return cfg.Spawn(ctx, `go`, append([]string{`run`, pkg}, args...)...)
}

var (
	wwwDir      string
	golangPkg   string
	esbuildFile string

	listenNetwork string = ``
	listenAddress string = ``

	tailscaleFunnel   bool
	tailscaleHostname string
	tailscaleListen   string
	tailscaleDir      string
	noTailscaleTLS    bool
)
