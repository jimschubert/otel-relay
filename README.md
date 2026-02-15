# OTel Relay

A language-agnostic transparent proxy for debugging/viewing OpenTelemetry signals.

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

Change the listening port:

```bash
otel-relay --listen :9999
```

## Configuration

The relay is configured via command-line flags:

```
--listen, -l    Address to listen on (default :14317)
--upstream, -u  Upstream collector address (optional)
--verbose       Show all attributes
```

## OS Signals

The relay supports the following OS signals:
- `SIGINT` and `SIGTERM`: Gracefully shut down the server.
- `SIGUSR1`: Toggle verbose mode on/off, where verbose mode shows all attributes instead of a maximum of 5.
- `SIGUSR2`: Toggle logging to stdout on/off (disables "debugging" of messages, but still forwards if enabled).

To send a signal, use the `kill` command with the appropriate signal and the process ID of the relay.

For example, to toggle verbose mode:

```bash
kill -USR1 $(pgrep -f 'otel-relay')
```

## Example

There's a working example in the `cmd/example/` directory. See [cmd/example/README.md](cmd/example/README.md).

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
