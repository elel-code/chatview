package interceptor

import (
	"context"
	"strings"

	"chatview/internal/contextx"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func AdminUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !strings.HasPrefix(info.FullMethod, "/chatview.admin.AdminService/") {
			return handler(ctx, req)
		}
		principal, ok := contextx.PrincipalFrom(ctx)
		if !ok || principal.Role != 1 {
			return nil, status.Error(codes.PermissionDenied, "admin required")
		}
		return handler(ctx, req)
	}
}
