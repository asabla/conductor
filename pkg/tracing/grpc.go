package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// grpcTracer is the tracer for gRPC operations.
var grpcTracer = otel.Tracer("conductor/grpc")

// UnaryServerInterceptor returns a gRPC unary server interceptor for tracing.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract trace context from incoming metadata
		ctx = extractTraceContext(ctx)

		// Start span
		ctx, span := grpcTracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
				attribute.String("rpc.service", extractServiceName(info.FullMethod)),
			),
		)
		defer span.End()

		// Call handler
		resp, err := handler(ctx, req)

		// Record result
		if err != nil {
			recordGRPCError(span, err)
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for tracing.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Extract trace context from incoming metadata
		ctx := extractTraceContext(ss.Context())

		// Start span
		ctx, span := grpcTracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
				attribute.String("rpc.service", extractServiceName(info.FullMethod)),
				attribute.Bool("rpc.stream.client", info.IsClientStream),
				attribute.Bool("rpc.stream.server", info.IsServerStream),
			),
		)
		defer span.End()

		// Wrap stream with traced context
		wrappedStream := &tracedServerStream{
			ServerStream: ss,
			ctx:          ctx,
			span:         span,
		}

		// Call handler
		err := handler(srv, wrappedStream)

		// Record result
		if err != nil {
			recordGRPCError(span, err)
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// UnaryClientInterceptor returns a gRPC unary client interceptor for tracing.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Start span
		ctx, span := grpcTracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.service", extractServiceName(method)),
				attribute.String("net.peer.name", cc.Target()),
			),
		)
		defer span.End()

		// Inject trace context into outgoing metadata
		ctx = injectTraceContext(ctx)

		// Call invoker
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		// Record result
		span.SetAttributes(attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())))
		if err != nil {
			recordGRPCError(span, err)
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// StreamClientInterceptor returns a gRPC stream client interceptor for tracing.
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Start span
		ctx, span := grpcTracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.service", extractServiceName(method)),
				attribute.String("net.peer.name", cc.Target()),
				attribute.Bool("rpc.stream.client", desc.ClientStreams),
				attribute.Bool("rpc.stream.server", desc.ServerStreams),
			),
		)

		// Inject trace context into outgoing metadata
		ctx = injectTraceContext(ctx)

		// Call streamer
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			recordGRPCError(span, err)
			span.End()
			return nil, err
		}

		// Wrap stream to track completion
		return &tracedClientStream{
			ClientStream: stream,
			span:         span,
		}, nil
	}
}

// tracedServerStream wraps grpc.ServerStream with tracing context.
type tracedServerStream struct {
	grpc.ServerStream
	ctx  context.Context
	span trace.Span
}

// Context returns the wrapped context.
func (s *tracedServerStream) Context() context.Context {
	return s.ctx
}

// SendMsg traces sent messages.
func (s *tracedServerStream) SendMsg(m interface{}) error {
	err := s.ServerStream.SendMsg(m)
	if err != nil {
		s.span.AddEvent("message.sent.error", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	} else {
		s.span.AddEvent("message.sent")
	}
	return err
}

// RecvMsg traces received messages.
func (s *tracedServerStream) RecvMsg(m interface{}) error {
	err := s.ServerStream.RecvMsg(m)
	if err != nil {
		s.span.AddEvent("message.received.error", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	} else {
		s.span.AddEvent("message.received")
	}
	return err
}

// tracedClientStream wraps grpc.ClientStream with tracing.
type tracedClientStream struct {
	grpc.ClientStream
	span trace.Span
}

// SendMsg traces sent messages.
func (s *tracedClientStream) SendMsg(m interface{}) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		s.span.AddEvent("message.sent.error", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	} else {
		s.span.AddEvent("message.sent")
	}
	return err
}

// RecvMsg traces received messages.
func (s *tracedClientStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.span.AddEvent("message.received.error", trace.WithAttributes(
			attribute.String("error", err.Error()),
		))
	} else {
		s.span.AddEvent("message.received")
	}
	return err
}

// CloseSend ends the span when closing.
func (s *tracedClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	s.span.End()
	return err
}

// extractTraceContext extracts trace context from incoming gRPC metadata.
func extractTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	// Use OTel propagator to extract trace context
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, &metadataCarrier{md: md})
}

// injectTraceContext injects trace context into outgoing gRPC metadata.
func injectTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	} else {
		md = md.Copy()
	}

	// Use OTel propagator to inject trace context
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, &metadataCarrier{md: md})

	return metadata.NewOutgoingContext(ctx, md)
}

// metadataCarrier adapts gRPC metadata to OTel TextMapCarrier.
type metadataCarrier struct {
	md metadata.MD
}

// Get returns the value for a key.
func (c *metadataCarrier) Get(key string) string {
	values := c.md.Get(key)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// Set sets the value for a key.
func (c *metadataCarrier) Set(key, value string) {
	c.md.Set(key, value)
}

// Keys returns all keys.
func (c *metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c.md))
	for k := range c.md {
		keys = append(keys, k)
	}
	return keys
}

// recordGRPCError records a gRPC error on the span.
func recordGRPCError(span trace.Span, err error) {
	st, _ := status.FromError(err)
	span.SetAttributes(attribute.String("rpc.grpc.status_code", st.Code().String()))
	span.RecordError(err)

	if st.Code() != grpccodes.OK {
		span.SetStatus(codes.Error, st.Message())
	}
}

// extractServiceName extracts the service name from a gRPC method.
func extractServiceName(fullMethod string) string {
	if len(fullMethod) > 0 && fullMethod[0] == '/' {
		fullMethod = fullMethod[1:]
	}
	for i := 0; i < len(fullMethod); i++ {
		if fullMethod[i] == '/' {
			return fullMethod[:i]
		}
	}
	return fullMethod
}
