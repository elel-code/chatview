package interceptor

import (
	"context"
	"log/slog"
	"time"

	"chatview/server/internal/contextx"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func LoggingUnary(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		logRPC(ctx, logger, info.FullMethod, time.Since(start), err)
		return resp, err
	}
}

func LoggingStream(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, stream)
		logRPC(stream.Context(), logger, info.FullMethod, time.Since(start), err)
		return err
	}
}

func logRPC(ctx context.Context, logger *slog.Logger, method string, elapsed time.Duration, err error) {
	principal, _ := contextx.PrincipalFrom(ctx)
	attrs := []any{
		"method", method,
		"duration_ms", elapsed.Milliseconds(),
		"code", status.Code(err).String(),
	}
	if principal.PubKey != "" {
		attrs = append(attrs, "pub_key", principal.PubKey)
	}
	if err != nil {
		attrs = append(attrs, "error", err)
		logger.Warn("grpc request completed", attrs...)
		return
	}
	logger.Info("grpc request completed", attrs...)
}
