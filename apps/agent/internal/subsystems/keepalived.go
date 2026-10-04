package subsystems

// F-vrrp-config-sync: the keepalived renderer stage (D-109 (d), as F-snmp's snmpd.config) — one singleton
// scheduler descriptor `keepalived.config` (key keepalived.config/ngfw, domain `ha`) wrapping RF-4's renderer
// with P12's linux-cp mapping as its InterfaceMapper (the mapping comes from the stage's own value, which
// carries the lcp leaf of every interface a keepalived instance uses — desired/vrrp.go).
//
//	Validate       TD-13 validator (S-keepalived-validator): render → `keepalived -t` on a staged copy in a
//	               private temp dir, read only; Stage() = StageDaemon (docs/agent/scheduler-validators.md)
//	Create/Update  Render → Validate (`keepalived -t`) → Apply (atomic write, SIGHUP, convergence) → record
//	Delete         the empty rendering (no vrrp_instance), record removed
//	Retrieve       the applied value while the live keepalived.conf is exactly its rendering (else nothing:
//	               drift → the next resync re-applies)
//
// The interface mapping is call-local: every render builds it from the value it renders (lcpmap.FromDesired)
// and passes it to keepalived.RenderWith — nothing shared is mutated (RV-A R4 n1), so Validate may run next to
// Retrieve and next to itself.
//
// Paths: the product paths, or with NGFW_TEST_PREFIX the slot's TestPaths (bin dir NGFW_KEEPALIVED_BIN_DIR,
// namespace NGFW_KEEPALIVED_NETNS) with a pidfile controller — a slot agent never touches /etc/keepalived or
// keepalived.service. No secret resolver (no API→agent secret channel, PENDING-secret-channel); the contract
// has no VRRP auth leaf yet (D-086 stand-ins not built), so nothing needs one.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/keepalived"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/scheduler"
)

// Environment of the keepalived stage (slot test agents).
const (
	EnvKeepalivedBinDir = "NGFW_KEEPALIVED_BIN_DIR"
	EnvKeepalivedNetNS  = "NGFW_KEEPALIVED_NETNS"
)

// KeepalivedStage is the keepalived renderer stage.
type KeepalivedStage struct {
	r      *keepalived.Renderer
	record string
}

var (
	_ scheduler.Descriptor = (*KeepalivedStage)(nil)
	_ scheduler.Validator  = (*KeepalivedStage)(nil)
	_ scheduler.Stager     = (*KeepalivedStage)(nil)
)

// NewKeepalivedStage builds the stage over r (whose own InterfaceMapper is never used: every render passes the
// value's mapping) with record as the applied-value file.
func NewKeepalivedStage(r *keepalived.Renderer, record string) *KeepalivedStage {
	return &KeepalivedStage{r: r, record: record}
}

// RecordsNoOwnership (TD-11b guard): the stage owns no VPP object; its only record is the applied value
// in the agent's state dir (written atomically, 0600).
func (*KeepalivedStage) RecordsNoOwnership() {}

// Name implements scheduler.Descriptor.
func (*KeepalivedStage) Name() string { return desired.KeepalivedConfigName }

// KeyOf implements scheduler.Descriptor.
func (*KeepalivedStage) KeyOf(proto.Message) scheduler.Key { return desired.KeepalivedKey }

// Dependencies implements scheduler.Descriptor.
func (*KeepalivedStage) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Stage implements scheduler.Stager: a daemon configuration, applied after the VPP objects of a transaction.
func (*KeepalivedStage) Stage() scheduler.Stage { return scheduler.StageDaemon }

// render builds keepalived.conf from v with a mapper local to this call (v's linux-cp pairs).
func (s *KeepalivedStage) render(ctx context.Context, v *ngfwv1.DesiredState) (renderers.Files, error) {
	m := lcpmap.FromDesired(v)
	return s.r.RenderWith(ctx, v, func(vpp string) (string, bool) { linux, ok := m[vpp]; return linux, ok })
}

// Validate implements scheduler.Validator: the value renders and `keepalived -t` accepts a staged copy of the
// rendering (private temp dir, removed again). Nothing else: no daemon file, no reload, no record. The copy
// carries `dynamic_interfaces` because the validator runs before the VPP stage creates the linux-cp pairs of the
// same transaction; Create re-checks the exact rendering once they exist. Secrets in the error are masked by the
// renderer (no VRRP auth leaf exists yet, so today there are none).
func (s *KeepalivedStage) Validate(ctx context.Context, _ scheduler.Key, value proto.Message, _ scheduler.ReadOnlyView) error {
	v, ok := value.(*ngfwv1.DesiredState)
	if !ok {
		return fmt.Errorf("%s: unexpected value %T", desired.KeepalivedConfigName, value)
	}
	files, err := s.render(ctx, v)
	if err != nil {
		return err
	}
	if err := s.r.Check(ctx, files, keepalived.CheckOptions{DynamicInterfaces: true}); err != nil {
		return err
	}
	return keepalivedRunningFinding(s.r.CheckRunning(ctx))
}

func (s *KeepalivedStage) apply(ctx context.Context, v *ngfwv1.DesiredState) error {
	files, err := s.render(ctx, v)
	if err != nil {
		return err
	}
	if err := s.r.Validate(ctx, files); err != nil {
		return err
	}
	if err := keepalivedRunningFinding(s.r.CheckRunning(ctx)); err != nil {
		return err
	}
	return keepalivedRunningFinding(s.r.Apply(ctx, files))
}

// Create implements scheduler.Descriptor.
func (s *KeepalivedStage) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*ngfwv1.DesiredState)
	if !ok {
		return nil, fmt.Errorf("%s: unexpected value %T", desired.KeepalivedConfigName, obj)
	}
	if err := s.apply(ctx, v); err != nil {
		return nil, err
	}
	return nil, s.saveRecord(v)
}

// Update implements scheduler.Descriptor.
func (s *KeepalivedStage) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return s.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: the empty rendering.
func (s *KeepalivedStage) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if err := s.apply(ctx, &ngfwv1.DesiredState{}); err != nil {
		return err
	}
	if err := os.Remove(s.record); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (s *KeepalivedStage) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	raw, err := os.ReadFile(s.record)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := &ngfwv1.DesiredState{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, v); err != nil {
		return nil, fmt.Errorf("keepalived record %s: %w", s.record, err)
	}
	files, err := s.render(ctx, v)
	if err != nil {
		return nil, nil //nolint:nilerr // no longer renders: reported absent, the next apply re-renders
	}
	live, err := os.ReadFile(s.r.Paths().ConfFile)
	if err != nil || !bytes.Equal(live, files[s.r.Paths().ConfFile].Content) {
		return nil, nil //nolint:nilerr // drift: reported absent, the next resync re-applies
	}
	return []scheduler.KV{{Key: desired.KeepalivedKey, Value: v}}, nil
}

func (s *KeepalivedStage) saveRecord(v *ngfwv1.DesiredState) error {
	raw, err := protojson.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.record), 0o700); err != nil {
		return err
	}
	return atomicWrite(s.record, raw)
}

// keepalivedPaths are the slot's test paths with a pidfile controller (NGFW_TEST_PREFIX set), else the
// product paths. keepalivedGate registers the stage only with a prefix or NGFW_VPP_ID_RANGE=all, so the
// product paths (and keepalived.service) are reached only by the product agent on a box of its own.
func keepalivedPaths() (keepalived.Paths, []keepalived.Option) {
	prefix := os.Getenv(EnvTestPrefix)
	if prefix == "" {
		return keepalived.ProductPaths(), nil
	}
	bin := os.Getenv(EnvKeepalivedBinDir)
	if bin == "" {
		bin = filepath.Dir(keepalived.NotifyHelperProduct)
	}
	p := keepalived.TestPaths(prefix, bin, os.Getenv(EnvKeepalivedNetNS))
	ctl := &rfkit.ProcessController{PID: rfkit.PIDFile(filepath.Join(filepath.Dir(p.ConfFile), "keepalived.pid")), Binary: keepalived.KeepalivedBin}
	return p, []keepalived.Option{keepalived.WithController(ctl)}
}

// registerKeepalived registers the keepalived stage (called by registerVrrp).
func registerKeepalived(r scheduler.Registry, w *Wiring) {
	runner := renderers.NewSystemRunner(renderers.NewAllowlist(keepalived.Binaries()...))
	paths, opts := keepalivedPaths()
	opts = append(opts, keepalived.WithPaths(paths))
	renderer := keepalived.New(runner, opts...)
	keepalivedRuntime.Store(renderer)
	r.Register(NewKeepalivedStage(renderer, filepath.Join(w.env.StateDir, "keepalived-"+w.env.Owner+".json")))
}

func keepalivedRunningFinding(err error) error {
	if errors.Is(err, rfkit.ErrNotRunning) {
		return fmt.Errorf("keepalived is not running for this agent; slot harnesses start it: %w", err)
	}
	return err
}
