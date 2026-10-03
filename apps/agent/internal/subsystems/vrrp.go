package subsystems

// F-vrrp-config-sync: domain `ha` — DF-7's VPP vrrp descriptors (engine "vpp"), the agent-local vrrp.meta
// record (names VPP cannot hold) and RF-4's keepalived renderer stage (engine "keepalived", keepalived.go).
//
//	ownership (TD-11b): a VR belongs to the owner of its interface (DF-7: re-resolved on every call, D-071);
//	                    the applied-once records of the df7 families live in the owner's BootStore
//	                    (df7.SetBootStore, installed by register); vrrp.meta is a persisted file store.
//	id range (TD-8b):   VRRP allocates no VPP id (a VR is keyed by interface + VRID + family), so no
//	                    df7.WithIDRange option is passed.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// HA is the `ha` domain.
const HA = "ha"

// Shared-host engine gates (RV-A R4 M1/M2, V22b, D-087/D-090). The VPP VRRP engine writes vrrp_vr_*
// on the shared /run/vpp/api.sock and the keepalived engine writes a daemon config and reloads it —
// neither is safe on the shared dev host outside a manager window. The owner name proves nothing here:
// NGFW_OWNER defaults to "ngfw", which is also the tools/app agent on the shared host (review S-rva R1 B1).
// Only NGFW_VPP_ID_RANGE=all (the product agent on a box of its own) turns an engine on by default.
const (
	// EnvVrrpVPP gates the VPP VRRP engine: "on" | "off". Unset → on only with NGFW_VPP_ID_RANGE=all.
	// "on" is the explicit opt-in of a manager window (VPP idle, V22b).
	EnvVrrpVPP = "NGFW_VRRP_VPP"
	// EnvKeepalived gates the keepalived engine: "on" | "off". Unset or "on" → on for a lab slot
	// (NGFW_TEST_PREFIX set: that slot's TestPaths and a pidfile controller) or with NGFW_VPP_ID_RANGE=all
	// (ProductPaths + keepalived.service); any other agent stays off — "on" without either is refused
	// with a warning, it never falls through to /etc/keepalived and systemctl.
	EnvKeepalived = "NGFW_KEEPALIVED"
)

// vrrpVPPEnabled / keepalivedEnabled are the projection's view of the gates, set at registration (one
// agent per process; the latest registration wins, like frrEnabled). VrrpEnv() reads them.
var (
	vrrpVPPEnabled    atomic.Bool
	keepalivedEnabled atomic.Bool
)

// VrrpEnv returns the ha.vrrp engine gates for the projection (desired.Vrrp). It reflects the values
// resolved by the latest registerVrrp of this process.
func VrrpEnv() desired.VrrpOptions {
	return desired.VrrpOptions{VPPEngine: vrrpVPPEnabled.Load(), Keepalived: keepalivedEnabled.Load()}
}

// vrrpVPPGate resolves NGFW_VRRP_VPP (value v) for an agent whose id scope is all (NGFW_VPP_ID_RANGE=all).
func vrrpVPPGate(v string, all bool) (bool, error) {
	switch v = strings.TrimSpace(v); v {
	case "on":
		return true, nil
	case "off":
		return false, nil
	case "":
		return all, nil
	default:
		return false, fmt.Errorf("%s=%q: want on or off", EnvVrrpVPP, v)
	}
}

// keepalivedGate resolves NGFW_KEEPALIVED (value v) given NGFW_TEST_PREFIX (prefix) and the id scope. The
// stage is on only where keepalivedPaths is safe: a slot prefix (TestPaths) or NGFW_VPP_ID_RANGE=all
// (ProductPaths). refused is set when "on" was asked for without either (the caller logs a warning).
func keepalivedGate(v, prefix string, all bool) (on, refused bool, err error) {
	safe := strings.TrimSpace(prefix) != "" || all
	switch v = strings.TrimSpace(v); v {
	case "off":
		return false, false, nil
	case "":
		return safe, false, nil
	case "on":
		return safe, !safe, nil
	default:
		return false, false, fmt.Errorf("%s=%q: want on or off", EnvKeepalived, v)
	}
}

// vrrpDescriptors are the descriptors of the `ha` domain.
var vrrpDescriptors = []string{vrrp.NameVR, vrrp.NamePeers, vrrp.NameTrack, vrrp.NameState, desired.VrrpMetaName, desired.KeepalivedConfigName}

func init() { Domains[HA] = append(Domains[HA], vrrpDescriptors...) }

// registerVrrp registers the vrrp family (tolerant Retrieve on a VPP without the vrrp plugin), vrrp.meta
// over <state dir>/vrrp-meta-<owner>.json and the keepalived stage.
func registerVrrp(r scheduler.Registry, w *Wiring) error {
	c, owner := w.env.Client, w.env.Owner
	vppEngine, err := vrrpVPPGate(os.Getenv(EnvVrrpVPP), w.env.IDs.All)
	if err != nil {
		return err
	}
	keepEngine, refused, err := keepalivedGate(os.Getenv(EnvKeepalived), os.Getenv(EnvTestPrefix), w.env.IDs.All)
	if err != nil {
		return err
	}
	if refused {
		w.env.Log.Warn(EnvKeepalived + "=on refused: no " + EnvTestPrefix + " (slot TestPaths) and not " + EnvIDRange + "=" + IDRangeAll +
			" — this agent never writes /etc/keepalived nor reloads keepalived.service; the keepalived engine stays off")
	}
	vrrpVPPEnabled.Store(vppEngine)
	keepalivedEnabled.Store(keepEngine)
	if vppEngine {
		if lookup, ok := r.(interface {
			Get(string) (scheduler.Descriptor, bool)
		}); ok {
			if descriptor, ok := lookup.Get(core.InterfaceAddrName); ok {
				if hook, ok := descriptor.(interface {
					SetVirtualAddressSource(core.VirtualAddressSource)
				}); ok {
					hook.SetVirtualAddressSource(vrrp.OwnedVirtualAddresses)
				}
			}
		}
		// The VPP vrrp descriptors and the vrrp.meta store are only registered when the engine is on:
		// with the engine off nothing dumps or writes vrrp_vr_* on the shared VPP, and the projection
		// reports engine-vpp instances as configured-but-not-applied (desired.Vrrp / VrrpOptions).
		// Turning the engine off does not remove VRs it created earlier: they stay in VPP until the engine
		// is on again and a commit drops them (docs/agent/renderers/keepalived.md, "Shared-host engine gates").
		vrrp.Register(vrrpRegistry{r, c}, c, owner, df7.WithInterfaceKey(dfkit.DefaultInterfaceKey))
		store, err := newVrrpMetaFile(filepath.Join(w.env.StateDir, "vrrp-meta-"+owner+".json"))
		if err != nil {
			return fmt.Errorf("vrrp: %w", err)
		}
		r.Register(desired.NewVrrpMeta(store))
	} else {
		w.env.Log.Info("VPP VRRP engine off (" + EnvVrrpVPP + "=off, or unset without " + EnvIDRange + "=" + IDRangeAll + "): ha.vrrp engine=vpp instances are reported unapplied, never written to VPP")
	}
	if keepEngine {
		registerKeepalived(r, w)
	} else {
		w.env.Log.Info("keepalived engine off (" + EnvKeepalived + "=off, or neither a lab slot prefix nor " + EnvIDRange + "=" + IDRangeAll + "): ha.vrrp engine=keepalived instances are reported unapplied, never staged")
	}
	return nil
}

// vrrpRegistry decorates every vrrp descriptor: Retrieve on a VPP without the vrrp plugin reports no
// objects instead of failing the Retrieve / plan of every domain (same rule as F-lisp / F-tunnels).
type vrrpRegistry struct {
	scheduler.Registry
	c vpp.Client
}

func (g vrrpRegistry) Register(d scheduler.Descriptor) {
	g.Registry.Register(vrrpTolerant{Descriptor: d, c: g.c})
}

type vrrpTolerant struct {
	scheduler.Descriptor
	c vpp.Client
}

// Unwrap exposes the decorated descriptor (dfkit/persist walks the chain).
func (t vrrpTolerant) Unwrap() scheduler.Descriptor { return t.Descriptor }

// Retrieve implements scheduler.Descriptor.
func (t vrrpTolerant) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	kvs, err := t.Descriptor.Retrieve(ctx)
	if err == nil || scheduler.IsRetrieveUnsupported(err) {
		return kvs, err
	}
	if errors.Is(err, df7.ErrPluginNotLoaded) {
		return nil, nil
	}
	if h, ok := t.c.(interface{ Handles(string) bool }); ok && !h.Handles("vrrp_vr_dump") {
		return nil, nil
	}
	return kvs, err
}

// vrrpMetaFile is the persisted desired.VrrpMetaStore (one JSON file, written atomically).
type vrrpMetaFile struct {
	mu   sync.Mutex
	path string
}

type vrrpMetaDoc struct {
	Version int                    `json:"version"`
	Entries []desired.VrrpMetaSpec `json:"entries"`
}

func newVrrpMetaFile(path string) (*vrrpMetaFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f := &vrrpMetaFile{path: path}
	if _, err := f.Load(); err != nil {
		return nil, err
	}
	return f, nil
}

// Persistent marks the store as surviving an agent restart (dfkit/persist).
func (*vrrpMetaFile) Persistent() bool { return true }

// Load implements desired.VrrpMetaStore.
func (f *vrrpMetaFile) Load() (map[string]desired.VrrpMetaSpec, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]desired.VrrpMetaSpec{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s store: %w", desired.VrrpMetaName, err)
	}
	var doc vrrpMetaDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s store %s: %w", desired.VrrpMetaName, f.path, err)
	}
	m := make(map[string]desired.VrrpMetaSpec, len(doc.Entries))
	for _, e := range doc.Entries {
		m[e.ID] = e
	}
	return m, nil
}

// Save implements desired.VrrpMetaStore.
func (f *vrrpMetaFile) Save(m map[string]desired.VrrpMetaSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	doc := vrrpMetaDoc{Version: 1, Entries: make([]desired.VrrpMetaSpec, 0, len(m))}
	for _, e := range m {
		doc.Entries = append(doc.Entries, e)
	}
	sort.Slice(doc.Entries, func(i, j int) bool { return doc.Entries[i].ID < doc.Entries[j].ID })
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(f.path, append(raw, '\n'))
}
