package subsystems

// P12 wiring (wave-A-hotspots A1): subsystems.go carries one registration line and the descriptor names in
// Domains[Interfaces] (lcp.itf-pair) and Domains[Routing] (frr.config) under its anchors; everything else is here.
//
// The FRR stage (P12-questions Q3, D-109 d): one singleton scheduler object `frr.config/vrx` whose value is the
// FRR-relevant subset of the document (desired.FRRDoc). Create/Update render it with every registered RF-1 section
// (bgp, policy, lcp-addresses, later ospf/isis/…) → `vtysh -C` → `frr-reload.py --reload` + convergence check; Delete
// applies the framework-only configuration; Retrieve reports the last applied document with its status from
// `frr-reload.py --test`. The FRR→VPP route sync is linux-nl's (AD-2, P12-questions Q6): this agent programs no route
// FRR learns. A 1 Hz poller publishes EVENT_KIND_BGP_NEIGHBOR_CHANGED / EVENT_KIND_ROUTING_CHANGED while FRR carries
// this agent's configuration.
//
// Which FRR this agent drives: the product agent (owner "vrx") → /etc/frr, /var/run/frr (no pathspace);
// VRX_FRR_PATHSPACE=<slot prefix> → that slot's frrtest instance (/run/vrx-test/<p>/frr, pathspace <p>: topology tests);
// otherwise, or with VRX_FRR=off, none. An agent without FRR projects no frr.config object: FRR content in the document
// is an agent.unsupported-field warning, as before P12 (unit tests, slots that do not run FRR). Before any vtysh call
// the runtime checks that a vty socket exists, so an agent whose FRR is down never spawns vtysh.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/bgp"
	_ "ngfw/agent/internal/renderers/frr/policy" // the policy section (init)
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names of P12 (Domains[Interfaces], Domains[Routing]).
const (
	lcpItfPairName = lcp.NameItfPair
	frrConfigName  = desired.FRRConfigName
)

// EnvFRR = "off" disables FRR for every owner.
const EnvFRR = "VRX_FRR"

// EnvFRRPathspace names a slot's frrtest instance for a non-product agent (see the file comment).
const EnvFRRPathspace = "VRX_FRR_PATHSPACE"

// ErrFRRUnavailable is returned when the configuration needs FRR and this agent has none.
var ErrFRRUnavailable = errors.New("frr: FRR is not available to this agent")

var (
	slotOwnerRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,5}$`)
	versionRe   = regexp.MustCompile(`FRRouting ([0-9][0-9A-Za-z.~+-]*)`)
)

// frrPaths returns the FRR paths for owner, ok=false when this agent drives no FRR.
func frrPaths(owner string) (frr.Paths, bool) {
	ps := os.Getenv(EnvFRRPathspace)
	switch {
	case os.Getenv(EnvFRR) == "off":
		return frr.Paths{}, false
	case ps != "" && slotOwnerRe.MatchString(ps):
		return frr.TestPaths(ps), true
	case owner == "vrx" && ps == "":
		return frr.ProductPaths(), true
	}
	return frr.Paths{}, false
}

// frrEnabled is set when a registered runtime drives an FRR (the projection's switch; one agent per process in the
// product and in topology tests).
var frrEnabled atomic.Bool

// FRR is one agent's FRR runtime: the renderer (with the linux-cp mapper), the last applied document and the event
// poller. The reconciler serialises Create/Update/Delete; Retrieve and the state readers may run concurrently.
type FRR struct {
	owner   string
	client  vpp.Client
	log     *slog.Logger
	publish func(*vrxv1.Event)
	paths   frr.Paths
	enabled bool
	mapper  *lcpmap.Mapper
	r       *frr.Renderer

	mu        sync.Mutex
	last      *vrxv1.DesiredState // last applied document (nil: none since start, or removed)
	lastFiles renderers.Files

	pollOnce sync.Once
	stop     chan struct{}
	stopOnce sync.Once
}

var frrRuntimes sync.Map // owner → *FRR

// FRRRuntime returns the FRR runtime of owner's agent (nil before Register).
func FRRRuntime(owner string) *FRR {
	if v, ok := frrRuntimes.Load(owner); ok {
		return v.(*FRR)
	}
	return nil
}

// FRRProjection returns the options of the FRR builder (desired.FRROptions): the D-072 selector, no secret resolver
// (PENDING-secret-channel) and the renderer's pure Render as the check, so DryRun reports what FRR would refuse to
// render before anything is applied.
func FRRProjection() desired.FRROptions {
	return desired.FRROptions{
		Selector: func(i int, sr *vrxv1.StaticRoute) bool { return frr.StaticOwnedByFRR(i, sr, nil) },
		Check:    CheckFRR,
		Disabled: !frrEnabled.Load(),
	}
}

// CheckFRR renders doc without applying it: every registered section and interface-line producer, the linux-cp
// mapping of doc's own pairs, no secret resolver. Render does no I/O (the runner is never called, the paths are only
// file names).
func CheckFRR(doc *vrxv1.DesiredState) error {
	m := &lcpmap.Mapper{}
	m.Set(lcpmap.FromDesired(doc))
	_, err := frr.New(renderers.NewRecordingRunner(), frr.WithInterfaceMapper(m.Map)).Render(context.Background(), doc)
	return err
}

func newFRR(env Env, runner renderers.Runner, opts ...frr.Option) *FRR {
	paths, ok := frrPaths(env.Owner)
	return newFRRAt(env, runner, paths, ok, opts...)
}

// newFRRAt builds a runtime for explicit paths (tests: a temporary directory and a recording runner).
func newFRRAt(env Env, runner renderers.Runner, paths frr.Paths, ok bool, opts ...frr.Option) *FRR {
	if env.Log == nil {
		env.Log = slog.Default()
	}
	rt := &FRR{owner: env.Owner, client: env.Client, log: env.Log.With("component", "frr"), publish: env.Publish,
		paths: paths, enabled: ok, mapper: &lcpmap.Mapper{}, stop: make(chan struct{})}
	if ok {
		base := []frr.Option{frr.WithPaths(paths), frr.WithInterfaceMapper(rt.mapper.Map)}
		rt.r = frr.New(runner, append(base, opts...)...)
	}
	return rt
}

// registerP12 registers DF-8's lcp.itf-pair (for `interfaces.<n>.lcp`) and the FRR stage descriptor.
func registerP12(r scheduler.Registry, w *Wiring) {
	r.Register(&tapGatedPairs{ItfPairDescriptor: lcp.NewItfPair(w.env.Client, w.env.Owner, lcp.WithInterfaceKey(dfkit.DefaultInterfaceKey)), client: w.env.Client, owner: w.env.Owner})
	env := w.env
	env.Publish = w.Publish // TD-8 event seam (A5): the agent's bus, nil-safe
	rt := newFRR(env, frr.NewSystemRunner())
	if old, ok := frrRuntimes.Swap(w.env.Owner, rt); ok {
		old.(*FRR).Close() // a re-registration (an agent restarted in-process) stops the previous poller
	}
	frrEnabled.Store(rt.enabled) // the latest registration wins: one agent per process (tests re-register)
	r.Register(&frrConfigDescriptor{rt: rt})
}

// Close stops the runtime's poller (tests; the product agent's runtime lives as long as the process).
func (rt *FRR) Close() { rt.stopOnce.Do(func() { close(rt.stop) }) }

// Enabled reports whether this agent drives an FRR instance.
func (rt *FRR) Enabled() bool { return rt != nil && rt.enabled }

// Paths returns the FRR paths of this agent.
func (rt *FRR) Paths() frr.Paths { return rt.paths }

// Renderer returns the FRR renderer (nil when FRR is off).
func (rt *FRR) Renderer() *frr.Renderer { return rt.r }

// Running reports whether FRR answers for this agent: a vty socket exists (no process is spawned to find out).
func (rt *FRR) Running() bool {
	if !rt.Enabled() {
		return false
	}
	for _, d := range []string{"zebra", "mgmtd", "bgpd"} {
		if _, err := os.Stat(filepath.Join(rt.paths.SocketDir(), d+".vty")); err == nil {
			return true
		}
	}
	return false
}

// render renders doc with the mapping of doc's own linux-cp pairs.
func (rt *FRR) render(ctx context.Context, doc *vrxv1.DesiredState) (renderers.Files, error) {
	rt.mapper.Set(lcpmap.FromDesired(doc))
	return rt.r.Render(ctx, doc)
}

// apply renders, validates and applies doc (nil = the framework-only configuration, which forgets the last document).
func (rt *FRR) apply(ctx context.Context, doc *vrxv1.DesiredState) error {
	remove := doc == nil
	if !rt.Enabled() {
		return fmt.Errorf("%w (owner %q, %s=%q)", ErrFRRUnavailable, rt.owner, EnvFRR, os.Getenv(EnvFRR))
	}
	if !rt.Running() {
		return fmt.Errorf("%w: no FRR vty socket in %s (is FRR running?)", ErrFRRUnavailable, rt.paths.SocketDir())
	}
	if doc == nil {
		doc = &vrxv1.DesiredState{}
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	files, err := rt.render(ctx, doc)
	if err != nil {
		return err
	}
	if err := rt.r.Validate(ctx, files); err != nil {
		return err
	}
	if err := rt.r.Apply(ctx, files); err != nil {
		return err
	}
	if remove {
		rt.last, rt.lastFiles = nil, nil
	} else {
		rt.last, rt.lastFiles = proto.Clone(doc).(*vrxv1.DesiredState), files
		rt.startPoller()
	}
	rt.log.Info("FRR configuration applied", "conf", rt.paths.ConfFile(), "bgp", doc.GetRouting().GetBgp() != nil)
	return nil
}

// retrieve reports the frr.config object: the last applied document with its status, an "unknown" object when FRR
// holds configuration of its own since this agent started, or nothing.
func (rt *FRR) retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if !rt.Enabled() {
		return nil, nil
	}
	rt.mu.Lock()
	last, files := rt.last, rt.lastFiles
	rt.mu.Unlock()
	if !rt.Running() {
		if last == nil {
			return nil, nil
		}
		return []scheduler.KV{{Key: desired.FRRConfigKey, Value: desired.FRRValue(last, desired.FRRUnreachable)}}, nil
	}
	if last == nil {
		// after an agent restart: FRR may still run a configuration applied by an earlier run of this agent
		out, err := rt.r.Show(ctx, frr.ShowRunningConfig)
		if err != nil {
			return nil, nil // FRR answers nothing useful: nothing is reported (a Create re-applies)
		}
		if hasOwnContent(string(out)) {
			return []scheduler.KV{{Key: desired.FRRConfigKey, Value: desired.FRRValue(&vrxv1.DesiredState{}, desired.FRRUnknown)}}, nil
		}
		return nil, nil
	}
	diff, err := rt.r.DryRun(ctx, files)
	switch {
	case err != nil:
		return []scheduler.KV{{Key: desired.FRRConfigKey, Value: desired.FRRValue(last, desired.FRRUnreachable)}}, nil
	case diff != "":
		rt.log.Warn("FRR running configuration drifted from the applied one", "diff", diff)
		return []scheduler.KV{{Key: desired.FRRConfigKey, Value: desired.FRRValue(last, desired.FRRDrift)}}, nil
	}
	return []scheduler.KV{{Key: desired.FRRConfigKey, Value: desired.FRRValue(last, desired.FRRApplied)}}, nil
}

// hasOwnContent reports whether a running configuration holds anything beyond the framework's globals.
func hasOwnContent(running string) bool {
	for _, l := range frr.NormalizeConfig(running) {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "frr defaults"), strings.HasPrefix(t, "hostname "), strings.HasPrefix(t, "log "),
			t == "service integrated-vtysh-config", strings.HasPrefix(t, "no ip forwarding"), strings.HasPrefix(t, "no ipv6 forwarding"):
			continue
		}
		return true
	}
	return false
}

// ---- lcp.itf-pair --------------------------------------------------------------------------------

// tapGatedPairs is DF-8's lcp.itf-pair with one read saved: VPP's end of a linux-cp pair is a tap/tun interface
// ("tap4096"…, lcp_interface.c auto_id_offset 4096), so while VPP has no such interface there is no pair and
// Retrieve answers from sw_interface_dump alone, without lcp_itf_pair_get (a VPP without linux_cp, or none in use).
type tapGatedPairs struct {
	*lcp.ItfPairDescriptor
	client vpp.Client
	owner  string
}

// CheckPersistent declares that the pairs record ownership (TD-11b): a pair on an untagged interface (a DPDK NIC) is
// ours through DF-1's claim store (dfkit Target.Claim), which the product wiring persists (iface.SetClaimStore with
// IfaceClaims). The store must say it survives an agent restart.
func (d *tapGatedPairs) CheckPersistent() error {
	if p, ok := iface.Claims(d.owner).(interface{ Persistent() bool }); ok && p.Persistent() {
		return nil
	}
	return fmt.Errorf("%s: claims on untagged interfaces of owner %s are not persisted (install a persisted store with iface.SetClaimStore)", lcpItfPairName, d.owner)
}

func (d *tapGatedPairs) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := dfkit.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	for _, i := range ifs.ByIndex {
		if strings.HasPrefix(i.Name, "tap") || strings.HasPrefix(i.Name, "tun") {
			return d.ItfPairDescriptor.Retrieve(ctx)
		}
	}
	return nil, nil
}

// ---- the frr.config descriptor ------------------------------------------------------------------

type frrConfigDescriptor struct{ rt *FRR }

var _ scheduler.Descriptor = (*frrConfigDescriptor)(nil)

// RecordsNoOwnership declares (TD-11b) that the FRR stage keeps no claim or boot record: it owns exactly one object,
// FRR's configuration of this agent's instance, found by its fixed key.
func (*frrConfigDescriptor) RecordsNoOwnership() {}

func (*frrConfigDescriptor) Name() string                      { return frrConfigName }
func (*frrConfigDescriptor) KeyOf(proto.Message) scheduler.Key { return desired.FRRConfigKey }

// Dependencies implements scheduler.Descriptor: none (review H1). A dependency on the linux-cp pairs — hard or optional —
// makes the scheduler recreate this singleton around every pair change: its Delete applies the framework-only
// configuration, which drops every BGP session and every tap address (and, with linux-nl listening in the taps' netns,
// the VPP interface addresses). zebra and bgpd accept configuration for an interface that appears later and apply it
// when it does, so no ordering is needed.
func (*frrConfigDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *frrConfigDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	doc, _, err := desired.ParseFRRValue(obj)
	if err != nil {
		return nil, err
	}
	return nil, d.rt.apply(ctx, doc)
}

func (d *frrConfigDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete applies the framework-only configuration (FRR drops every protocol and filter of this agent). An FRR that is
// not running holds nothing to remove.
func (d *frrConfigDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if !d.rt.Running() {
		d.rt.mu.Lock()
		d.rt.last, d.rt.lastFiles = nil, nil
		d.rt.mu.Unlock()
		return nil
	}
	return d.rt.apply(ctx, nil)
}

func (d *frrConfigDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	return d.rt.retrieve(ctx)
}

// ---- events ---------------------------------------------------------------------------------------

// pollInterval is the event poll period (frr.DefaultPollInterval; tests shorten it).
var pollInterval = frr.DefaultPollInterval

// startPoller starts the 1 Hz event poller once (after the first successful apply).
func (rt *FRR) startPoller() {
	if rt.publish == nil {
		return
	}
	rt.pollOnce.Do(func() {
		go rt.pollLoop()
	})
}

func (rt *FRR) pollLoop() {
	var p *frr.Poller
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-rt.stop:
			return
		case <-t.C:
		}
		rt.mu.Lock()
		active := rt.last != nil
		rt.mu.Unlock()
		if !active || !rt.Running() {
			p = nil // a new baseline once FRR carries this agent's configuration again
			continue
		}
		if p == nil {
			p = rt.r.NewPoller()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		evs, err := p.Step(ctx)
		cancel()
		if err != nil {
			rt.log.Debug("FRR poll", "err", err)
		}
		for _, e := range evs {
			if ev := EventOf(e); ev != nil {
				rt.publish(ev)
			}
		}
	}
}

// EventOf maps an FRR poller event to the agent's Event: bgp-neighbors → EVENT_KIND_BGP_NEIGHBOR_CHANGED, routes →
// EVENT_KIND_ROUTING_CHANGED; FRR's interface events are dropped (the agent reports VPP's own link events).
func EventOf(e frr.Event) *vrxv1.Event {
	switch e.Poller {
	case bgp.PollerNeighbors:
		vrf, peer := bgp.SplitNeighborKey(e.Key)
		return &vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_BGP_NEIGHBOR_CHANGED,
			Message:    fmt.Sprintf("BGP neighbour %s (vrf %s): %s → %s", peer, vrf, dash(e.Old), dash(e.New)),
			Attributes: map[string]string{"source": "frr", "vrf": vrf, "peer": peer, "old": e.Old, "new": e.New}}
	case frr.PollerRoutes:
		parts := strings.SplitN(e.Key, "/", 3)
		if len(parts) != 3 {
			return nil
		}
		return &vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_ROUTING_CHANGED,
			Message: fmt.Sprintf("FRR %s routes in vrf %s (%s): %s → %s", parts[2], parts[1], parts[0], zero(e.Old), zero(e.New)),
			Attributes: map[string]string{"source": "frr", "family": parts[0], "vrf": parts[1], "protocol": parts[2],
				"old": zero(e.Old), "new": zero(e.New)}}
	}
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func zero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// ---- state (RoutingState RPC) -----------------------------------------------------------------------

// frrStateTimeout bounds the FRR part of RoutingState (each vtysh call has its own 15 s bound as well).
const frrStateTimeout = 20 * time.Second

// MaxRIBLookups bounds RoutingStateRequest.rib_prefixes (one scoped vtysh call each).
const MaxRIBLookups = 100

// ErrState marks a caller error in a state request (INVALID_ARGUMENT).
var ErrState = errors.New("routing state: invalid request")

// FRRState is what RoutingState reports from FRR.
type FRRState struct {
	Running   bool
	Version   string
	Err       string
	BGP       []bgp.Instance
	RIBCounts map[string]uint32
	Readers   map[string]string
	RIB       []*vrxv1.RoutingRibEntry
}

// State reads the live FRR state: version, BGP summary, RIB counts, the named registered readers and a RIB lookup of
// prefixes in vrf. Errors about the request wrap ErrState; FRR being down is reported in Err, not as an error.
func (rt *FRR) State(ctx context.Context, readers, prefixes []string, vrf string) (*FRRState, error) {
	known := map[string]frr.StateReader{}
	for _, sr := range frr.RegisteredStateReaders() {
		known[sr.Key] = sr
	}
	for _, k := range readers {
		if _, ok := known[k]; !ok {
			return nil, fmt.Errorf("%w: unknown state reader %q (registered: %s)", ErrState, k, strings.Join(slices.Sorted(maps.Keys(known)), ", "))
		}
	}
	if len(prefixes) > MaxRIBLookups {
		return nil, fmt.Errorf("%w: %d rib_prefixes, at most %d", ErrState, len(prefixes), MaxRIBLookups)
	}
	pfxs := make([]netip.Prefix, 0, len(prefixes))
	for _, s := range prefixes {
		p, err := netip.ParsePrefix(s)
		if err != nil || p.Masked() != p {
			return nil, fmt.Errorf("%w: rib_prefixes: %q is not a canonical prefix", ErrState, s)
		}
		pfxs = append(pfxs, p)
	}
	if vrf == "" {
		vrf = frr.DefaultVRF
	}
	if _, err := frr.VRFName(vrf); err != nil {
		return nil, fmt.Errorf("%w: rib_vrf: %v", ErrState, err)
	}
	st := &FRRState{RIBCounts: map[string]uint32{}, Readers: map[string]string{}}
	ctx, cancel := context.WithTimeout(ctx, frrStateTimeout)
	defer cancel()
	switch {
	case !rt.Enabled():
		st.Err = fmt.Sprintf("FRR is not available to this agent (owner %q)", rt.owner)
		return st, nil
	case !rt.Running():
		st.Err = "FRR is not running (no vty socket in " + rt.paths.SocketDir() + ")"
		return st, nil
	}
	ver, err := rt.r.Show(ctx, frr.ShowVersion)
	if err != nil {
		st.Err = err.Error()
		return st, nil
	}
	st.Running = true
	if m := versionRe.FindStringSubmatch(string(ver)); m != nil {
		st.Version = m[1]
	}
	var errs []string
	if st.BGP, err = bgp.Summary(ctx, rt.r.ShowJSON); err != nil {
		errs = append(errs, err.Error())
	}
	for fam, cmd := range map[string]frr.ShowCommand{"ipv4": frr.ShowIPSummaryAll, "ipv6": frr.ShowIPv6SummaryAll} {
		raw, err := rt.r.ShowJSON(ctx, cmd)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		var byVRF map[string]struct {
			Routes []struct {
				RIB  uint32 `json:"rib"`
				Type string `json:"type"`
			} `json:"routes"`
		}
		if err := json.Unmarshal(raw, &byVRF); err != nil {
			errs = append(errs, fmt.Sprintf("decode %s: %v", cmd, err))
			continue
		}
		for v, sm := range byVRF {
			for _, r := range sm.Routes {
				st.RIBCounts[fam+"/"+v+"/"+r.Type] += r.RIB
				if r.Type == "ebgp" || r.Type == "ibgp" { // FRR's summary splits BGP; "<fam>/<vrf>/bgp" is their sum
					st.RIBCounts[fam+"/"+v+"/bgp"] += r.RIB
				}
			}
		}
	}
	for _, k := range readers {
		raw, err := rt.r.ShowJSON(ctx, known[k].Command)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		st.Readers[k] = string(raw)
	}
	for _, p := range pfxs {
		entries, err := rt.lookup(ctx, vrf, p)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		st.RIB = append(st.RIB, entries...)
	}
	st.Err = strings.Join(errs, "; ")
	return st, nil
}

// lookup reads FRR's RIB entries of exactly p in vrf: `show ip[v6] route vrf <vrf> <p> json`. The command is built
// from a canonical netip.Prefix and a validated VRF name (never raw input) and passes the framework's show-command
// check; its output is bounded by the entries of one prefix.
func (rt *FRR) lookup(ctx context.Context, vrf string, p netip.Prefix) ([]*vrxv1.RoutingRibEntry, error) {
	fam := "ip"
	if p.Addr().Is6() {
		fam = "ipv6"
	}
	raw, err := rt.r.ShowJSON(ctx, frr.ShowCommand(fmt.Sprintf("show %s route vrf %s %s json", fam, vrf, p)))
	if err != nil {
		return nil, err
	}
	var byPrefix map[string][]struct {
		Prefix    string `json:"prefix"`
		Protocol  string `json:"protocol"`
		VRFName   string `json:"vrfName"`
		Selected  bool   `json:"selected"`
		Installed bool   `json:"installed"`
		Distance  uint32 `json:"distance"`
		Metric    uint32 `json:"metric"`
		Nexthops  []struct {
			IP            string `json:"ip"`
			InterfaceName string `json:"interfaceName"`
			Active        bool   `json:"active"`
			FIB           bool   `json:"fib"`
		} `json:"nexthops"`
	}
	if err := json.Unmarshal(raw, &byPrefix); err != nil {
		return nil, fmt.Errorf("decode route %s: %w", p, err)
	}
	var out []*vrxv1.RoutingRibEntry
	for _, e := range byPrefix[p.String()] {
		re := &vrxv1.RoutingRibEntry{Prefix: p.String(), Vrf: vrf, Protocol: e.Protocol, Selected: e.Selected,
			Installed: e.Installed, Distance: e.Distance, Metric: e.Metric}
		for _, nh := range e.Nexthops {
			re.NextHops = append(re.NextHops, &vrxv1.RoutingRibNextHop{Address: nh.IP, Interface: nh.InterfaceName, Active: nh.Active, Fib: nh.FIB})
		}
		out = append(out, re)
	}
	return out, nil
}

// lcpPairsTimeout bounds the VPP part of RoutingState: a slow or stuck VPP API must not hold the FRR answer.
const lcpPairsTimeout = 10 * time.Second

// LcpPairs returns this owner's linux-cp pairs from VPP (lcp_itf_pair_get + the owner's interface table).
func (rt *FRR) LcpPairs(ctx context.Context) ([]*vrxv1.RoutingLcpPair, error) {
	ctx, cancel := context.WithTimeout(ctx, lcpPairsTimeout)
	defer cancel()
	kvs, err := lcp.NewItfPair(rt.client, rt.owner).Retrieve(ctx)
	if err != nil {
		if errors.Is(err, dfkit.ErrPluginNotLoaded) {
			return nil, nil
		}
		return nil, err
	}
	var out []*vrxv1.RoutingLcpPair
	for _, kv := range kvs {
		var p lcp.ItfPair
		if err := dfkit.Decode(kv.Value, &p); err != nil {
			continue
		}
		rp := &vrxv1.RoutingLcpPair{Interface: p.Interface, HostIfName: p.HostIfName, HostIfType: p.HostIfType, Netns: p.Netns}
		if m, ok := kv.Meta.(lcp.PairMeta); ok {
			rp.PhySwIfIndex, rp.HostSwIfIndex, rp.VifIndex = m.PhySwIfIndex, m.HostSwIfIndex, m.VifIndex
		}
		out = append(out, rp)
	}
	return out, nil
}
