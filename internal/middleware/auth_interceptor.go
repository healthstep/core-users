package middleware

import (
	"context"
	"strings"

	"github.com/helthtech/core-users/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	UserTypeKey contextKey = "user_type"
)

func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(UserIDKey).(string)
	return v
}

func AuthUnaryInterceptor(jwtSvc *service.JWTService) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return handler(ctx, req)
		}

		authVals := md.Get("authorization")
		if len(authVals) == 0 {
			return handler(ctx, req)
		}

		token := authVals[0]
		token = strings.TrimPrefix(token, "Bearer ")
		token = strings.TrimPrefix(token, "bearer ")

		claims, err := jwtSvc.Validate(token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token")
		}

		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UserTypeKey, claims.UserType)
		return handler(ctx, req)
	}
}

func PanicRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = status.Errorf(codes.Internal, "panic: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}
