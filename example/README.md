# Example: generate traces

This example generates some OpenTelemetry traces to `otel-relay`.

## Run it…

In one terminal (from repo root):

```bash
go build -o otel-relay ./cmd
./otel-relay
```

This listens on `localhost:14317` by default and prints formatted traces to stdout.

In another terminal (in this directory):

```bash
go run main.go
```
This sends sample traces to the inspector.

Then, check out the traces in the first terminal. You'll see something like this:

```
📊 TRACE
├─ Resource:
│  ├─ environment: dev
│  ├─ service.name: otel-relay-example
│  └─ service.version: 1.0.0
├─ Scope: example-tracer
│
├─ 🔗 Span: database-query
│  ├─ TraceID: 6aab98ef96e805a391bb1b7e09aea220
│  ├─ SpanID: 511a5a65aeb928b9
│  ├─ ParentSpanID: b17273352285c46b
│  ├─ Kind: SPAN_KIND_INTERNAL
│  ├─ Duration: 16.077792ms
│  ├─ Status: STATUS_CODE_UNSET
│  ├─ Attributes:
│  │  ├─ db.system: postgresql
│  │  ├─ db.name: example_db
│  │  ├─ db.operation: SELECT
│  │  ├─ db.table: users
│  │  └─ db.rows.affected: 16
```

## Note on ports

The `otel-relay` port defaults to 14317. This differs from a "real" collector's `4317` to avoid conflicts.
You can start the relay on a different port:

```bash
./otel-relay --listen :9999
```

But this example doesn't have flags/switches, so you have to use the official OTel environment variable at startup to change the port:
```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:9999 go run main.go
```
