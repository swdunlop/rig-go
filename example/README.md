 ## Rig-Go Example

 This is a simple example of how to use [Rig-Go](../README.md) to create a HTTP service in Go.  It can be run three different ways.

 ### Running the Example Using Rig

 The `rig` command can run the example while providing support for controlling the server itself.  The following command will run the example using Tailscale to expose the service to the internet:

 ```shell
 env TAILSCALE_FUNNEL=true TAILSCALE_HOSTNAME=example rig run .
 ```

Note that this works despite the fact that [main.go](./main.go) does not have any settings for Tailscale, becaue [rig](../README.md) is running the example as a worker subprocess.  Rig will automatically restart the worker if it crashes or if the source code changes.


## Running the Example Directly

The example can also be run directly using the following command:

```shell
env go run .
```

This will run the example on a random TCP port on the local machine and will automatically build [example.ts](./example.ts) into [example.js](www/example.js) using [esbuild](https://esbuild.github.io/).

## Building and Running a Deployment Version of the Example

This will build the example, embedding the contents of the [www](./www) directory into the binary because we are using the `deploy` tag:

```shell
go build -tags deploy -o bin/example .
```

We can then run the example using the following command:

```shell
bin/example
```
