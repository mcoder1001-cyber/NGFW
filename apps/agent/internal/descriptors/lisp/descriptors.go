package lisp

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	lispapi "ngfw/agent/binapi/lisp"
	gpeapi "ngfw/agent/binapi/lisp_gpe"
	"ngfw/agent/binapi/lisp_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	EnableName        = "lisp.enable"
	GpeEnableName     = "lisp-gpe.enable"
	LocatorSetName    = "lisp.locator-set"
	LocatorName       = "lisp.locator"
	LocalEidName      = "lisp.local-eid"
	MapResolverName   = "lisp.map-resolver"
	MapServerName     = "lisp.map-server"
	RemoteMappingName = "lisp.remote-mapping"
	AdjacencyName     = "lisp.adjacency"
	EidTableMapName   = "lisp.eid-table-map"
	PitrName          = "lisp.pitr"
	GpeFwdEntryName   = "lisp-gpe.fwd-entry"
)

// NewEnable returns the global LISP switch. Note: VPP enables LISP-GPE together with LISP.
func NewEnable(c vpp.Client) *df6.SingletonDescriptor[*Enable] {
	set := func(ctx context.Context, c vpp.Client, on bool) error {
		if _, err := lispapi.NewServiceClient(c).LispEnableDisable(ctx, &lispapi.LispEnableDisable{IsEnable: on}); err != nil {
			return fmt.Errorf("lisp_enable_disable: %w", err)
		}
		return nil
	}
	return df6.NewSingletonDescriptor(df6.SingletonSpec[*Enable]{
		Name: EnableName, Plugin: Plugin,
		Set:   func(ctx context.Context, c vpp.Client, _ *Enable) error { return set(ctx, c, true) },
		Unset: func(ctx context.Context, c vpp.Client, _ *Enable) error { return set(ctx, c, false) },
		Get: func(ctx context.Context, c vpp.Client) (*Enable, bool, error) {
			on, _, err := status(ctx, c)
			return &Enable{}, on, err
		},
	}, c)
}

// NewGpeEnable returns the global LISP-GPE switch.
func NewGpeEnable(c vpp.Client) *df6.SingletonDescriptor[*GpeEnable] {
	set := func(ctx context.Context, c vpp.Client, on bool) error {
		if _, err := gpeapi.NewServiceClient(c).GpeEnableDisable(ctx, &gpeapi.GpeEnableDisable{IsEnable: on}); err != nil {
			return fmt.Errorf("gpe_enable_disable: %w", err)
		}
		return nil
	}
	return df6.NewSingletonDescriptor(df6.SingletonSpec[*GpeEnable]{
		Name: GpeEnableName, Plugin: Plugin,
		Set:   func(ctx context.Context, c vpp.Client, _ *GpeEnable) error { return set(ctx, c, true) },
		Unset: func(ctx context.Context, c vpp.Client, _ *GpeEnable) error { return set(ctx, c, false) },
		Get: func(ctx context.Context, c vpp.Client) (*GpeEnable, bool, error) {
			_, on, err := status(ctx, c)
			return &GpeEnable{}, on, err
		},
	}, c)
}

// NewPitr returns the proxy-ITR singleton.
func NewPitr(c vpp.Client) *df6.SingletonDescriptor[*Pitr] {
	set := func(ctx context.Context, c vpp.Client, name string, on bool) error {
		if _, err := lispapi.NewServiceClient(c).LispPitrSetLocatorSet(ctx, &lispapi.LispPitrSetLocatorSet{IsAdd: on, LsName: name}); err != nil {
			return fmt.Errorf("lisp_pitr_set_locator_set: %w", err)
		}
		return nil
	}
	return df6.NewSingletonDescriptor(df6.SingletonSpec[*Pitr]{
		Name: PitrName, Plugin: Plugin,
		Validate: func(p *Pitr) error { return checkName(p.GetLocatorSet()) },
		Set:      func(ctx context.Context, c vpp.Client, p *Pitr) error { return set(ctx, c, p.GetLocatorSet(), true) },
		Unset:    func(ctx context.Context, c vpp.Client, p *Pitr) error { return set(ctx, c, p.GetLocatorSet(), false) },
		Get: func(ctx context.Context, c vpp.Client) (*Pitr, bool, error) {
			rep, err := lispapi.NewServiceClient(c).ShowLispPitr(ctx, &lispapi.ShowLispPitr{})
			if err != nil {
				return nil, false, fmt.Errorf("show_lisp_pitr: %w", err)
			}
			return &Pitr{LocatorSet: rep.LocatorSetName}, rep.IsEnabled && rep.LocatorSetName != "", nil
		},
		Deps: func(p *Pitr) []scheduler.Dependency {
			return []scheduler.Dependency{{Key: LocatorSetKey(p.GetLocatorSet())}, {Key: EnableKey}}
		},
	}, c)
}

// NewLocatorSet returns the local locator set descriptor.
func NewLocatorSet(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*LocatorSet] {
	addDel := func(ctx context.Context, c vpp.Client, ls *LocatorSet, add bool) error {
		if _, err := lispapi.NewServiceClient(c).LispAddDelLocatorSet(ctx, &lispapi.LispAddDelLocatorSet{IsAdd: add, LocatorSetName: ls.GetName()}); err != nil {
			return fmt.Errorf("lisp_add_del_locator_set: %w", err)
		}
		return nil
	}
	return df6.NewKeyedDescriptor(df6.KeyedSpec[*LocatorSet]{
		Name: LocatorSetName, Plugin: Plugin,
		Canon: func(ls *LocatorSet) (*LocatorSet, error) { return ls, checkName(ls.GetName()) },
		ID:    func(ls *LocatorSet) string { return ls.GetName() },
		Deps:  func(*LocatorSet) []scheduler.Dependency { return []scheduler.Dependency{{Key: EnableKey}} },
		Add:   func(ctx context.Context, c vpp.Client, ls *LocatorSet) error { return addDel(ctx, c, ls, true) },
		Del:   func(ctx context.Context, c vpp.Client, ls *LocatorSet) error { return addDel(ctx, c, ls, false) },
		List: func(ctx context.Context, c vpp.Client) ([]*LocatorSet, error) {
			sets, err := locatorSets(ctx, c)
			if err != nil {
				return nil, err
			}
			out := make([]*LocatorSet, 0, len(sets))
			for _, n := range sets {
				out = append(out, &LocatorSet{Name: n})
			}
			return out, nil
		},
		Owns: func(ls *LocatorSet) bool { return scope.OwnsName(ls.GetName()) },
	}, c)
}

// NewLocator returns the descriptor of interfaces in local locator sets.
func NewLocator(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*Locator] {
	addDel := func(ctx context.Context, c vpp.Client, l *Locator, add bool) error {
		ifs, err := df6.DumpInterfaces(ctx, c, "")
		if err != nil {
			return err
		}
		idx, err := ifs.Index(l.GetInterface())
		if err != nil {
			return err
		}
		if _, err := lispapi.NewServiceClient(c).LispAddDelLocator(ctx, &lispapi.LispAddDelLocator{
			IsAdd: add, LocatorSetName: l.GetLocatorSet(), SwIfIndex: idx, Priority: uint8(l.GetPriority()), Weight: uint8(l.GetWeight()), //nolint:gosec // validated ≤ 255
		}); err != nil {
			return fmt.Errorf("lisp_add_del_locator: %w", err)
		}
		return nil
	}
	return df6.NewKeyedDescriptor(df6.KeyedSpec[*Locator]{
		Name: LocatorName, Plugin: Plugin,
		Canon: func(l *Locator) (*Locator, error) {
			if err := checkName(l.GetLocatorSet()); err != nil {
				return nil, err
			}
			if l.GetInterface() == "" {
				return nil, fmt.Errorf("%w: interface is mandatory", df6.ErrBadValue)
			}
			if _, err := u8(l.GetPriority(), "priority"); err != nil {
				return nil, err
			}
			if _, err := u8(l.GetWeight(), "weight"); err != nil {
				return nil, err
			}
			return l, nil
		},
		ID: func(l *Locator) string { return l.GetLocatorSet() + "/" + l.GetInterface() },
		Deps: func(l *Locator) []scheduler.Dependency {
			return append([]scheduler.Dependency{{Key: LocatorSetKey(l.GetLocatorSet())}}, df6.InterfaceDeps(l.GetInterface())...)
		},
		Add: func(ctx context.Context, c vpp.Client, l *Locator) error { return addDel(ctx, c, l, true) },
		Del: func(ctx context.Context, c vpp.Client, l *Locator) error { return addDel(ctx, c, l, false) },
		List: func(ctx context.Context, c vpp.Client) ([]*Locator, error) {
			sets, err := locatorSets(ctx, c)
			if err != nil {
				return nil, err
			}
			ifs, err := df6.DumpInterfaces(ctx, c, "")
			if err != nil {
				return nil, err
			}
			var out []*Locator
			for idx, name := range sets {
				locs, err := locatorsOf(ctx, c, idx)
				if err != nil {
					return nil, err
				}
				for _, l := range locs {
					if l.Local == 0 {
						continue
					}
					out = append(out, &Locator{LocatorSet: name, Interface: ifs.NameOrEmpty(uint32(l.SwIfIndex)), Priority: uint32(l.Priority), Weight: uint32(l.Weight)})
				}
			}
			return out, nil
		},
		Owns: func(l *Locator) bool { return scope.OwnsName(l.GetLocatorSet()) },
	}, c)
}

// eidTableMapKey is the key of the EID-table map a (vni, eid) needs (none for VNI 0).
func eidTableMapDeps(vni uint32, eid string) []scheduler.Dependency {
	if vni == 0 {
		return nil
	}
	kind := "l3"
	if isMAC(eid) {
		kind = "l2"
	}
	return []scheduler.Dependency{{Key: scheduler.Join(EidTableMapName, kind, df6.U32(vni))}}
}

// NewLocalEid returns the local EID descriptor.
func NewLocalEid(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*LocalEid] {
	addDel := func(ctx context.Context, c vpp.Client, e *LocalEid, add bool) error {
		eid, err := eidOf(e.GetEid())
		if err != nil {
			return err
		}
		if _, err := lispapi.NewServiceClient(c).LispAddDelLocalEid(ctx, &lispapi.LispAddDelLocalEid{
			IsAdd: add, Eid: eid, LocatorSetName: e.GetLocatorSet(), Vni: e.GetVni(),
		}); err != nil {
			return fmt.Errorf("lisp_add_del_local_eid: %w", err)
		}
		return nil
	}
	return df6.NewKeyedDescriptor(df6.KeyedSpec[*LocalEid]{
		Name: LocalEidName, Plugin: Plugin,
		Canon: func(e *LocalEid) (*LocalEid, error) {
			eid, err := canonEID(e.GetEid())
			if err != nil {
				return nil, err
			}
			// An empty locator_set is accepted here only so that a retrieved mapping whose
			// set VPP reports as ~0 can still be deleted; Add requires it.
			if e.GetLocatorSet() != "" {
				if err := checkName(e.GetLocatorSet()); err != nil {
					return nil, err
				}
			}
			out := proto.Clone(e).(*LocalEid)
			out.Eid = eid
			return out, nil
		},
		ID: func(e *LocalEid) string { return df6.U32(e.GetVni()) + "/" + e.GetEid() },
		Deps: func(e *LocalEid) []scheduler.Dependency {
			deps := []scheduler.Dependency{{Key: LocatorSetKey(e.GetLocatorSet())}, {Key: EnableKey}}
			return append(deps, eidTableMapDeps(e.GetVni(), e.GetEid())...)
		},
		Add: func(ctx context.Context, c vpp.Client, e *LocalEid) error {
			if err := checkName(e.GetLocatorSet()); err != nil {
				return err
			}
			return addDel(ctx, c, e, true)
		},
		Del: func(ctx context.Context, c vpp.Client, e *LocalEid) error {
			if e.GetLocatorSet() == "" {
				// VPP's delete only needs *a* valid locator-set name for its lookup.
				sets, err := locatorSets(ctx, c)
				if err != nil {
					return err
				}
				for _, n := range sets {
					e = proto.Clone(e).(*LocalEid)
					e.LocatorSet = n
					break
				}
			}
			return addDel(ctx, c, e, false)
		},
		List: func(ctx context.Context, c vpp.Client) ([]*LocalEid, error) {
			recs, err := eidTable(ctx, c, lispapi.LISP_LOCATOR_SET_FILTER_API_LOCAL)
			if err != nil {
				return nil, err
			}
			sets, err := locatorSets(ctx, c)
			if err != nil {
				return nil, err
			}
			var out []*LocalEid
			for _, r := range recs {
				eid := mappingEID(r)
				if !r.IsLocal || eid == "" {
					continue
				}
				// VPP reports locator_set_index ~0 when the set has no locators; the name
				// is then unknown ("") and Retrieve differs from desired until a locator
				// is added (docs/agent/descriptors/lisp.md).
				out = append(out, &LocalEid{Vni: r.Vni, Eid: eid, LocatorSet: sets[r.LocatorSetIndex]})
			}
			return out, nil
		},
		Owns: func(e *LocalEid) bool { return ownsEID(scope, e.GetEid(), e.GetVni()) },
	}, c)
}

// addrSpec builds the map-resolver / map-server descriptors (one address each).
func addrSpec[T interface {
	proto.Message
	GetAddress() string
}](name string, mk func(string) T, addDel func(context.Context, vpp.Client, T, bool) error, list func(context.Context, vpp.Client) ([]T, error), scope *df6.Scope) df6.KeyedSpec[T] {
	return df6.KeyedSpec[T]{
		Name: name, Plugin: Plugin,
		Canon: func(t T) (T, error) {
			a, err := df6.ParseAddr(t.GetAddress())
			if err != nil {
				var zero T
				return zero, err
			}
			return mk(a.String()), nil
		},
		ID:   func(t T) string { return t.GetAddress() },
		Deps: func(T) []scheduler.Dependency { return []scheduler.Dependency{{Key: EnableKey}} },
		Add:  func(ctx context.Context, c vpp.Client, t T) error { return addDel(ctx, c, t, true) },
		Del:  func(ctx context.Context, c vpp.Client, t T) error { return addDel(ctx, c, t, false) },
		List: list,
		Owns: func(t T) bool { return scope.OwnsAddrString(t.GetAddress()) },
	}
}

// NewMapResolver returns the map-resolver descriptor.
func NewMapResolver(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*MapResolver] {
	mk := func(a string) *MapResolver { return &MapResolver{Address: a} }
	return df6.NewKeyedDescriptor(addrSpec(MapResolverName, mk,
		func(ctx context.Context, c vpp.Client, m *MapResolver, add bool) error {
			a, err := df6.AddressOf(m.GetAddress())
			if err != nil {
				return err
			}
			if _, err := lispapi.NewServiceClient(c).LispAddDelMapResolver(ctx, &lispapi.LispAddDelMapResolver{IsAdd: add, IPAddress: a}); err != nil {
				return fmt.Errorf("lisp_add_del_map_resolver: %w", err)
			}
			return nil
		},
		func(ctx context.Context, c vpp.Client) ([]*MapResolver, error) {
			stream, err := lispapi.NewServiceClient(c).LispMapResolverDump(ctx, &lispapi.LispMapResolverDump{})
			if err != nil {
				return nil, fmt.Errorf("lisp_map_resolver_dump: %w", err)
			}
			recs, err := df6.Collect(stream.Recv)
			if err != nil {
				return nil, fmt.Errorf("lisp_map_resolver_dump: %w", err)
			}
			out := make([]*MapResolver, 0, len(recs))
			for _, r := range recs {
				out = append(out, mk(df6.FromAddress(r.IPAddress).String()))
			}
			return out, nil
		}, scope), c)
}

// NewMapServer returns the map-server descriptor.
func NewMapServer(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*MapServer] {
	mk := func(a string) *MapServer { return &MapServer{Address: a} }
	return df6.NewKeyedDescriptor(addrSpec(MapServerName, mk,
		func(ctx context.Context, c vpp.Client, m *MapServer, add bool) error {
			a, err := df6.AddressOf(m.GetAddress())
			if err != nil {
				return err
			}
			if _, err := lispapi.NewServiceClient(c).LispAddDelMapServer(ctx, &lispapi.LispAddDelMapServer{IsAdd: add, IPAddress: a}); err != nil {
				return fmt.Errorf("lisp_add_del_map_server: %w", err)
			}
			return nil
		},
		func(ctx context.Context, c vpp.Client) ([]*MapServer, error) {
			stream, err := lispapi.NewServiceClient(c).LispMapServerDump(ctx, &lispapi.LispMapServerDump{})
			if err != nil {
				return nil, fmt.Errorf("lisp_map_server_dump: %w", err)
			}
			recs, err := df6.Collect(stream.Recv)
			if err != nil {
				return nil, fmt.Errorf("lisp_map_server_dump: %w", err)
			}
			out := make([]*MapServer, 0, len(recs))
			for _, r := range recs {
				out = append(out, mk(df6.FromAddress(r.IPAddress).String()))
			}
			return out, nil
		}, scope), c)
}

func canonRlocs(in []*Rloc) ([]*Rloc, error) {
	out := make([]*Rloc, 0, len(in))
	for _, r := range in {
		a, err := df6.ParseAddr(r.GetAddress())
		if err != nil {
			return nil, err
		}
		if _, err := u8(r.GetPriority(), "priority"); err != nil {
			return nil, err
		}
		if _, err := u8(r.GetWeight(), "weight"); err != nil {
			return nil, err
		}
		out = append(out, &Rloc{Address: a.String(), Priority: r.GetPriority(), Weight: r.GetWeight()})
	}
	// VPP keeps remote locators in its own order; the canonical form is sorted by address.
	slices.SortFunc(out, func(a, b *Rloc) int { return strings.Compare(a.GetAddress(), b.GetAddress()) })
	return out, nil
}

// NewRemoteMapping returns the static remote-mapping descriptor.
func NewRemoteMapping(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*RemoteMapping] {
	return df6.NewKeyedDescriptor(df6.KeyedSpec[*RemoteMapping]{
		Name: RemoteMappingName, Plugin: Plugin,
		Canon: func(m *RemoteMapping) (*RemoteMapping, error) {
			eid, err := canonEID(m.GetEid())
			if err != nil {
				return nil, err
			}
			rlocs, err := canonRlocs(m.GetRlocs())
			if err != nil {
				return nil, err
			}
			if m.GetAction() > 3 {
				return nil, fmt.Errorf("%w: action %d", df6.ErrBadValue, m.GetAction())
			}
			if len(rlocs) > 0 && m.GetAction() != 0 {
				return nil, fmt.Errorf("%w: action only for negative mappings (no rlocs)", df6.ErrBadValue)
			}
			return &RemoteMapping{Vni: m.GetVni(), Eid: eid, Rlocs: rlocs, Action: m.GetAction()}, nil
		},
		ID: func(m *RemoteMapping) string { return df6.U32(m.GetVni()) + "/" + m.GetEid() },
		Deps: func(m *RemoteMapping) []scheduler.Dependency {
			return append([]scheduler.Dependency{{Key: EnableKey}}, eidTableMapDeps(m.GetVni(), m.GetEid())...)
		},
		Add: func(ctx context.Context, c vpp.Client, m *RemoteMapping) error {
			eid, err := eidOf(m.GetEid())
			if err != nil {
				return err
			}
			req := &lispapi.LispAddDelRemoteMapping{IsAdd: true, Vni: m.GetVni(), Deid: eid, Action: uint8(m.GetAction())} //nolint:gosec // ≤ 3
			for _, r := range m.GetRlocs() {
				a, _ := df6.AddressOf(r.GetAddress())
				req.Rlocs = append(req.Rlocs, lisp_types.RemoteLocator{IPAddress: a, Priority: uint8(r.GetPriority()), Weight: uint8(r.GetWeight())}) //nolint:gosec // ≤ 255
			}
			if _, err := lispapi.NewServiceClient(c).LispAddDelRemoteMapping(ctx, req); err != nil {
				return fmt.Errorf("lisp_add_del_remote_mapping: %w", err)
			}
			return nil
		},
		Del: func(ctx context.Context, c vpp.Client, m *RemoteMapping) error {
			eid, err := eidOf(m.GetEid())
			if err != nil {
				return err
			}
			if _, err := lispapi.NewServiceClient(c).LispAddDelRemoteMapping(ctx, &lispapi.LispAddDelRemoteMapping{IsAdd: false, Vni: m.GetVni(), Deid: eid}); err != nil {
				return fmt.Errorf("lisp_add_del_remote_mapping (del): %w", err)
			}
			return nil
		},
		List: func(ctx context.Context, c vpp.Client) ([]*RemoteMapping, error) {
			recs, err := eidTable(ctx, c, lispapi.LISP_LOCATOR_SET_FILTER_API_REMOTE)
			if err != nil {
				return nil, err
			}
			var out []*RemoteMapping
			for _, r := range recs {
				eid := mappingEID(r)
				if r.IsLocal || eid == "" || r.IsSrcDst {
					continue
				}
				m := &RemoteMapping{Vni: r.Vni, Eid: eid, Action: uint32(r.Action)}
				if r.LocatorSetIndex != ^uint32(0) {
					locs, err := locatorsOf(ctx, c, r.LocatorSetIndex)
					if err != nil {
						return nil, err
					}
					for _, l := range locs {
						m.Rlocs = append(m.Rlocs, &Rloc{Address: df6.FromAddress(l.IPAddress).String(), Priority: uint32(l.Priority), Weight: uint32(l.Weight)})
					}
				}
				if len(m.Rlocs) > 0 {
					m.Action = 0
				}
				m.Rlocs, _ = canonRlocs(m.Rlocs)
				out = append(out, m)
			}
			return out, nil
		},
		Owns: func(m *RemoteMapping) bool { return ownsEID(scope, m.GetEid(), m.GetVni()) },
	}, c)
}

// NewAdjacency returns the adjacency descriptor.
func NewAdjacency(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*Adjacency] {
	addDel := func(ctx context.Context, c vpp.Client, a *Adjacency, add bool) error {
		reid, err := eidOf(a.GetReid())
		if err != nil {
			return err
		}
		leid, err := eidOf(a.GetLeid())
		if err != nil {
			return err
		}
		if _, err := lispapi.NewServiceClient(c).LispAddDelAdjacency(ctx, &lispapi.LispAddDelAdjacency{IsAdd: add, Vni: a.GetVni(), Reid: reid, Leid: leid}); err != nil {
			return fmt.Errorf("lisp_add_del_adjacency: %w", err)
		}
		return nil
	}
	return df6.NewKeyedDescriptor(df6.KeyedSpec[*Adjacency]{
		Name: AdjacencyName, Plugin: Plugin,
		Canon: func(a *Adjacency) (*Adjacency, error) {
			r, err := canonEID(a.GetReid())
			if err != nil {
				return nil, err
			}
			l, err := canonEID(a.GetLeid())
			if err != nil {
				return nil, err
			}
			return &Adjacency{Vni: a.GetVni(), Reid: r, Leid: l}, nil
		},
		ID: func(a *Adjacency) string { return df6.U32(a.GetVni()) + "/" + a.GetReid() + "/" + a.GetLeid() },
		Deps: func(a *Adjacency) []scheduler.Dependency {
			v := df6.U32(a.GetVni())
			return []scheduler.Dependency{
				{Key: scheduler.Join(RemoteMappingName, v, a.GetReid())},
				{Key: scheduler.Join(LocalEidName, v, a.GetLeid())},
			}
		},
		Add: func(ctx context.Context, c vpp.Client, a *Adjacency) error { return addDel(ctx, c, a, true) },
		Del: func(ctx context.Context, c vpp.Client, a *Adjacency) error { return addDel(ctx, c, a, false) },
		List: func(ctx context.Context, c vpp.Client) ([]*Adjacency, error) {
			vnis, err := eidVNIs(ctx, c)
			if err != nil {
				return nil, err
			}
			var out []*Adjacency
			for _, v := range vnis {
				rep, err := lispapi.NewServiceClient(c).LispAdjacenciesGet(ctx, &lispapi.LispAdjacenciesGet{Vni: v})
				if err != nil {
					return nil, fmt.Errorf("lisp_adjacencies_get %d: %w", v, err)
				}
				for _, a := range rep.Adjacencies {
					out = append(out, &Adjacency{Vni: v, Reid: eidString(a.Reid), Leid: eidString(a.Leid)})
				}
			}
			return out, nil
		},
		Owns: func(a *Adjacency) bool { return ownsEID(scope, a.GetReid(), a.GetVni()) },
	}, c)
}

// eidVNIs lists the VNIs VPP has EID-table entries for (VNI 0 always included).
func eidVNIs(ctx context.Context, c vpp.Client) ([]uint32, error) {
	stream, err := lispapi.NewServiceClient(c).LispEidTableVniDump(ctx, &lispapi.LispEidTableVniDump{})
	if err != nil {
		return nil, fmt.Errorf("lisp_eid_table_vni_dump: %w", err)
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("lisp_eid_table_vni_dump: %w", err)
	}
	out := []uint32{0}
	for _, r := range recs {
		if !slices.Contains(out, r.Vni) {
			out = append(out, r.Vni)
		}
	}
	return out, nil
}

// EidTableMapID is the object id of m: "l2/<vni>" or "l3/<vni>".
func EidTableMapID(m *EidTableMap) string {
	if m.GetIsL2() {
		return "l2/" + df6.U32(m.GetVni())
	}
	return "l3/" + df6.U32(m.GetVni())
}

// NewEidTableMap returns the VNI ↔ VRF/BD map descriptor.
func NewEidTableMap(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*EidTableMap] {
	addDel := func(ctx context.Context, c vpp.Client, m *EidTableMap, add bool) error {
		if _, err := lispapi.NewServiceClient(c).LispEidTableAddDelMap(ctx, &lispapi.LispEidTableAddDelMap{IsAdd: add, Vni: m.GetVni(), DpTable: m.GetDpTable(), IsL2: m.GetIsL2()}); err != nil {
			return fmt.Errorf("lisp_eid_table_add_del_map: %w", err)
		}
		return nil
	}
	return df6.NewKeyedDescriptor(df6.KeyedSpec[*EidTableMap]{
		Name: EidTableMapName, Plugin: Plugin,
		Canon: func(m *EidTableMap) (*EidTableMap, error) {
			if m.GetVni() == 0 {
				return nil, fmt.Errorf("%w: vni 0 is implicitly mapped to table 0", df6.ErrBadValue)
			}
			return m, nil
		},
		ID: EidTableMapID,
		Deps: func(m *EidTableMap) []scheduler.Dependency {
			if m.GetIsL2() {
				return []scheduler.Dependency{{Key: df6.BridgeDomainKey(m.GetDpTable())}}
			}
			return df6.VRFDeps(m.GetDpTable())
		},
		Add: func(ctx context.Context, c vpp.Client, m *EidTableMap) error {
			if !m.GetIsL2() {
				// VPP installs routes in fib_table_find(dp_table) of both families.
				if err := df6.RequireTable(ctx, c, m.GetDpTable(), false); err != nil {
					return err
				}
			}
			return addDel(ctx, c, m, true)
		},
		Del: func(ctx context.Context, c vpp.Client, m *EidTableMap) error { return addDel(ctx, c, m, false) },
		List: func(ctx context.Context, c vpp.Client) ([]*EidTableMap, error) {
			var out []*EidTableMap
			for _, l2 := range []bool{false, true} {
				stream, err := lispapi.NewServiceClient(c).LispEidTableMapDump(ctx, &lispapi.LispEidTableMapDump{IsL2: l2})
				if err != nil {
					return nil, fmt.Errorf("lisp_eid_table_map_dump: %w", err)
				}
				recs, err := df6.Collect(stream.Recv)
				if err != nil {
					return nil, fmt.Errorf("lisp_eid_table_map_dump: %w", err)
				}
				for _, r := range recs {
					if r.Vni == 0 {
						continue
					}
					out = append(out, &EidTableMap{Vni: r.Vni, DpTable: r.DpTable, IsL2: l2})
				}
			}
			return out, nil
		},
		Owns: func(m *EidTableMap) bool { return scope.OwnsVNI(m.GetVni()) },
	}, c)
}

// NewGpeFwdEntry returns the LISP-GPE forwarding entry descriptor.
func NewGpeFwdEntry(c vpp.Client, scope *df6.Scope) *df6.KeyedDescriptor[*GpeFwdEntry] {
	req := func(e *GpeFwdEntry, add bool) (*gpeapi.GpeAddDelFwdEntry, error) {
		reid, err := eidOf(e.GetReid())
		if err != nil {
			return nil, err
		}
		leid, err := eidOf(e.GetLeid())
		if err != nil {
			return nil, err
		}
		r := &gpeapi.GpeAddDelFwdEntry{IsAdd: add, RmtEid: reid, LclEid: leid, Vni: e.GetVni(), DpTable: e.GetDpTable(), Action: uint8(e.GetAction())} //nolint:gosec // ≤ 3
		// Locators: all local ones first, then the remote ones (unformat_gpe_loc_pairs).
		var remote []gpeapi.GpeLocator
		for _, p := range e.GetPairs() {
			l, _ := df6.AddressOf(p.GetLocal())
			rm, _ := df6.AddressOf(p.GetRemote())
			r.Locs = append(r.Locs, gpeapi.GpeLocator{Weight: uint8(p.GetWeight()), Addr: l})  //nolint:gosec // ≤ 255
			remote = append(remote, gpeapi.GpeLocator{Weight: uint8(p.GetWeight()), Addr: rm}) //nolint:gosec // ≤ 255
		}
		r.Locs = append(r.Locs, remote...)
		return r, nil
	}
	return df6.NewKeyedDescriptor(df6.KeyedSpec[*GpeFwdEntry]{
		Name: GpeFwdEntryName, Plugin: Plugin,
		Canon: func(e *GpeFwdEntry) (*GpeFwdEntry, error) {
			reid, err := canonEID(e.GetReid())
			if err != nil {
				return nil, err
			}
			// leid is mandatory: an unset API EID decodes as 0.0.0.0/0, which would be ambiguous.
			leid, err := canonEID(e.GetLeid())
			if err != nil {
				return nil, err
			}
			if e.GetAction() > 3 || (len(e.GetPairs()) > 0 && e.GetAction() != 0) {
				return nil, fmt.Errorf("%w: action only for negative entries (no pairs), 0–3", df6.ErrBadValue)
			}
			out := &GpeFwdEntry{Vni: e.GetVni(), DpTable: e.GetDpTable(), Reid: reid, Leid: leid, Action: e.GetAction()}
			for _, p := range e.GetPairs() {
				l, err := df6.ParseAddr(p.GetLocal())
				if err != nil {
					return nil, err
				}
				r, err := df6.ParseAddr(p.GetRemote())
				if err != nil {
					return nil, err
				}
				if l.Is4() != r.Is4() {
					return nil, fmt.Errorf("%w: locator pair families differ", df6.ErrBadValue)
				}
				if _, err := u8(p.GetWeight(), "weight"); err != nil {
					return nil, err
				}
				out.Pairs = append(out.Pairs, &LocatorPair{Local: l.String(), Remote: r.String(), Weight: p.GetWeight()})
			}
			slices.SortFunc(out.Pairs, func(a, b *LocatorPair) int {
				return strings.Compare(a.GetLocal()+" "+a.GetRemote(), b.GetLocal()+" "+b.GetRemote())
			})
			return out, nil
		},
		ID: func(e *GpeFwdEntry) string { return df6.U32(e.GetVni()) + "/" + e.GetReid() + "/" + e.GetLeid() },
		Deps: func(e *GpeFwdEntry) []scheduler.Dependency {
			return append([]scheduler.Dependency{{Key: GpeEnableKey}}, df6.VRFDeps(e.GetDpTable())...)
		},
		Add: func(ctx context.Context, c vpp.Client, e *GpeFwdEntry) error {
			if !isMAC(e.GetReid()) {
				p, _ := netip.ParsePrefix(e.GetReid())
				if err := df6.RequireTable(ctx, c, e.GetDpTable(), p.Addr().Is6()); err != nil {
					return err
				}
			}
			r, err := req(e, true)
			if err != nil {
				return err
			}
			if _, err := gpeapi.NewServiceClient(c).GpeAddDelFwdEntry(ctx, r); err != nil {
				return fmt.Errorf("gpe_add_del_fwd_entry: %w", err)
			}
			return nil
		},
		Del: func(ctx context.Context, c vpp.Client, e *GpeFwdEntry) error {
			r, err := req(e, false)
			if err != nil {
				return err
			}
			if _, err := gpeapi.NewServiceClient(c).GpeAddDelFwdEntry(ctx, r); err != nil {
				return fmt.Errorf("gpe_add_del_fwd_entry (del): %w", err)
			}
			return nil
		},
		List: func(ctx context.Context, c vpp.Client) ([]*GpeFwdEntry, error) {
			svc := gpeapi.NewServiceClient(c)
			// Entries the LISP control plane programs for its adjacencies belong to
			// lisp.adjacency, not to this descriptor.
			cp := map[string]bool{}
			if on, _, err := status(ctx, c); err == nil && on {
				if vnis, err := eidVNIs(ctx, c); err == nil {
					for _, v := range vnis {
						if rep, err := lispapi.NewServiceClient(c).LispAdjacenciesGet(ctx, &lispapi.LispAdjacenciesGet{Vni: v}); err == nil {
							for _, a := range rep.Adjacencies {
								cp[df6.U32(v)+"/"+eidString(a.Reid)+"/"+eidString(a.Leid)] = true
							}
						}
					}
				}
			}
			vr, err := svc.GpeFwdEntryVnisGet(ctx, &gpeapi.GpeFwdEntryVnisGet{})
			if err != nil {
				return nil, fmt.Errorf("gpe_fwd_entry_vnis_get: %w", err)
			}
			var out []*GpeFwdEntry
			for _, v := range vr.Vnis {
				er, err := svc.GpeFwdEntriesGet(ctx, &gpeapi.GpeFwdEntriesGet{Vni: v})
				if err != nil {
					return nil, fmt.Errorf("gpe_fwd_entries_get %d: %w", v, err)
				}
				for _, fe := range er.Entries {
					e := &GpeFwdEntry{Vni: fe.Vni, DpTable: fe.DpTable, Reid: eidString(fe.Reid), Leid: eidString(fe.Leid), Action: uint32(fe.Action)}
					if cp[df6.U32(e.GetVni())+"/"+e.GetReid()+"/"+e.GetLeid()] {
						continue
					}
					// gpe_fwd_entry_path_dump is unusable in VPP 26.06 (V9: its details carry
					// the message id without the plugin base), so pairs cannot be read back
					// and the descriptor is write-only (WriteOnly below).
					out = append(out, e)
				}
			}
			return out, nil
		},
		Owns:      func(e *GpeFwdEntry) bool { return ownsEID(scope, e.GetReid(), e.GetVni()) },
		WriteOnly: true,
	}, c)
}

// Register registers every LISP / LISP-GPE descriptor with r.
func Register(r scheduler.Registry, c vpp.Client, scope *df6.Scope) {
	r.Register(NewEnable(c))
	r.Register(NewGpeEnable(c))
	r.Register(NewLocatorSet(c, scope))
	r.Register(NewLocator(c, scope))
	r.Register(NewEidTableMap(c, scope))
	r.Register(NewLocalEid(c, scope))
	r.Register(NewMapResolver(c, scope))
	r.Register(NewMapServer(c, scope))
	r.Register(NewRemoteMapping(c, scope))
	r.Register(NewAdjacency(c, scope))
	r.Register(NewPitr(c))
	r.Register(NewGpeFwdEntry(c, scope))
}

// mappingEID is the EID of a non-src/dst mapping: VPP encodes it in seid (deid is only
// used, with seid, for src/dst mappings).
func mappingEID(r *lispapi.LispEidTableDetails) string {
	if r.IsSrcDst {
		return ""
	}
	return eidString(r.Seid)
}
