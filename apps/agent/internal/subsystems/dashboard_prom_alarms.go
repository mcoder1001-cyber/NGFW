package subsystems

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"io"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/promexport"
	"ngfw/agent/internal/scheduler"
	"os"
	"sync"
)

// PrometheusStage retrieves only the live listener owned by this process.
// Restarted agents recreate it from persisted desired state, never stale records.
type PrometheusStage struct {
	mu       sync.Mutex
	listener promexport.Listener
	source   promexport.StatsSource
	value    *ngfwv1.ManagementPrometheus
}

// NewPrometheusStage creates an external listener descriptor.
func NewPrometheusStage(source promexport.StatsSource) *PrometheusStage {
	return &PrometheusStage{source: source}
}

// Name identifies the listener descriptor.
func (*PrometheusStage) Name() string { return desired.PrometheusDescriptorName }

// KeyOf returns the singleton key.
func (*PrometheusStage) KeyOf(proto.Message) scheduler.Key { return desired.PrometheusKey }

// Dependencies returns no VPP dependencies.
func (*PrometheusStage) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create starts the listener and saves its applied value.
func (s *PrometheusStage) Create(ctx context.Context, obj proto.Message) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v, ok := obj.(*ngfwv1.ManagementPrometheus)
	if !ok || v == nil || !v.GetEnabled() {
		return nil, fmt.Errorf("enabled ManagementPrometheus required")
	}
	addr, err := desired.PrometheusAddress(v)
	if err != nil {
		return nil, err
	}
	allow, err := promexport.ParseAllow(v.GetAllow())
	if err != nil {
		return nil, err
	}
	if err := s.listener.Start(addr, promexport.NewHandler(s.source, "", allow)); err != nil {
		return nil, err
	}
	s.value = proto.Clone(v).(*ngfwv1.ManagementPrometheus)
	return nil, nil
}

// Update atomically replaces the handler or binds the replacement endpoint.
func (s *PrometheusStage) Update(ctx context.Context, _ proto.Message, next proto.Message, _ any) (any, error) {
	return s.Create(ctx, next)
}

// Delete closes the listener.
func (s *PrometheusStage) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.Close()
	return nil
}

// Retrieve returns the listener actually owned by this agent.
func (s *PrometheusStage) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.value == nil || s.listener.Addr() == "" {
		return nil, nil
	}
	return []scheduler.KV{{Key: desired.PrometheusKey, Value: proto.Clone(s.value)}}, nil
}

// Close stops the listener and clears its applied value.
func (s *PrometheusStage) Close() { s.mu.Lock(); defer s.mu.Unlock(); s.listener.Stop(); s.value = nil }
func registerDashboardPromAlarms(r scheduler.Registry, w *Wiring) error {
	source := promexport.NewGovppSource(os.Getenv(EnvStatsSocket))
	stage := NewPrometheusStage(source)
	if err := w.AddMetricsCollector(MetricsCollector{Name: "dashboard-prom-alarms", Collect: func(ctx context.Context, out io.Writer) error { return promexport.Collect(ctx, source, "", out) }}); err != nil {
		return err
	}
	r.Register(stage)
	// Closers run in reverse order: terminate the shared stats source first so
	// active/queued loopback and external scrapes cannot reopen it, then stop HTTP.
	w.OnClose(stage.Close)
	w.OnClose(source.Stop)
	return nil
}

// RecordsNoOwnership declares that the listener owns no VPP objects or persisted claims.
func (*PrometheusStage) RecordsNoOwnership() {}

// Stage orders the listener after VPP operations.
func (*PrometheusStage) Stage() scheduler.Stage { return scheduler.StageDaemon }
