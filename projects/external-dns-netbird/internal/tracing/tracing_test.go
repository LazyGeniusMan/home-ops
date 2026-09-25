package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// exporterProvider installs a deterministic in-memory provider so tests
// assert spans without network or global state leakage.
func exporterProvider(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return exp
}

func TestSetupDisabled(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")
	shutdown, err := Setup(context.Background(), "test-svc", "dev")
	if err != nil {
		t.Fatalf("Setup disabled: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown disabled: %v", err)
	}
}

func TestMiddlewareBypassSkipsTracing(t *testing.T) {
	exp := exporterProvider(t)
	h := Middleware("test-svc", "/metrics")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if len(exp.GetSpans()) != 0 {
		t.Fatalf("bypassed path exported %d spans, want 0", len(exp.GetSpans()))
	}
}

func TestMiddlewareSpanNameUsesRoutePattern(t *testing.T) {
	exp := exporterProvider(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/notify", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := Middleware("test-svc", "/metrics")(mux)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/notify", nil))
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if got := spans[0].Name; got != "POST /notify" {
		t.Errorf("span name = %q, want %q", got, "POST /notify")
	}
}

func TestMiddlewareUnmatchedUsesNotFound(t *testing.T) {
	exp := exporterProvider(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/notify", func(w http.ResponseWriter, _ *http.Request) {})
	h := Middleware("test-svc")(mux)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/evil-path-12345", nil)
	// Undispatched: route falls back to the bounded literal.
	h.ServeHTTP(rec, req)
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if got := spans[0].Name; got != "GET notfound" {
		t.Errorf("span name = %q, want %q", got, "GET notfound")
	}
}

func TestTraceAttrsEmptyWithoutSpan(t *testing.T) {
	if attrs := TraceAttrs(context.Background()); attrs != nil {
		t.Errorf("TraceAttrs(background) = %v, want nil", attrs)
	}
}

func TestTraceAttrsPresentWithSpan(t *testing.T) {
	exporterProvider(t)
	ctx, span := otel.Tracer("test-svc").Start(context.Background(), "op")
	defer span.End()
	attrs := TraceAttrs(ctx)
	if len(attrs) != 2 {
		t.Fatalf("TraceAttrs = %v, want trace_id+span_id", attrs)
	}
}
