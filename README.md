# OTel Inspector

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

## Installation

```bash
go install github.com/jimschubert/otel-inspector/cmd@latest
```

Or build from source:

```bash
git clone https://github.com/jimschubert/otel-inspector
cd otel-inspector
go build -o otel-inspector cmd/main.go
```

## Usage

Start the proxy server:

```bash
otel-inspector
```

Point your app to `localhost:14317` (note the port starts with a `1`). You'll see telemetry data in real-time.

To forward to an actual collector (note the port _does not_ start with a `1` here):

```bash
otel-inspector --upstream localhost:4317
```

See all attributes:

```bash
otel-inspector --verbose
```

Change the listening port:

```bash
otel-inspector --listen :9999
```

## Configuration

The inspector is configured via command-line flags:

```
--listen, -l    Address to listen on (default :14317)
--upstream, -u  Upstream collector address (optional)
--verbose       Show all attributes
```

## Example

There's a working example in the `example/` directory. See [example/README.md](example/README.md).

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
