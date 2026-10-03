package subsystems

// F-vrrp-config-sync: the keepalived renderer stage (D-109 (d), as F-snmp's snmpd.config) — one singleton
// scheduler descriptor `keepalived.config` (key keepalived.config/ngfw, domain `ha`) wrapping RF-4's renderer
// with P12's linux-cp mapping as its InterfaceMapper (the mapping comes from the stage's own value, which
// carries the lcp leaf of every interface a keepalived instance uses — desired/vrrp.go).
//
//	Create/Update  Render → Validate (`keepalived -t`) → Apply (atomic write, SIGHUP, convergence) → record
//	Delete         the empty rendering (no vrrp_instance), record removed
//	Retrieve       the applied value while the live keepalived.conf is exactly its rendering (else nothing:
//	               drift → the next resync re-applies)
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
	mapper *lcpmap.Mapper
	record string
}

var _ scheduler.Descriptor = (*KeepalivedStage)(nil)

// NewKeepalivedStage builds the stage; mapper must be the renderer's InterfaceMapper.
func NewKeepalivedStage(r *keepalived.Renderer, mapper *lcpmap.Mapper, record string) *KeepalivedStage {
	return &KeepalivedStage{r: r, mapper: mapper, record: record}
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

func (s *KeepalivedStage) render(ctx context.Context, v *ngfwv1.DesiredState) (renderers.Files, error) {
	s.mapper.Set(lcpmap.FromDesired(v))
	return s.r.Render(ctx, v)
}

func (s *KeepalivedStage) apply(ctx context.Context, v *ngfwv1.DesiredState) error {
	files, err := s.render(ctx, v)
	if err != nil {
		return err
	}
	if err := s.r.Validate(ctx, files); err != nil {
		return err
	}
	return s.r.Apply(ctx, files)
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

// keepalivedPaths are the product paths, or the slot's test paths with a pidfile controller.
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
	mapper := &lcpmap.Mapper{}
	opts = append(opts, keepalived.WithPaths(paths), keepalived.WithInterfaceMapper(mapper.Map))
	r.Register(NewKeepalivedStage(keepalived.New(runner, opts...), mapper, filepath.Join(w.env.StateDir, "keepalived-"+w.env.Owner+".json")))
}
