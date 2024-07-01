// Package mrpc implements a simple RPC system using MessagePack over WebSockets that supports streaming responses and
// middleware.
package mrpc

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/swdunlop/html-go/hog"
	"github.com/swdunlop/rig-go/rig/mrpc/internal/protocol"

	"github.com/swdunlop/rig-go/rig/api"
	"github.com/tinylib/msgp/msgp"
	"nhooyr.io/websocket"
)

// API returns an api.Option that supports RPC requests at the specified route.
func API(route string, options ...Option) api.Option {
	return api.Handle(route, Handle(options...))
}

// Handle returns a http.Handler that upgrades the connection to a WebSocket and handles RPC requests
// until the connection is closed.
func Handle(options ...Option) http.Handler {
	var cfg config
	cfg.init(options...)
	return &cfg
}

// Use specifies middleware that is applied to all requests.
func Use(fn func(Handler) Handler) Option {
	return func(cfg *config) {
		cfg.handler = fn(cfg.handler)
	}
}

// For creates a new context for the given request and send function.  Generally this is not necessary but it can
// be useful for testing.
func For(ctx context.Context, req protocol.Request, send func(bin []byte) error) *Scope {
	self := &Scope{Context: ctx, Request: req, send: send}
	self.Context = context.WithValue(ctx, ctxKey{}, self)
	return self
}

// From returns the context of the request from a Go context.  May return nil if there is no RPC
// context in the Go context.
func From(ctx context.Context) *Scope {
	rcx, _ := ctx.Value(ctxKey{}).(*Scope)
	return rcx
}

type ctxKey struct{}

// A Scope describes the scope of an RPC request.
type Scope struct {
	context.Context
	protocol.Request
	send func(bin []byte) error
}

// Succ sends a success response to the client.
func (ctx *Scope) Succ(output msgp.MarshalSizer) error { return ctx.Respond(`succ`, output) }

// Yield yields a response to the client.
func (ctx *Scope) Yield(output msgp.MarshalSizer) error { return ctx.Respond(`yield`, output) }

// End sends an end response to the client.  You may not send any more responses after this.
func (ctx *Scope) End() error {
	err := ctx.Respond(`end`, nil)
	ctx.send = nil
	return err
}

// Fail sends a failure response to the client.
func (ctx *Scope) Fail(code int, msg string) error {
	err := ctx.Respond(`fail`, protocol.Fail{Code: code, Msg: msg})
	ctx.send = nil
	return err
}

// Respond sends a response to the client.  The method is typically one of "succ", "fail", "yield" or "end" and the
// output depends on the method.
func (ctx *Scope) Respond(method string, output msgp.MarshalSizer) error {
	if ctx.send == nil {
		// This happens if the context has ended or when the context was created with a
		// nil send function.  This is a programming error.
		return fmt.Errorf(`response not supported`)
	}
	ret := protocol.Response{ID: ctx.ID, Method: method, Output: output}
	// fmt.Printf("id: %q, method: %q\n", ret.ID, ret.Method)
	msg, err := ret.MarshalMsg(nil)
	if err != nil {
		return fmt.Errorf(`%w while encoding response`, err)
	}
	return ctx.send(msg)
}

// An Option affects the rigging of an RPC API.
type Option func(*config)

type config struct {
	handler       Handler
	startHandlers map[string]Handler
	callHandlers  map[string]Handler
}

func (cfg *config) init(options ...Option) {
	cfg.handler = cfg.handleRequest
	cfg.startHandlers = make(map[string]Handler, len(options))
	cfg.callHandlers = make(map[string]Handler, len(options))
	for _, opt := range options {
		opt(cfg)
	}
}

// ServeHTTP implements http.Handler.
func (cfg *config) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := cfg.serveHTTP(w, r)
	if err != nil {
		hog.For(r).Error().Err(err).Msg(`MRPC error`)
	}
}

func (cfg *config) serveHTTP(w http.ResponseWriter, r *http.Request) error {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}
	defer func() { _ = c.CloseNow() }()
	send := func(bin []byte) error {
		return c.Write(r.Context(), websocket.MessageBinary, bin)
	}
	handle := cfg.handler

	ctx := r.Context()
	var group sync.WaitGroup
	defer group.Wait()
	for {
		mt, msg, err := c.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) < 0 {
				return err
			}
			return nil
		}
		if mt != websocket.MessageBinary {
			continue
		}
		var req protocol.Request
		_, err = req.UnmarshalMsg(msg)
		if err != nil {
			return err
		}
		group.Add(1)
		go func() {
			defer group.Done()
			handle(For(ctx, req, send))
		}()
	}
}

func (cfg *config) handleRequest(ctx *Scope) {
	var table map[string]Handler
	switch ctx.Method {
	case "call":
		table = cfg.callHandlers
	case "start":
		table = cfg.startHandlers
	default:
		ctx.Fail(404, `method not found`)
		return
	}
	handler := table[ctx.Function]
	if handler == nil {
		ctx.Fail(404, fmt.Sprintf(`function %q not found`, ctx.Function))
		return
	}
	handler(ctx)
}

// A CallFn is a function that handles a "call" request.
func CallFn[I any, PI interface {
	*I
	msgp.Unmarshaler
}, O any, PO interface {
	*O
	msgp.MarshalSizer
}](function string, fn func(*Scope, I) (O, error)) Option {
	return func(cfg *config) {
		cfg.callHandlers[function] = func(ctx *Scope) {
			in := PI(new(I))
			_, err := in.UnmarshalMsg(ctx.Input)
			if err != nil {
				_ = ctx.Fail(406, fmt.Sprintf(`%v while decoding input`, err))
				return
			}
			out, err := fn(ctx, *in)
			if err != nil {
				_ = ctx.Fail(500, err.Error())
				return
			}
			_ = ctx.Succ(PO(&out))
		}
	}
}

// A StartFn is a function that handles a "start" request.  This function must call ctx.Yield for each
// response and should not call ctx.Succ, ctx.Fail, ctx.End or ctx.Respond directly.  The framework
// will call either ctx.End or ctx.Fail when the function returns.
func StartFn[I any, PI interface {
	*I
	msgp.Unmarshaler
}, O any, PO interface {
	*O
	msgp.MarshalSizer
}](function string, fn func(ctx *Scope) error) Option {
	return func(cfg *config) {
		cfg.startHandlers[function] = func(ctx *Scope) {
			in := PI(new(I))
			_, err := in.UnmarshalMsg(ctx.Input)
			if err != nil {
				_ = ctx.Fail(406, fmt.Sprintf(`%v while decoding input`, err))
				return
			}
			err = fn(ctx)
			if err != nil {
				_ = ctx.Fail(500, err.Error())
			} else {
				_ = ctx.End()
			}
		}
	}
}

// A Handler is a function that handles an RPC request.
type Handler func(*Scope)

type Input[T any] interface {
	*T
	UnmarshalMsg([]byte) ([]byte, error)
}

type Output[T any] interface {
	*T
	MarshalMsg([]byte) ([]byte, error)
	Msgsize() int
}
