package subsystems

// F-snmp: the snmpd renderer stage (D-109 (d)) — one singleton scheduler descriptor `snmpd.config`
// (key snmpd.config/vrx, domain `services`) wrapping RF-4's renderer, plus the VRX-MIB AgentX subagent
// as background work owned by that descriptor (started when an enabled configuration is applied,
// stopped by Delete). Choice logged in docs/status/tasks/F-snmp.md (options: this singleton / a shared
// renderer stage in the agent core / an API-side commit hook).
//
//	Create/Update  Render → Validate (daemon parse run) → Apply (atomic write, SIGHUP, convergence)
//	Delete         the renderer's disabled rendering (answers nobody), subagent stopped
//	Retrieve       the applied services.snmp while the live snmpd.conf is exactly its rendering
//	               (else nothing: drift → the next resync re-applies)
//
// A restart request (startup-only directive changed, D-079) or a stopped daemon is not a failure: the
// file is written, the request is persisted by the renderer and shown by GET /api/v1/state/snmp
// (`pendingAction`); the agent never restarts snmpd on its own (unit control is P10's).
//
// Secrets: communities and USM passphrases are D-051 refs. No API→agent secret channel exists yet
// (docs/decisions/PENDING-secret-channel.md); until it does, refs resolve through a slot-local fixture
// file (VRX_SNMP_FIXTURE_SECRETS, 0600, values `VRX_TEST_PSK_F-snmp_*` only) and are refused otherwise.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/renderers/snmpd"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/snmpagent"
)

// Environment of the snmpd stage.
const (
	// EnvSnmpFixtureSecrets names the slot-local fixture secret file (JSON {"password/<n>": "VRX_TEST_PSK_F-snmp_…"}).
	EnvSnmpFixtureSecrets = "VRX_SNMP_FIXTURE_SECRETS" //nolint:gosec // env var name, not a credential
	// EnvTestPrefix selects snmpd.TestPaths(<prefix>) and a pidfile controller: a slot agent never
	// touches /etc/snmp or the system snmpd unit.
	EnvTestPrefix = "VRX_TEST_PREFIX"
	// FixtureSecretPrefix is the only value prefix the fixture resolver accepts.
	FixtureSecretPrefix = "VRX_TEST_PSK_F-snmp_" //nolint:gosec // fixture prefix, not a credential
)

// ErrNoSecretChannel is returned for every secret ref while no channel exists.
var ErrNoSecretChannel = errors.New("no API→agent secret channel yet (PENDING-secret-channel): SNMP communities and passphrases cannot be resolved by this agent build")

// SnmpFixtureResolver resolves refs from a 0600 JSON file of fixture values.
func SnmpFixtureResolver(path string) rfkit.SecretResolver {
	return rfkit.SecretResolverFunc(func(_ context.Context, ref string) (string, error) {
		if path == "" {
			return "", ErrNoSecretChannel
		}
		st, err := os.Stat(path) //nolint:gosec // operator-configured path
		if err != nil {
			return "", fmt.Errorf("fixture secrets: %w", err)
		}
		if st.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("fixture secrets %s must be 0600", path)
		}
		raw, err := os.ReadFile(path) //nolint:gosec // operator-provided slot fixture
		if err != nil {
			return "", err
		}
		m := map[string]string{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return "", fmt.Errorf("fixture secrets %s: not a JSON object", path)
		}
		v, ok := m[ref]
		if !ok {
			return "", fmt.Errorf("fixture secrets: no value for %s", ref)
		}
		if !strings.HasPrefix(v, FixtureSecretPrefix) {
			return "", fmt.Errorf("fixture secrets: the value of %s is not a %s* test literal", ref, FixtureSecretPrefix)
		}
		return v, nil
	})
}

// SnmpStage is the snmpd renderer stage: the descriptor plus the state GET /state/snmp reads.
type SnmpStage struct {
	r      *snmpd.Renderer
	owner  string // registration key (registerSnmp); "" = unregistered (tests)
	record string // applied services.snmp (protojson; refs only, never values)
	log    *slog.Logger
	source snmpagent.Source

	mu        sync.Mutex
	validated []byte // content of the last file that passed the parse run
	pending   string // last ActionRequired, redacted
	sub       *snmpagent.Subagent
	subStop   context.CancelFunc
	subDone   chan struct{}
}

var _ scheduler.Descriptor = (*SnmpStage)(nil)

// NewSnmpStage builds the stage. record is the file of the applied value.
func NewSnmpStage(r *snmpd.Renderer, record string, source snmpagent.Source, log *slog.Logger) *SnmpStage {
	if log == nil {
		log = slog.Default()
	}
	return &SnmpStage{r: r, record: record, source: source, log: log}
}

// RecordsNoOwnership (TD-11b guard): the stage owns no VPP object; its only record is the applied
// services.snmp in the agent's state dir (written atomically, 0600, refs only).
func (*SnmpStage) RecordsNoOwnership() {}

// Name implements scheduler.Descriptor.
func (s *SnmpStage) Name() string { return desired.SnmpDescriptorName }

// KeyOf implements scheduler.Descriptor.
func (s *SnmpStage) KeyOf(proto.Message) scheduler.Key { return desired.SnmpKey }

// Dependencies implements scheduler.Descriptor.
func (s *SnmpStage) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func docOf(v *vrxv1.SnmpService) *vrxv1.DesiredState {
	return &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Snmp: v}}
}

// Check is the projection's pre-transaction check (D-125): render + parse run, cached by content.
func (s *SnmpStage) Check(v *vrxv1.SnmpService) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s.renderValidated(ctx, v)
	return err
}

func (s *SnmpStage) renderValidated(ctx context.Context, v *vrxv1.SnmpService) (renderers.Files, error) {
	files, err := s.r.Render(ctx, docOf(v))
	if err != nil {
		return nil, err
	}
	content := files[s.r.Paths().ConfFile].Content
	s.mu.Lock()
	ok := bytes.Equal(content, s.validated)
	s.mu.Unlock()
	if ok {
		return files, nil
	}
	if err := s.r.Validate(ctx, files); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.validated = append([]byte(nil), content...)
	s.mu.Unlock()
	return files, nil
}

func (s *SnmpStage) apply(ctx context.Context, v *vrxv1.SnmpService) error {
	files, err := s.renderValidated(ctx, v)
	if err != nil {
		return err
	}
	err = s.r.Apply(ctx, files)
	var ar *rfkit.ActionRequired
	switch {
	case errors.As(err, &ar):
		msg := s.r.Redact(ar.Error())
		s.log.Warn("snmpd configuration written; the daemon needs an explicit action", "action", ar.Action, "reason", s.r.Redact(ar.Reason))
		s.mu.Lock()
		s.pending = msg
		s.mu.Unlock()
	case err != nil:
		return err // the renderer redacts its own errors
	default:
		s.mu.Lock()
		s.pending = ""
		s.mu.Unlock()
	}
	for _, w := range snmpd.Warnings(files, s.r.Paths()) {
		s.log.Warn("snmpd", "warning", w)
	}
	if err := s.saveRecord(v); err != nil {
		return err
	}
	s.subagent(v.GetEnabled() && (v.GetSubagent() == nil || v.GetSubagent().Enabled == nil || v.GetSubagent().GetEnabled()))
	return nil
}

// Create implements scheduler.Descriptor.
func (s *SnmpStage) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, ok := obj.(*vrxv1.SnmpService)
	if !ok {
		return nil, fmt.Errorf("snmpd.config: unexpected value %T", obj)
	}
	return nil, s.apply(ctx, v)
}

// Update implements scheduler.Descriptor.
func (s *SnmpStage) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return s.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: the disabled rendering.
func (s *SnmpStage) Delete(ctx context.Context, _ proto.Message, _ any) error {
	s.subagent(false)
	if err := s.apply(ctx, &vrxv1.SnmpService{Enabled: proto.Bool(false)}); err != nil {
		return err
	}
	if err := os.Remove(s.record); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (s *SnmpStage) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	v, err := s.loadRecord()
	if err != nil || v == nil || !v.GetEnabled() {
		return nil, err
	}
	files, err := s.r.Render(ctx, docOf(v))
	if err != nil {
		s.log.Warn("snmpd: the applied configuration no longer renders; reporting it absent", "err", err)
		return nil, nil
	}
	live, err := os.ReadFile(s.r.Paths().ConfFile)
	if err != nil || !bytes.Equal(live, files[s.r.Paths().ConfFile].Content) {
		s.log.Warn("snmpd.conf differs from the applied configuration (drift); reporting it absent")
		return nil, nil
	}
	return []scheduler.KV{{Key: desired.SnmpKey, Value: v}}, nil
}

func (s *SnmpStage) saveRecord(v *vrxv1.SnmpService) error {
	raw, err := protojson.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.record), 0o700); err != nil {
		return err
	}
	tmp := s.record + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.record)
}

func (s *SnmpStage) loadRecord() (*vrxv1.SnmpService, error) {
	raw, err := os.ReadFile(s.record)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := &vrxv1.SnmpService{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, v); err != nil {
		return nil, fmt.Errorf("snmpd record %s: %w", s.record, err)
	}
	return v, nil
}

// subagent starts or stops the VRX-MIB subagent.
func (s *SnmpStage) subagent(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on == (s.subStop != nil) || s.source == nil {
		return
	}
	if !on {
		s.subStop()
		<-s.subDone
		s.subStop, s.subDone, s.sub = nil, nil, nil
		s.log.Info("VRX-MIB subagent stopped")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.sub = &snmpagent.Subagent{Socket: s.r.Paths().AgentXSocket, Source: s.source, Log: s.log}
	s.subStop, s.subDone = cancel, make(chan struct{})
	go func(sub *snmpagent.Subagent, done chan struct{}) {
		defer close(done)
		sub.Run(ctx)
	}(s.sub, s.subDone)
	s.log.Info("VRX-MIB subagent started", "socket", s.r.Paths().AgentXSocket)
}

// Close stops the subagent and unregisters the stage (agent shutdown).
func (s *SnmpStage) Close() {
	s.subagent(false)
	if s.owner != "" {
		desired.SetSnmpCheck(s.owner, nil)
		snmpStages.CompareAndDelete(s.owner, s)
	}
}

// SnmpStateView is what GET /api/v1/state/snmp shows (no secret: credentials by name only).
type SnmpStateView struct {
	Configured    bool              `json:"configured"`
	Daemon        *snmpd.State      `json:"daemon,omitempty"`
	EngineID      string            `json:"engineId,omitempty"`
	PendingAction string            `json:"pendingAction,omitempty"`
	Subagent      *snmpagent.Status `json:"subagent,omitempty"`
}

// State reads the daemon state (Retrieve of the renderer) and the subagent status.
func (s *SnmpStage) State(ctx context.Context) (*SnmpStateView, error) {
	v, err := s.loadRecord()
	if err != nil {
		return nil, err
	}
	out := &SnmpStateView{Configured: v.GetEnabled(), EngineID: v.GetEngineId()}
	if st, err := s.r.State(ctx); err == nil {
		out.Daemon = st
	}
	s.mu.Lock()
	out.PendingAction = s.pending
	if s.sub != nil {
		st := s.sub.Status()
		out.Subagent = &st
	}
	s.mu.Unlock()
	return out, nil
}

// ---- wiring --------------------------------------------------------------------------------

var snmpStages sync.Map // owner → *SnmpStage

// SnmpStageOf returns the registered stage of an owner (rpc_snmp.go).
func SnmpStageOf(owner string) (*SnmpStage, bool) {
	v, ok := snmpStages.Load(owner)
	if !ok {
		return nil, false
	}
	return v.(*SnmpStage), true
}

// snmpPaths are the product paths, or the slot's test paths with a pidfile controller.
func snmpPaths(_ renderers.Runner) (snmpd.Paths, []snmpd.Option) {
	prefix := os.Getenv(EnvTestPrefix)
	if prefix == "" {
		return snmpd.ProductPaths(), nil
	}
	p := snmpd.TestPaths(prefix)
	ctl := &rfkit.ProcessController{PID: rfkit.PIDFile(filepath.Join(filepath.Dir(p.ConfFile), "snmpd.pid")), Binary: snmpd.SnmpdBin}
	return p, []snmpd.Option{snmpd.WithController(ctl)}
}

// registerSnmp registers the snmpd stage (one line in register(), wave-BC: F-snmp).
func registerSnmp(r scheduler.Registry, w *Wiring) {
	runner := renderers.NewSystemRunner(renderers.NewAllowlist(snmpd.Binaries()...))
	paths, opts := snmpPaths(runner)
	opts = append(opts, snmpd.WithPaths(paths), snmpd.WithSecretResolver(SnmpFixtureResolver(os.Getenv(EnvSnmpFixtureSecrets))))
	rend := snmpd.New(runner, opts...)
	src := &snmpagent.ProductSource{
		StateFile: filepath.Join(w.env.StateDir, "agent-state.json"),
		Version:   AgentVersion,
		Connected: w.env.Client.Connected,
		Status: func(ctx context.Context) (map[uint32]snmpagent.IfStatus, error) {
			t, err := iface.Dump(ctx, w.env.Client, w.env.Owner)
			if err != nil {
				return nil, err
			}
			out := map[uint32]snmpagent.IfStatus{}
			for _, idx := range t.Indexes() {
				d, ok := t.Details(idx)
				if !ok {
					continue
				}
				out[idx] = ifStatus(t.VPPName(idx), d)
			}
			return out, nil
		},
	}
	st := NewSnmpStage(rend, filepath.Join(w.env.StateDir, "snmpd-"+w.env.Owner+".json"), src, w.env.Log)
	r.Register(st)
	st.owner = w.env.Owner
	desired.SetSnmpCheck(w.env.Owner, st.Check)
	snmpStages.Store(w.env.Owner, st)
}

func ifStatus(name string, d *ifapi.SwInterfaceDetails) snmpagent.IfStatus {
	return snmpagent.IfStatus{
		Name:    name,
		AdminUp: d.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0,
		OperUp:  d.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0,
	}
}

// AgentVersion is reported as vrxAgentVersion (set by the agent's main through -ldflags when it has one).
var AgentVersion = "dev"
