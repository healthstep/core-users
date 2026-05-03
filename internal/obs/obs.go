// Package obs: root logger, trace_id, Prometheus go/process collectors.
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/porebric/logger"
	otrace "go.opentelemetry.io/otel/trace"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// L is the root app logger; set in Init.
var L *logger.Logger

func init() {
	_ = prometheus.DefaultRegisterer.Register(collectors.NewGoCollector())
	_ = prometheus.DefaultRegisterer.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

func Init(serviceName string) {
	L = logger.New(logger.InfoLevel).With("service", serviceName)
}

// TraceIDOrGen returns an OTel trace id from ctx, or a new 32-hex id.
func TraceIDOrGen(ctx context.Context) string {
	if sc := otrace.SpanContextFromContext(ctx); sc.IsValid() && sc.HasTraceID() {
		return sc.TraceID().String()
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// WithTrace attaches a trace_id to context for [logger.FromContext].
func WithTrace(ctx context.Context) context.Context {
	if L == nil {
		return ctx
	}
	tid := TraceIDOrGen(ctx)
	return logger.ToContext(ctx, L.With("trace_id", tid))
}

// BG returns a logger for non-request work (no distributed trace); still has trace_id for Loki.
func BG(component string) *logger.Logger {
	if L == nil {
		return logger.New(logger.InfoLevel)
	}
	return L.With("trace_id", "bg", "component", component)
}
