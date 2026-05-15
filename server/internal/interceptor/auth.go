package interceptor

import (
	"context"
	"strings"

	"chatview/internal/contextx"
	"chatview/internal/db"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Auth struct {
	Store *db.Store
}

func (a Auth) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		principal, err := a.authenticate(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		return handler(contextx.WithPrincipal(ctx, principal), req)
	}
}

func (a Auth) Stream() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if isPublicMethod(info.FullMethod) {
			return handler(srv, stream)
		}
		principal, err := a.authenticate(stream.Context())
		if err != nil {
			return status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		return handler(srv, &wrappedStream{
			ServerStream: stream,
			ctx:          contextx.WithPrincipal(stream.Context(), principal),
		})
	}
}

func (a Auth) authenticate(ctx context.Context) (contextx.Principal, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return contextx.Principal{}, status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		values = md.Get("Authorization")
	}
	if len(values) == 0 {
		return contextx.Principal{}, status.Error(codes.Unauthenticated, "missing authorization")
	}
	return a.Store.AuthenticateToken(ctx, values[0])
}

func isPublicMethod(method string) bool {
	return strings.HasPrefix(method, "/chatview.auth.AuthService/")
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}
