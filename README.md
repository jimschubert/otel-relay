# OTel Relay

A language-agnostic transparent proxy for debugging/viewing OpenTelemetry signals; not intended for production use.

```
📊 TRACE
├─ Resource:
│  ├─ service.name: my-service
│  ├─ service.version: 1.0.0
├─ 🔗 Span: database-query
│  ├─ Duration: 45.2ms
│  ├─ db.system: postgresql
│  └─ db.statement: SELECT * FROM users
```

Why would you use this?

1. You're just getting started and want to see what signals are being emitted to your collector, but don't want to
   configure a "real" collector.
2. You wanna see what attributes are actually being sent.
3. You want to see if your instrumentation is working at all, but you're not ready to emit them _somewhere_ (maybe
   verbose logging in the collector doesn't give you what you want).

What are some issues with just using otel-collector?

1. OTel Collector supports SIGHUP for reload (see [PR](https://github.com/open-telemetry/opentelemetry-collector/pull/6000)), but this will reload _everything_. There's no way to reload just OTLP receivers.
2. Any logging/debugging is mixed into other pipelines, which allows human error to misconfigure the collector on reload ([this blog](https://last9.io/blog/hot-reload-for-opentelemetry-collector/) has it right: "Always, and I mean ALWAYS, validate your config changes before reloading.").
3. It's designed for multiple exporters, usually signal-specific or target an external or system-wide sink which is not ideal for ad hoc debugging. Many useful exporters are in [opentelemetry-collector-contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/HEAD/exporter), adding complexity to the setup.
4. Enabling ad hoc evaluation of a single service on a system with mulitiple running services could get tricky.

**Note**: this is not meant to replace the collector. You can setup either in front of the collector or as a separate exporter in the collector.

## Installation

```bash
go install github.com/jimschubert/otel-relay/cmd@latest
```

Or build from source:

```bash
git clone https://github.com/jimschubert/otel-relay
cd otel-relay
go build -o otel-relay cmd/otel-relay/main.go
```

## Usage

Start the proxy server:

```bash
otel-relay
```

Point your app to `localhost:14317` (note the port starts with a `1`). You'll see telemetry data in real-time.

To forward to an actual collector (note the port _does not_ start with a `1` here):

```bash
otel-relay --upstream localhost:4317
```

See all attributes:

```bash
otel-relay --verbose
```

Emit formatted signals to stdout:

```bash
otel-relay --log
```

Emit formatted signals to a Unix domain socket:

```bash
otel-relay --socket /tmp/otel-relay.sock --emit
```

Change the listening port:

```bash
otel-relay --listen :9999
```

## Configuration

The relay is configured via command-line flags:

```
-l, --listen=":14317"                   Address to listen on for OTLP gRPC
-u, --upstream=<host:port>              Upstream OTLP collector address (optional)
    --[no-]log                          Whether to emit formatted signals to stdout
-s, --socket="/tmp/otel-relay.sock" Path to Unix domain socket to emit formatted signals on (optional)
    --[no-]emit                         Whether to emit formatted signals to unix socket
    --verbose                           Verbose output (show all attributes)
```

## OS Signals

The relay supports the following OS signals:
- `SIGINT` and `SIGTERM`: Gracefully shut down the server.
- `SIGUSR1`: Toggle verbose mode on/off, where verbose mode shows all attributes instead of a maximum of 5.
- `SIGUSR2`: Toggle logging to stdout on/off (disables "debugging" of messages, but still forwards if enabled).

To send a signal, use the `kill` command with the appropriate signal and the process ID of the relay.

For example, to toggle verbose mode:

```bash
# Find the process ID of otel-relay, not the daemon, socat, or inspector if running
ps aux | grep 'otel-relay'
kill -USR1 <pid>
```

## Emitted signals

If emitting signals via unix socket, you can view these either with:

```bash
socat - UNIX-CONNECT:/tmp/otel-relay.sock
```
Or with the provided inspector tool:

```bash
go build -o otel-inspector cmd/otel-inspector/main.go
./otel-inspector --socket /tmp/otel-relay.sock
```

## Examples

### Local Example

There's a working example in the `cmd/example/` directory. See [cmd/example/README.md](cmd/example/README.md).

### OpenTelemetry Full Demo

You can also check this out with the [OpenTelemetry Full Demo](https://opentelemetry.io/docs/demo/docker-deployment/), it just requires a little modification.
The demo can run with Docker or Kubernetes, but we'll use Docker for this example's instructions.

**1. Set ENV Overrides**
In `.env.override`, add the following to match the ports used by `otel-relay`:

```
OTEL_COLLECTOR_HOST=host.docker.internal
OTEL_COLLECTOR_PORT_GRPC=14317
OTEL_COLLECTOR_PORT_HTTP=14318
```

The `host.docker.internal` is a special DNS name in default docker networks. If this doesn't work for you, I'm assuming you know how to modify it.

**2. Modify docker-compose.yml**
In `docker-compose.yml`, remove these environment mappings from the `otel-collector` service:

```
- OTEL_COLLECTOR_HOST
- OTEL_COLLECTOR_PORT_GRPC
- OTEL_COLLECTOR_PORT_HTTP
```

And replace the `otel-collector` service's port mappings with the following to map to the host ports:

```
- "4317:4317"
- "4318:4318"
```

**3. Modify otel-collector config**

In `src/otel-collector/otelcol-config.yml`, find and replace:
* `${env:OTEL_COLLECTOR_HOST}` with `otel-collector`
* `${env:OTEL_COLLECTOR_PORT_GRPC}` with `4317`
* `${env:OTEL_COLLECTOR_PORT_HTTP}` with `4318`

**4. Start the demo**

```bash
make start # NOTE be sure to `make stop` when you're done.
````

**The result**

All services will emit to your locally running `otel-relay` instead of the compose environment's otel-collector.
The local `otel-relay` will forward to the collector as well, so you can see the full demo in action.

**Start otel-relay**

```bash
./otel-relay --listen=":14317" \
             --upstream=localhost:4317 \
             --listen-http=":14318" \
             --upstream-http="http://localhost:4318" \
             --emit \
             --log
```
This enables both the HTTP and GRPC forwarders, emitting them to the socket and logging to stdout. 
You can adjust these flags as needed. For example:

* disable logging, chnage `--log` to `--no-log`
* disable the local socket, change `--emit` to `--no-emit`

Note: if you start without logs or unix socket, you can't toggle them "on" at runtime (at least not yet).

## Acknowledgements

I was inspired after mentioning [Expedia's Haystack](https://github.com/ExpediaDotCom/haystack) to someone.
I used to work at Expedia and I _loved_ Haystack. We use OpenTelemetry where I work now, and its architecture is a lot 
like Haystack's. However, Haystack's stream-first design  meant you could "attach" viewers without ever needing to 
recompile core components or stop/start processing. I wanted  something like that for  OpenTelemetry, and this will aim to be that option.

I took additional inspirate from the OTel
collector's [debugexporter](https://github.com/open-telemetry/opentelemetry-collector/tree/main/exporter/debugexporter).
The initial intent of this project is to have a streaming view of OTel signals, so you can "peek" at the traffic
locally.

There are some Marshal functions in debugexporter which I may try to utilize later.

## License

Apache 2.0 - see [LICENSE](LICENSE)
