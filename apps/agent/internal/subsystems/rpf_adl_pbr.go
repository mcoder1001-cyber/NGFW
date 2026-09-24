package subsystems

// F-rpf-adl-pbr wiring (wave-A-hotspots A1: the registration and the descriptor names live here;
// subsystems.go carries one registration call and the names under this task's anchors):
//
//	interfaces  DF-2: urpf.interface, adl.interface, adl.allowlist (write-only, D-063/D-076)
//	routing     DF-2: abf.policy, abf.attach; agent-local: pbr.policy (policy name records)
//	services    auto-sdl.config (VPP-global: globals owner only, D-071; write-only)
//	(no domain) pbr.acl-ref: observe-only bridge that resolves abf.policy's acl.acl/<name> dependency
//	            until F-acl registers the acl.acl descriptor (then it switches itself off)
//
// Stores: every DF-2 family gets the persisted "acl" claim store (Wiring.KeyedClaims("acl"), the one
// F-acl also uses; never the in-memory default, D-080), the allow-list and Auto-SDL their applied-once
// records in the owner's BootStore, the policy name records <state dir>/pbr-<owner>.json. ABF policy
// ids are scoped to the slot range on the shared host (SlotIDRange, VRX_VPP_TABLE_BASE … +999) and
// unrestricted in the product agent.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	aclapi "ngfw/agent/binapi/acl"
	"ngfw/agent/internal/descriptors/abf"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/adl"
	autosdl "ngfw/agent/internal/descriptors/auto_sdl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/urpf"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names of the feature, per domain (the Domains entries in subsystems.go).
const (
	rpfAdlPbrURPF       = urpf.Name
	rpfAdlPbrADL        = adl.InterfaceName
	rpfAdlPbrADLAllow   = adl.AllowlistName
	rpfAdlPbrABFPolicy  = abf.PolicyName
	rpfAdlPbrABFAttach  = abf.AttachName
	rpfAdlPbrPolicyName = desired.PbrPolicyName
	rpfAdlPbrAutoSdl    = autosdl.Name
)

// rpfAdlPbrIDRange is the ABF policy id range of this agent: the slot's table range on the shared
// host (SlotIDRange: VRX_VPP_TABLE_BASE … +999), nil (every id) for the product agent.
func rpfAdlPbrIDRange() (*df2.IDRange, error) {
	r, err := SlotIDRange()
	if err != nil || r == nil {
		return nil, err
	}
	return &df2.IDRange{Lo: r.Lo, Hi: r.Hi}, nil
}

// rpfAdlPbrState is what the projection reads (desired.RpfAdlPbrEnv): process-wide, set by Register.
var rpfAdlPbrState struct {
	ids     atomic.Pointer[df2.IDRange]
	autoSdl atomic.Bool
	names   atomic.Pointer[pbrNames]
}

// RpfAdlPbrEnv returns what desired.RpfAdlPbr needs: the ABF policy id range, the ids already recorded
// for policy names (sticky: an existing policy keeps its id when other names come and go) and whether this
// agent applies Auto-SDL (the globals owner).
func RpfAdlPbrEnv() desired.RpfAdlPbrEnv {
	env := desired.RpfAdlPbrEnv{PolicyIDs: rpfAdlPbrState.ids.Load(), AutoSdl: rpfAdlPbrState.autoSdl.Load()}
	if n := rpfAdlPbrState.names.Load(); n != nil {
		env.RecordedIDs = n.ids()
	}
	return env
}

// registerACLBridge registers the pbr.acl-ref bridge unless DF-4's acl.acl is registered already (F-acl's
// registration ran first). When acl.acl is registered later, the bridge switches itself off at plan time
// (aclRefs.active). A registry without lookup cannot tell: the bridge then stays active, which is harmless
// (a registered acl.acl/<name> key always beats the bridge's alias), and it is logged.
func (w *Wiring) registerACLBridge(r scheduler.Registry) {
	reg, ok := r.(registryLookup)
	if !ok {
		w.env.Log.Warn("pbr.acl-ref: the registry cannot be queried; the ACL bridge stays active (a registered acl.acl still wins over its aliases)")
	} else if _, has := reg.Get(acl.NameACL); has {
		return
	}
	r.Register(&aclRefs{client: w.env.Client, owner: w.env.Owner, reg: reg})
}

// registerRpfAdlPbr registers the feature's descriptors with r.
func (w *Wiring) registerRpfAdlPbr(r scheduler.Registry) error {
	c, owner := w.env.Client, w.env.Owner
	claims, err := w.KeyedClaims("acl")
	if err != nil {
		return err
	}
	ids, err := rpfAdlPbrIDRange()
	if err != nil {
		return err
	}
	rpfAdlPbrState.ids.Store(ids)
	names, err := openPbrNames(filepath.Join(w.env.StateDir, "pbr-"+owner+".json"))
	if err != nil {
		return err
	}
	urpf.Register(r, c, owner, df2.WithClaims(claims))
	adl.RegisterWriteOnly(r, c, owner, adl.WithBootStore(w.boot), adl.WithAllowlistClaims(claims))
	adl.Register(r, c, owner, df2.WithClaims(claims))
	abf.Register(r, c, owner, ids, df2.WithClaims(claims))
	r.Register(names)
	rpfAdlPbrState.names.Store(names)
	w.registerACLBridge(r)
	rpfAdlPbrState.autoSdl.Store(w.env.GlobalsOwner)
	if w.env.GlobalsOwner {
		autosdl.Register(r, c, w.boot)
	} else {
		w.env.Log.Info("services.autoSdl is not applied by this agent: auto_sdl_config is a VPP-global and this agent is not the globals owner (D-071)")
	}
	return nil
}

// pbrNames is the agent-local pbr.policy descriptor: the name ↔ ABF policy id records of the PBR
// policies (VPP numbers policies and keeps no name), with the policy priority. The records are this
// agent's own configuration state — Retrieve returns what the store holds, like any other agent-local
// object; the ABF objects themselves are retrieved from VPP by abf.policy/abf.attach. A record depends
// on its abf.policy, so it is written after the policy exists and dropped before the policy goes.
type pbrNames struct {
	mu   sync.Mutex
	path string
	m    map[string]desired.PbrPolicyRecord
}

func openPbrNames(path string) (*pbrNames, error) {
	s := &pbrNames{path: path, m: map[string]desired.PbrPolicyRecord{}}
	raw, err := os.ReadFile(path) //nolint:gosec // the agent's own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("pbr name store %s: %w", path, err)
	}
	var recs []desired.PbrPolicyRecord
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &recs); err != nil {
			return nil, fmt.Errorf("pbr name store %s: %w", path, err)
		}
	}
	for _, r := range recs {
		s.m[r.Name] = r
	}
	return s, nil
}

func (s *pbrNames) save(m map[string]desired.PbrPolicyRecord) error {
	recs := make([]desired.PbrPolicyRecord, 0, len(m))
	for _, r := range m {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Name < recs[j].Name })
	raw, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	return df2.WriteFileAtomic(s.path, raw)
}

// ids returns the recorded name → policy id map (a copy).
func (s *pbrNames) ids() map[string]uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]uint32, len(s.m))
	for n, r := range s.m {
		out[n] = r.PolicyID
	}
	return out
}

func (s *pbrNames) put(r desired.PbrPolicyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]desired.PbrPolicyRecord, len(s.m)+1)
	for k, v := range s.m {
		next[k] = v
	}
	next[r.Name] = r
	if err := s.save(next); err != nil {
		return err
	}
	s.m = next
	return nil
}

var _ scheduler.Descriptor = (*pbrNames)(nil)

// Name implements scheduler.Descriptor.
func (*pbrNames) Name() string { return desired.PbrPolicyName }

func decodeRecord(obj proto.Message) desired.PbrPolicyRecord {
	r, _ := desired.DecodePbrPolicyRecord(obj)
	return r
}

// KeyOf implements scheduler.Descriptor.
func (*pbrNames) KeyOf(obj proto.Message) scheduler.Key {
	return desired.PbrPolicyKey(decodeRecord(obj).Name)
}

// Dependencies implements scheduler.Descriptor: the ABF policy the name stands for.
func (*pbrNames) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: desired.AbfPolicyKey(decodeRecord(obj).PolicyID)}}
}

func (s *pbrNames) apply(obj proto.Message) error {
	r, err := desired.DecodePbrPolicyRecord(obj)
	if err != nil {
		return err
	}
	if r.Name == "" {
		return dfkit.Specf("pbr.policy: empty name")
	}
	return s.put(r)
}

// Create implements scheduler.Descriptor.
func (s *pbrNames) Create(_ context.Context, obj proto.Message) (any, error) {
	return nil, s.apply(obj)
}

// Update implements scheduler.Descriptor (a new id or priority for the same name).
func (s *pbrNames) Update(_ context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, s.apply(newObj)
}

// Delete implements scheduler.Descriptor.
func (s *pbrNames) Delete(_ context.Context, obj proto.Message, _ any) error {
	name := decodeRecord(obj).Name
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[name]; !ok {
		return nil
	}
	next := make(map[string]desired.PbrPolicyRecord, len(s.m))
	for k, v := range s.m {
		if k != name {
			next[k] = v
		}
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.m = next
	return nil
}

// Retrieve implements scheduler.Descriptor: every record of the store.
func (s *pbrNames) Retrieve(context.Context) ([]scheduler.KV, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]scheduler.KV, 0, len(s.m))
	for _, r := range s.m {
		out = append(out, scheduler.KV{Key: desired.PbrPolicyKey(r.Name), Value: dfkit.Encode(r)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// registryLookup is the part of *scheduler.MapRegistry the ACL bridge needs.
type registryLookup interface {
	Get(name string) (scheduler.Descriptor, bool)
}

// aclRefsName is the observe-only ACL reference bridge (no domain: it only resolves dependencies).
const aclRefsName = "pbr.acl-ref"

// aclRefs bridges ABF's mandatory dependency acl.acl/<name> (DF-4's key, D-066) while no acl.acl
// descriptor is registered — F-acl registers it with the acl domain; until then nothing in this build
// would satisfy the reference and every PBR policy would fail with "dependency missing". It retrieves
// this owner's ACLs (tag "<owner>:<name>") and provides acl.acl/<name> as an alias; it is observe-only
// (never creates, changes or deletes an ACL) and switches itself off as soon as acl.acl is registered, so
// F-acl's real descriptor (which also sees ACL deletes) is the only source then.
type aclRefs struct {
	client vpp.Client
	owner  string
	reg    registryLookup
}

var (
	_ scheduler.Descriptor     = (*aclRefs)(nil)
	_ scheduler.KeyProvider    = (*aclRefs)(nil)
	_ scheduler.AbsenceDeleter = (*aclRefs)(nil)
)

// active reports whether the bridge is needed (no acl.acl descriptor registered).
func (d *aclRefs) active() bool {
	if d.reg == nil {
		return true
	}
	_, registered := d.reg.Get(acl.NameACL)
	return !registered
}

func (*aclRefs) Name() string { return aclRefsName }

func (*aclRefs) KeyOf(obj proto.Message) scheduler.Key {
	var ref struct {
		Name string `json:"name"`
	}
	_ = dfkit.Decode(obj, &ref)
	return scheduler.Join(aclRefsName, ref.Name)
}

func (*aclRefs) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// DeleteOnAbsence implements scheduler.AbsenceDeleter: observe-only.
func (*aclRefs) DeleteOnAbsence() bool { return false }

var errObserveOnly = errors.New("pbr.acl-ref is observe-only: ACLs are configured by F-acl (acl.lists)")

func (*aclRefs) Create(context.Context, proto.Message) (any, error) { return nil, errObserveOnly }

func (*aclRefs) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, errObserveOnly
}

func (*aclRefs) Delete(context.Context, proto.Message, any) error { return errObserveOnly }

// ProvidedKeys implements scheduler.KeyProvider: acl.acl/<name> while the bridge is active.
func (d *aclRefs) ProvidedKeys(obj proto.Message) []scheduler.Key {
	if !d.active() {
		return nil
	}
	var ref struct {
		Name string `json:"name"`
	}
	if err := dfkit.Decode(obj, &ref); err != nil || ref.Name == "" {
		return nil
	}
	return []scheduler.Key{acl.KeyACL(ref.Name)}
}

// Retrieve reports one reference per ACL name tagged by this owner (nothing once acl.acl is registered).
func (d *aclRefs) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if !d.active() {
		return nil, nil
	}
	stream, err := aclapi.NewServiceClient(d.client).ACLDump(ctx, &aclapi.ACLDump{ACLIndex: ^uint32(0)})
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	seen := map[string]bool{}
	var out []scheduler.KV
	for _, det := range details {
		name, ok := vpp.ParseOwnerTag(det.Tag, d.owner)
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		v := dfkit.Encode(struct {
			Name string `json:"name"`
		}{name})
		out = append(out, scheduler.KV{Key: scheduler.Join(aclRefsName, name), Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
