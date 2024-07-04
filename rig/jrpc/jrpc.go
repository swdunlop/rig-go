package jrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/swdunlop/html-go/hog"
	"github.com/swdunlop/rig-go/rig/api"
	"github.com/swdunlop/rig-go/rig/jrpc/internal/protocol"
	"nhooyr.io/websocket"
)

// API returns an api.Option that supports RPC requests at the specified route.
func API(route string, options ...Option) api.Option {
	return api.Handle(route, Handle(options...))
}

// ReadLimit specifies the maximum size of a read message.  Defaults to -1 which imposes no limit.
func ReadLimit(limit int64) Option {
	// TODO: port to mrpc
	return func(cfg *config) { cfg.readLimit = limit }
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
func (ctx *Scope) Succ(result any) error { return ctx.respond(protocol.Response{Result: result}) }

// Fail sends an error response to the client.
func (ctx *Scope) Fail(code int, msg string) error {
	err := ctx.respond(protocol.Response{Error: &protocol.Error{Code: code, Message: msg}})
	ctx.send = nil
	return err
}

// Notify sends a notification to the client.  This is not normally tolerated by
// JSON-RPC 2.0 clients.
func (ctx *Scope) Notify(method string, params any) error {
	msg := protocol.Notification{
		Method: method,
		Params: params,
	}
	js, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	err = ctx.send(js)
	return err
}

// Call sends a request to the client.
func (ctx *Scope) Call(method string, params any) error {
	js, err := json.Marshal(params)
	if err != nil {
		return err
	}
	js, err = json.Marshal(protocol.Request{
		Method: method,
		Params: js,
	})
	if err != nil {
		return err
	}
	err = ctx.send(js)
	return err
}

// output depends on the method.
func (ctx *Scope) respond(ret protocol.Response) error {
	if ctx.send == nil {
		// This happens if the context has ended or when the context was created with a
		// nil send function.  This is a programming error.
		return fmt.Errorf(`response not supported`)
	}
	ret.ID = ctx.ID
	msg, err := json.Marshal(&ret)
	if err != nil {
		return fmt.Errorf(`%w while encoding response`, err)
	}
	println(`sending`, string(msg))
	return ctx.send(msg)
}

// An Option affects the rigging of an RPC API.
type Option func(*config)

type config struct {
	handler      Handler
	readLimit    int64
	procHandlers map[string]Handler
	callHandlers map[string]Handler
}

func (cfg *config) init(options ...Option) {
	cfg.readLimit = -1
	cfg.handler = cfg.handleRequest
	cfg.procHandlers = make(map[string]Handler, len(options))
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
	c.SetReadLimit(cfg.readLimit)
	send := func(bin []byte) error {
		return c.Write(r.Context(), websocket.MessageText, bin)
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
		if mt != websocket.MessageText {
			continue
		}
		var req protocol.Request
		err = json.Unmarshal(msg, &req)
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
	if ctx.ID == `` {
		table = cfg.procHandlers
	} else {
		table = cfg.callHandlers
	}
	handler := table[ctx.Method]
	if handler == nil {
		ctx.Fail(404, fmt.Sprintf(`function %q not found`, ctx.Method))
		return
	}
	handler(ctx)
}

// A Proc is a function that handles a notification.
func Proc[I any](function string, fn func(*Scope, I)) Option {
	return func(cfg *config) {
		cfg.procHandlers[function] = func(ctx *Scope) {
			in := new(I)
			err := json.Unmarshal(ctx.Params, in)
			if err != nil {
				_ = ctx.Fail(406, fmt.Sprintf(`%v while decoding input`, err))
				return
			}
			fn(ctx, *in)
		}
	}
}

// A Fn is a function that handles a request.
func Fn[I, O any](
	function string, fn func(*Scope, I) (O, error),
) Option {
	return func(cfg *config) {
		cfg.callHandlers[function] = func(ctx *Scope) {
			in := new(I)
			err := json.Unmarshal(ctx.Params, in)
			if err != nil {
				_ = ctx.Fail(406, fmt.Sprintf(`%v while decoding input`, err))
				return
			}
			out, err := fn(ctx, *in)
			if err != nil {
				_ = ctx.Fail(500, err.Error())
				return
			}
			_ = ctx.Succ(out)
		}
	}
}

// A Handler is a function that handles an RPC request.
type Handler func(*Scope)
