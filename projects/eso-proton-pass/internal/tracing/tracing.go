// Package tracing wires OpenTelemetry traces: W3C propagation on inbound
// requests, OTLP/HTTP export to the infra otel-gateway collector, and trace_id /
// span_id slog correlation. Prometheus /metrics is untouched.
package tracing

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// DefaultEndpoint is the infra otel-gateway OTLP/HTTP collector.
const DefaultEndpoint = "http://otel-gateway-collector.otel-collectors.svc:4318"

// Setup installs the global tracer provider (W3C TraceContext+Baggage
// propagation, parent-based always-sample, batch OTLP/HTTP export) for
// serviceName at serviceVersion. OTEL_SERVICE_NAME wins over serviceName;
// the endpoint comes from OTEL_EXPORTER_OTLP_ENDPOINT and defaults to
// DefaultEndpoint. OTEL_SDK_DISABLED=true leaves the globals untouched and
// returns a no-op shutdown so tests and local runs stay dependency-free.
func Setup(ctx context.Context, serviceName, serviceVersion string) (func(context.Context) error, error) {
	if name := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); name != "" {
		serviceName = name
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true") {
		return func(context.Context) error { return nil }, nil
	}
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion),
		),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

// Extract returns ctx with the inbound W3C trace context (TraceContext +
// Baggage) for services that manage their own server spans.
func Extract(r *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
}

// Middleware starts one server span per request and names it from the
// matched mux pattern (r.Pattern, "notfound" when unmatched), so span names
// share the bounded route labels used by metrics and logs. Bypassed paths
// (e.g. /metrics, /healthz) skip tracing entirely.
func Middleware(tracerName string, bypass ...string) func(http.Handler) http.Handler {
	skip := make(map[string]struct{}, len(bypass))
	for _, b := range bypass {
		skip[b] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := skip[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}
			ctx, span := otel.Tracer(tracerName).Start(Extract(r), "HTTP "+r.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("http.method", r.Method)),
			)
			defer span.End()
			// In-place: downstream handlers (and r.Pattern after mux
			// dispatch) share this pointer, so logs correlate and the span
			// name below sees the matched pattern.
			*r = *r.WithContext(ctx)
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			route := r.Pattern
			if route == "" {
				route = "notfound"
			}
			name := r.Method + " " + route
			if strings.HasPrefix(route, r.Method+" ") {
				name = route
			}
			span.SetName(name)
			span.SetAttributes(
				attribute.String("http.route", route),
				attribute.Int("http.status_code", rec.status),
			)
			if rec.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(rec.status))
			}
		})
	}
}

// TraceAttrs returns trace_id/span_id slog attrs for ctx, or nil when the
// request carries no valid span (untraced bypass paths, unit tests).
func TraceAttrs(ctx context.Context) []any {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []any{
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	}
}

// statusRecorder captures the status code for span attributes.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
