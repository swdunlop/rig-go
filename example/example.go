package main

import (
	"bytes"

	"github.com/rs/zerolog"
	"github.com/swdunlop/html-go/hog"
	"github.com/swdunlop/rig-go/example/printf"
	"github.com/swdunlop/rig-go/rig"
	"github.com/swdunlop/rig-go/rig/api"
	"github.com/swdunlop/rig-go/rig/local"
	"github.com/swdunlop/rig-go/rig/mrpc"
	"github.com/tinylib/msgp/msgp"
)

func main() {
	rig.Main(
		local.Rig(
			local.TCP(`localhost:8080`),
		),
		api.Rig(
			// api.Use(hog.Middleware()),
			api.FS(wwwFS, // is either os.DirFS(`www`) or an embed.FS when built with `deploy` tag.
				`GET /`, // becomes index.html due to screwy Go behavior.
				`GET /style.css`,
				`GET /example.js`,
				`GET /favicon.ico`,
			),
			mrpc.API(
				`GET /mrpc`,
				mrpc.Use(func(next mrpc.Handler) mrpc.Handler {
					return func(ctx *mrpc.Scope) {
						ctx.Context = hog.With(ctx, func(z zerolog.Context) zerolog.Context {
							return z.
								Str(`id`, ctx.ID).
								Str(`method`, ctx.Method).
								Str(`fn`, ctx.Function)
						})
						evt := hog.From(ctx).Trace()
						if evt.Enabled() {
							var buf bytes.Buffer
							_, _ = msgp.UnmarshalAsJSON(&buf, ctx.Request.Input)
							js := buf.Bytes()
							evt.RawJSON(`input`, js).Msg(``)
						}
						next(ctx)
					}
				}),
				mrpc.CallFn(`printf`, printf.Call),
			),
		),
		rigExtras, // defines rules to rebuild the rig when files change.
	)
}

func debugMRPC(ctx *mrpc.Scope) {

}
