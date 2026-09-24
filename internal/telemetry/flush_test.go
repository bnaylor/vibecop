package telemetry

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestForceFlushExportsBufferedSpans is the regression test for buffered
// telemetry lost on an unclean exit: a batch processor holds spans until its
// timer fires or Shutdown runs, and neither happens under SIGKILL. ForceFlush
// must push them out on demand, without tearing the provider down.
func TestForceFlushExportsBufferedSpans(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
	p := &Provider{tp: tp, tracer: tp.Tracer(InstrumentationName)}
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	_, span := p.StartPermissionSpan(context.Background(), "Bash", "abcd1234", "claude", "PreToolUse")
	span.End()

	if got := len(exp.GetSpans()); got != 0 {
		t.Fatalf("batch processor should still be buffering: got %d exported spans", got)
	}

	if err := p.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	if got := len(exp.GetSpans()); got != 1 {
		t.Errorf("after ForceFlush: got %d exported spans, want 1", got)
	}

	// Still usable afterwards — ForceFlush is not a teardown.
	_, span2 := p.StartPermissionSpan(context.Background(), "Read", "abcd1234", "claude", "PreToolUse")
	span2.End()
	if err := p.ForceFlush(context.Background()); err != nil {
		t.Fatalf("second ForceFlush: %v", err)
	}
	if got := len(exp.GetSpans()); got != 2 {
		t.Errorf("after second ForceFlush: got %d exported spans, want 2", got)
	}
}

func TestForceFlushNilProvider(t *testing.T) {
	var p *Provider
	if err := p.ForceFlush(context.Background()); err != nil {
		t.Errorf("nil ForceFlush should not error: %v", err)
	}
}
