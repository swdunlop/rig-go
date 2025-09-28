# Rig-Go -- Rigging Useful Go Features into a HTTP Server

This package is an opinionated meta package that binds a number of useful Go packages and tools into a system of [functional options](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis) that can be used to quickly stand up a Go server.

*See [./example](./example) for a detailed example of how to use this package, for internals, read on..*

Each feature is integrated as a separate package under [./rig](./rig), where each rig package exports a function that builds `rig.Option` from options in the package.

### The `rig` Utility

If your Rig server uses `rig.Main`, `rig.Run`, or `rig.Config.Run` as an entrypoint, it can be wrapped by the `rig` utility.  This lets the utility manage the actual listeners and forward the requests to your server as a child process using the `RIG_SOCKET` feature (see "Rig Internals" below).

You can fetch the utility with `go install github.com/swdunlop/rig-go@latest`.  (The utility is not necessary to use the [rig](./rig) package itself.)

### Features Provided as Rigs

HTTP Request Handlers:

- [API](./rig/api) -- Bind HTTP request handlers and middleware into the service.
- [JRPC](./rig/jrpc) -- Supports a simplified subset of JSON-RPC 2.0 over Websockets.
- [MRPC](./rig/mrpc) -- Like [JRPC](./rig/jrpc) but using MessagePack instead of JSON.

HTTP Services:

- [Local](./rig/local) -- Serve HTTP requests to clients over various networks supported by Go, such as TCP or UNIX.
- [Tailscale](./rig/tailscale) -- Serve HTTP requests over a Tailscale network, either privately, or publicly via funnels.
- [Esbuild](./rig/estbuild) -- Bundle various resources into a JavaScript bundle using [esbuild](https://esbuild.github.io).

### Rig Internals

Each `rig.Option` expects `rig.Config` as an argument, and applies itself to the configuration by hooking various parts of the server configuration.  The necessary hook interfaces are described in the [rig/hook](./rig/hook) package.

The `rig.Main`, `rig.Run` and `rig.Config.Run` functions in [rig/rig.go](rig/rig.go) will check the OS environment for `RIG_SOCKET` -- if this variable is set, they will listen to that address instead of any other network.  This behavior is used by `rig.Spawn` and the top level [run.go](./run.go) to spawn a service as a child process.

Originally, this behavior was meant to be connected to a filesystem watcher so the child process could be rebuilt for a hot reload without losing the state of the outer process.  It might, still.

### Contributions

Bug fixes are welcome, but this is mostly just where I squirrel away things that I use in PoCs, so features might be better in other repositories.  The hook functionality is intended to enable third party rigs.
