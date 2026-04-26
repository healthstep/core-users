package middleware

import (
	"context"

	"github.com/helthtech/core-users/internal/obs"
	"github.com/porebric/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// GRPCUnaryAccessLog logs unary gRPC: request/response JSON (capped) and errors; trace_id on context.
func GRPCUnaryAccessLog() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		ctx = obs.WithTrace(ctx)
		var remote string
		if pr, ok := peer.FromContext(ctx); ok && pr != nil {
			remote = pr.Addr.String()
		}
		reqS := protoToJSON(req)
		logger.Info(ctx, "grpc request", "method", info.FullMethod, "client", remote, "request", reqS)
		resp, err := h(ctx, req)
		respS := protoToJSON(resp)
		if err != nil {
			logger.Error(ctx, err, "grpc error", "method", info.FullMethod, "response", respS)
			return resp, err
		}
		logger.Info(ctx, "grpc response", "method", info.FullMethod, "response", respS)
		return resp, err
	}
}

func protoToJSON(msg any) string {
	if msg == nil {
		return "null"
	}
	p, ok := msg.(proto.Message)
	if !ok {
		return "{}"
	}
	b, e := protojson.Marshal(p)
	if e != nil {
		return `{"_marshal_error":true}`
	}
	s := string(b)
	const max = 8000
	if len(s) > max {
		return s[:max] + `...truncated`
	}
	return s
}
