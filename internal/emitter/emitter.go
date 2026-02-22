package emitter

import (
	"context"
	"fmt"

	"github.com/jimschubert/otel-relay/proto/inspector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

type Emitter interface {
	EmitTrace(data proto.Message) error
	EmitMetric(data proto.Message) error
	EmitLog(data proto.Message) error
}

type grpcEmitter struct {
	socketPath string
	conn       *grpc.ClientConn
	client     inspector.InspectorServiceClient
}

func NewGrpcEmitter(socketPath string) Emitter {
	return &grpcEmitter{socketPath: socketPath}
}
func (e *grpcEmitter) EmitTrace(data proto.Message) error {
	if err := e.connect(); err != nil {
		return err
	}

	bytes, err := proto.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal trace: %w", err)
	}

	event := &inspector.TelemetryEvent{
		Data: bytes,
		Type: inspector.TelemetryType_TELEMETRY_TYPE_TRACE,
	}

	_, err = e.client.Emit(context.Background(), event)
	return err
}

func (e *grpcEmitter) EmitMetric(data proto.Message) error {
	if err := e.connect(); err != nil {
		return err
	}

	bytes, err := proto.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal metric: %w", err)
	}

	event := &inspector.TelemetryEvent{
		Data: bytes,
		Type: inspector.TelemetryType_TELEMETRY_TYPE_METRIC,
	}

	_, err = e.client.Emit(context.Background(), event)
	return err
}

func (e *grpcEmitter) EmitLog(data proto.Message) error {
	if err := e.connect(); err != nil {
		return err
	}

	bytes, err := proto.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal log: %w", err)
	}

	event := &inspector.TelemetryEvent{
		Data: bytes,
		Type: inspector.TelemetryType_TELEMETRY_TYPE_LOG,
	}

	_, err = e.client.Emit(context.Background(), event)
	return err
}

func (e *grpcEmitter) connect() error {
	if e.client != nil {
		return nil
	}

	conn, err := grpc.NewClient(
		"unix://"+e.socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC daemon: %w", err)
	}

	e.conn = conn
	e.client = inspector.NewInspectorServiceClient(conn)
	return nil
}
