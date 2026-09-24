package acl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.fd.io/govpp/adapter/statsclient"
	"google.golang.org/protobuf/types/known/timestamppb"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/vlib"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	descacl "ngfw/agent/internal/descriptors/acl"
	dfiface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// Errors of State (the RPC maps them to gRPC codes).
var (
	// ErrInvalid: a request field is out of range (INVALID_ARGUMENT).
	ErrInvalid = errors.New("acl state: invalid request")
	// ErrNotFound: the list is not in VPP for this owner (NOT_FOUND).
	ErrNotFound = errors.New("acl state: no such list in VPP")
)

// CountersFlagCommand is the read-only VPP CLI that prints the acl plugin's counters flag
// ("Stats counters enabled for interface ACLs: 0|1", acl.c acl_show_aclplugin_tables_fn). With the
// "mask" qualifier it prints only the flags and the mask-type table — never the hash tables.
const CountersFlagCommand = "show acl-plugin tables mask"

var countersFlagRe = regexp.MustCompile(`Stats counters enabled for interface ACLs:\s*(\d+)`)

// flagTTL is how long a read of the counters flag is reused.
const flagTTL = 5 * time.Second

// Config opens a Runtime.
type Config struct {
	StateDir string
	Owner    string
	Client   vpp.Client
	// Stats is the stats segment; nil = connect to StatsSocket on first use.
	Stats       descacl.StatsSource
	StatsSocket string
	// GlobalsOwner is D-071's flag (only for the counters_reason text).
	GlobalsOwner bool
	Log          *slog.Logger
	Now          func() time.Time
	// Record is the expansion record (nil = Default).
	Record *Record
}

// Runtime is the acl runtime state of one agent (owner + state dir).
type Runtime struct {
	cfg     Config
	key     string
	tracker *Tracker
	record  *Record

	statsMu sync.Mutex
	stats   descacl.StatsSource
	sc      *statsclient.StatsClient

	flagMu  sync.Mutex
	flagAt  time.Time
	flagOn  bool
	flagErr error
}

var (
	runtimesMu sync.Mutex
	runtimes   = map[string]*Runtime{}
)

func runtimeKey(stateDir, owner string) string { return filepath.Clean(stateDir) + "\x00" + owner }

// Open creates the runtime of cfg.Owner and registers it for RuntimeFor (replacing, and closing,
// an earlier one of the same state dir and owner).
func Open(cfg Config) *Runtime {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Record == nil {
		cfg.Record = Default
	}
	rt := &Runtime{cfg: cfg, key: runtimeKey(cfg.StateDir, cfg.Owner), tracker: NewTracker(), record: cfg.Record, stats: cfg.Stats}
	runtimesMu.Lock()
	prev := runtimes[rt.key]
	runtimes[rt.key] = rt
	runtimesMu.Unlock()
	if prev != nil {
		prev.closeStats()
	}
	return rt
}

// RuntimeFor returns the runtime open for stateDir and owner (nil if none).
func RuntimeFor(stateDir, owner string) *Runtime {
	runtimesMu.Lock()
	defer runtimesMu.Unlock()
	return runtimes[runtimeKey(stateDir, owner)]
}

// Close disconnects the stats segment and unregisters the runtime (idempotent).
func (rt *Runtime) Close() {
	runtimesMu.Lock()
	if runtimes[rt.key] == rt {
		delete(runtimes, rt.key)
	}
	runtimesMu.Unlock()
	rt.closeStats()
}

func (rt *Runtime) closeStats() {
	rt.statsMu.Lock()
	defer rt.statsMu.Unlock()
	if rt.sc != nil {
		_ = rt.sc.Disconnect()
		rt.sc, rt.stats = nil, rt.cfg.Stats
	}
}

// Tracker returns the tracker the wrapped descriptors update.
func (rt *Runtime) Tracker() *Tracker { return rt.tracker }

// Record returns the expansion record.
func (rt *Runtime) Record() *Record { return rt.record }

// statsSource returns the stats segment, connecting on first use.
func (rt *Runtime) statsSource() (descacl.StatsSource, error) {
	rt.statsMu.Lock()
	defer rt.statsMu.Unlock()
	if rt.stats != nil {
		return rt.stats, nil
	}
	if rt.cfg.StatsSocket == "" {
		return nil, errors.New("no stats segment configured")
	}
	sc := statsclient.NewStatsClient(rt.cfg.StatsSocket)
	if err := sc.Connect(); err != nil {
		return nil, fmt.Errorf("stats segment %s: %w", rt.cfg.StatsSocket, err)
	}
	rt.sc, rt.stats = sc, sc
	return sc, nil
}

// dropStats forgets a failed connection (the next call reconnects).
func (rt *Runtime) dropStats() {
	rt.statsMu.Lock()
	defer rt.statsMu.Unlock()
	if rt.sc != nil {
		_ = rt.sc.Disconnect()
		rt.sc, rt.stats = nil, nil
	}
}

// CountersFlag reads the acl plugin's counters flag (VPP has no API getter, V7; the read-only CLI
// prints it). The value is reused for flagTTL.
func (rt *Runtime) CountersFlag(ctx context.Context) (bool, error) {
	rt.flagMu.Lock()
	defer rt.flagMu.Unlock()
	now := rt.cfg.Now()
	if !rt.flagAt.IsZero() && now.Sub(rt.flagAt) < flagTTL && rt.flagErr == nil {
		return rt.flagOn, nil
	}
	on, err := ReadCountersFlag(ctx, rt.cfg.Client)
	rt.flagAt, rt.flagOn, rt.flagErr = now, on, err
	return on, err
}

// ReadCountersFlag runs CountersFlagCommand and parses the flag.
func ReadCountersFlag(ctx context.Context, c vpp.Client) (bool, error) {
	rep, err := vlib.NewServiceClient(c).CliInband(ctx, &vlib.CliInband{Cmd: CountersFlagCommand})
	if err != nil {
		return false, fmt.Errorf("cli_inband %q: %w", CountersFlagCommand, err)
	}
	m := countersFlagRe.FindStringSubmatch(rep.Reply)
	if m == nil {
		return false, fmt.Errorf("cli_inband %q: no counters flag in the reply", CountersFlagCommand)
	}
	n, _ := strconv.Atoi(m[1])
	return n != 0, nil
}

// readCounters reads the counter vectors of the given ACL indexes (one stats dump). A vector VPP
// has not registered yet is reported as absent (all zero).
func (rt *Runtime) readCounters(indexes []uint32) (map[uint32][]descacl.RuleCounter, error) {
	out := map[uint32][]descacl.RuleCounter{}
	if len(indexes) == 0 {
		return out, nil
	}
	src, err := rt.statsSource()
	if err != nil {
		return nil, err
	}
	parts := make([]string, len(indexes))
	for i, idx := range indexes {
		parts[i] = strconv.FormatUint(uint64(idx), 10)
	}
	pattern := `^/acl/(` + strings.Join(parts, "|") + `)/matches$`
	r := descacl.NewStatsReader(src, rt.cfg.Client, rt.cfg.Owner)
	for _, idx := range indexes {
		c, err := r.ReadIndex(idx)
		switch {
		case err == nil:
			out[idx] = c
		case errors.Is(err, descacl.ErrNoCounters):
		default:
			rt.dropStats()
			return nil, fmt.Errorf("%s: %w", pattern, err)
		}
	}
	return out, nil
}

// State implements the AclState RPC (docs/contracts/proto.md §11 F-acl).
func (rt *Runtime) State(ctx context.Context, req *vrxv1.AclStateRequest) (*vrxv1.AclStateResponse, error) {
	limit := int(req.GetLimit())
	switch {
	case limit == 0:
		limit = DefaultLimit
	case limit > MaxLimit:
		return nil, fmt.Errorf("%w: limit %d is more than %d", ErrInvalid, limit, MaxLimit)
	}
	if n := len(req.GetFilter().GetSequences()); n > MaxSequences {
		return nil, fmt.Errorf("%w: %d sequences in the filter, at most %d", ErrInvalid, n, MaxSequences)
	}
	resp := &vrxv1.AclStateResponse{Owner: rt.cfg.Owner, RetrievedAt: timestamppb.New(rt.cfg.Now())}

	tracked := rt.tracker.ACLs()
	if req.GetList() != "" {
		a, ok := rt.tracker.ACL(req.GetList())
		if !ok {
			return nil, fmt.Errorf("%w: %q (owner %q)", ErrNotFound, req.GetList(), rt.cfg.Owner)
		}
		tracked = []Applied{a}
	}

	on, ferr := rt.CountersFlag(ctx)
	var counters map[uint32][]descacl.RuleCounter
	switch {
	case ferr != nil:
		resp.CountersReason = "the counters flag could not be read: " + ferr.Error()
	case !on:
		resp.CountersReason = "per-rule counters are off in VPP (acl.stats-enable is set only by the globals owner, D-071)"
		if rt.cfg.GlobalsOwner {
			resp.CountersReason = "per-rule counters are off in VPP (acl.stats-enable not applied yet)"
		}
	default:
		idx := make([]uint32, len(tracked))
		for i, a := range tracked {
			idx[i] = a.Index
		}
		c, err := rt.readCounters(idx)
		if err != nil {
			resp.CountersReason = "stats segment: " + err.Error()
		} else {
			counters, resp.CountersAvailable = c, true
		}
	}

	for _, a := range tracked {
		exp, _ := rt.record.ACL(a.Name, a.Fingerprint)
		resp.Lists = append(resp.Lists, ListState(a, exp, counters[a.Index]))
	}
	if req.GetList() != "" {
		a := tracked[0]
		if exp, ok := rt.record.ACL(a.Name, a.Fingerprint); ok {
			var c []descacl.RuleCounter
			if resp.CountersAvailable {
				c = counters[a.Index]
				if c == nil {
					c = []descacl.RuleCounter{}
				}
			}
			var total int
			resp.Rules, total = RulePage(exp, c, req.GetFilter(), int(req.GetOffset()), limit)
			resp.Total = uint32(min(total, maxRuleCounts)) //nolint:gosec // bounded
		}
	} else {
		for _, a := range rt.tracker.MacipACLs() {
			resp.MacipLists = append(resp.MacipLists, &vrxv1.AclListState{
				Name: a.Name, AclIndex: a.Index, VppRules: uint32(min(a.VPPRules, maxRuleCounts)), //nolint:gosec // bounded
				MappingKnown: true, ConfigRules: uint32(min(a.VPPRules, maxRuleCounts)), //nolint:gosec // bounded
			})
		}
	}
	if req.GetIncludeInterfaces() {
		ifs, err := rt.Interfaces(ctx)
		if err != nil {
			return nil, err
		}
		resp.Interfaces = ifs
	}
	return resp, nil
}

// Interfaces reports every interface with ACLs or a MACIP ACL bound, by sw_if_index: input and
// output lists in VPP order, other owners' ACLs included (D-066). Small dumps only: the interface
// table, the binding lists, and one acl_dump per foreign ACL index (to show its tag).
func (rt *Runtime) Interfaces(ctx context.Context) ([]*vrxv1.AclInterfaceState, error) {
	c, owner := rt.cfg.Client, rt.cfg.Owner
	tbl, err := dfiface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	svc := vppacl.NewServiceClient(c)
	lists, err := dumpAll(svc.ACLInterfaceListDump(ctx, &vppacl.ACLInterfaceListDump{SwIfIndex: ^interface_types.InterfaceIndex(0)}))
	if err != nil {
		return nil, fmt.Errorf("acl_interface_list_dump: %w", err)
	}
	macips, err := dumpAll(svc.MacipACLInterfaceListDump(ctx, &vppacl.MacipACLInterfaceListDump{SwIfIndex: ^interface_types.InterfaceIndex(0)}))
	if err != nil {
		return nil, fmt.Errorf("macip_acl_interface_list_dump: %w", err)
	}
	ours := map[uint32]string{}
	for _, a := range rt.tracker.ACLs() {
		ours[a.Index] = a.Name
	}
	oursMacip := map[uint32]string{}
	for _, a := range rt.tracker.MacipACLs() {
		oursMacip[a.Index] = a.Name
	}
	tags := map[uint32]string{}
	bound := func(idx uint32) *vrxv1.AclBoundAcl {
		if name, ok := ours[idx]; ok {
			tag, _ := vpp.OwnerTag(owner, name)
			return &vrxv1.AclBoundAcl{AclIndex: idx, Name: name, Tag: tag}
		}
		tag, ok := tags[idx]
		if !ok {
			tag = aclTag(ctx, svc, idx)
			tags[idx] = tag
		}
		if name, mine := vpp.ParseOwnerTag(tag, owner); mine {
			return &vrxv1.AclBoundAcl{AclIndex: idx, Name: name, Tag: tag}
		}
		return &vrxv1.AclBoundAcl{AclIndex: idx, Tag: tag, Foreign: true}
	}
	byIndex := map[uint32]*vrxv1.AclInterfaceState{}
	get := func(sw uint32) *vrxv1.AclInterfaceState {
		st, ok := byIndex[sw]
		if !ok {
			name, logical := tbl.Logical(sw)
			if !logical {
				name = tbl.VPPName(sw)
			}
			st = &vrxv1.AclInterfaceState{Interface: name, SwIfIndex: sw}
			byIndex[sw] = st
		}
		return st
	}
	for _, d := range lists {
		if len(d.Acls) == 0 {
			continue
		}
		st := get(uint32(d.SwIfIndex))
		for i, idx := range d.Acls {
			if i < int(d.NInput) {
				st.Input = append(st.Input, bound(idx))
			} else {
				st.Output = append(st.Output, bound(idx))
			}
		}
	}
	for _, d := range macips {
		for _, idx := range d.Acls {
			if idx == ^uint32(0) {
				continue // VPP reports ~0 for an interface whose MACIP ACL was removed
			}
			b := &vrxv1.AclBoundAcl{AclIndex: idx}
			if name, ok := oursMacip[idx]; ok {
				b.Name = name
				b.Tag, _ = vpp.OwnerTag(owner, name)
			} else {
				b.Tag = macipTag(ctx, svc, idx)
				if name, mine := vpp.ParseOwnerTag(b.Tag, owner); mine {
					b.Name = name
				} else {
					b.Foreign = true
				}
			}
			get(uint32(d.SwIfIndex)).Macip = b
		}
	}
	out := make([]*vrxv1.AclInterfaceState, 0, len(byIndex))
	for _, st := range byIndex {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SwIfIndex < out[j].SwIfIndex })
	return out, nil
}

type recvStream[T any] interface{ Recv() (T, error) }

// dumpAll drains a generated dump stream (io.EOF = the control_ping_reply).
func dumpAll[T any, S recvStream[T]](stream S, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	var out []T
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			if cl, ok := any(stream).(interface{ Close() error }); ok {
				_ = cl.Close()
			}
			return nil, err
		}
		out = append(out, d)
	}
}

// aclTag dumps one ACL by index for its tag ("" when it cannot be read).
func aclTag(ctx context.Context, svc vppacl.RPCService, idx uint32) string {
	ds, err := dumpAll(svc.ACLDump(ctx, &vppacl.ACLDump{ACLIndex: idx}))
	if err != nil || len(ds) == 0 {
		return ""
	}
	return strings.TrimRight(ds[0].Tag, "\x00")
}

// macipTag dumps one MACIP ACL by index for its tag.
func macipTag(ctx context.Context, svc vppacl.RPCService, idx uint32) string {
	ds, err := dumpAll(svc.MacipACLDump(ctx, &vppacl.MacipACLDump{ACLIndex: idx}))
	if err != nil || len(ds) == 0 {
		return ""
	}
	return strings.TrimRight(ds[0].Tag, "\x00")
}
