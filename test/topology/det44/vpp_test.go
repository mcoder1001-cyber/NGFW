package det44topo

// NAT access of the test itself (never of the product): dumps of the slot's det44 / map / cnat / dslite objects
// (the "nothing of ours remains" checks), the simulated loss behind the agent's back, and the two VPP-global
// fixtures (D-071: a slot agent never sets a global; the test does, under the exclusive globals lock, D-167):
// the det44 plugin enable (irreversible in VPP 26.06, V9 — opt-in) and the DS-Lite AFTR address (restored).

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	vppapi "go.fd.io/govpp/api"

	cnatapi "ngfw/agent/binapi/cnat"
	"ngfw/agent/binapi/det44"
	dsliteapi "ngfw/agent/binapi/dslite"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	maps "ngfw/agent/binapi/map"
)

// scope is what "ours" means on the shared VPP: the slot's IPv4 block, its domain tag prefix and its interfaces.
type scope struct {
	s     slot
	v4    netip.Prefix
	tag   string            // "<prefix>:"
	ifIdx map[uint32]string // sw_if_index → name of the slot's interfaces
}

func (sc scope) owns4(a ip_types.IP4Address) bool { return sc.v4.Contains(netip.AddrFrom4(a)) }

type natObjects struct {
	det44If  []*det44.Det44InterfaceDetails
	det44Map []*det44.Det44MapDetails
	domains  []*maps.MapDomainDetails
	trs      []*cnatapi.CnatTranslationDetails
	pool     []*dsliteapi.DsliteAddressDetails
}

func (o natObjects) count() int {
	return len(o.det44If) + len(o.det44Map) + len(o.domains) + len(o.trs) + len(o.pool)
}

func (o natObjects) String() string {
	var parts []string
	for _, i := range o.det44If {
		parts = append(parts, fmt.Sprintf("det44-if(sw_if_index=%d in=%v out=%v)", i.SwIfIndex, i.IsInside, i.IsOutside))
	}
	for _, m := range o.det44Map {
		parts = append(parts, fmt.Sprintf("det44-map(%s/%d→%s/%d ratio=%d ports/host=%d sessions=%d)", netip.AddrFrom4(m.InAddr), m.InPlen, netip.AddrFrom4(m.OutAddr), m.OutPlen, m.SharingRatio, m.PortsPerHost, m.SesNum))
	}
	for _, d := range o.domains {
		parts = append(parts, fmt.Sprintf("map-domain(%d tag=%q ip4=%s/%d ip6=%s/%d src=%s/%d ea=%d psid=%d+%d)", d.DomainIndex, strings.TrimRight(d.Tag, "\x00"),
			netip.AddrFrom4(d.IP4Prefix.Address), d.IP4Prefix.Len, netip.AddrFrom16(d.IP6Prefix.Address), d.IP6Prefix.Len, netip.AddrFrom16(d.IP6Src.Address), d.IP6Src.Len, d.EaBitsLen, d.PsidOffset, d.PsidLength))
	}
	for _, t := range o.trs {
		tr := t.Translation
		var paths []string
		for _, p := range tr.Paths {
			paths = append(paths, fmt.Sprintf("%s:%d", addrString(p.DstEp.Addr), p.DstEp.Port))
		}
		parts = append(parts, fmt.Sprintf("cnat-translation(id=%d vip=%s:%d proto=%d lb=%d paths=%v)", tr.ID, addrString(tr.Vip.Addr), tr.Vip.Port, tr.IPProto, tr.LbType, paths))
	}
	for _, p := range o.pool {
		parts = append(parts, "dslite-pool("+netip.AddrFrom4(p.IPAddress).String()+")")
	}
	return strings.Join(parts, " ")
}

func addrString(a ip_types.Address) string {
	if a.Af == ip_types.ADDRESS_IP6 {
		return netip.AddrFrom16(a.Un.GetIP6()).String()
	}
	return netip.AddrFrom4(a.Un.GetIP4()).String()
}

// ours dumps every det44 / map / cnat / dslite object of the slot (all owners are dumped; filtered by our scope).
func (sc scope) ours(t *testing.T, conn vppapi.Connection) natObjects {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	var o natObjects
	chk := func(err error, what string) {
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	dsvc := det44.NewServiceClient(conn)
	s1, err := dsvc.Det44InterfaceDump(ctx, &det44.Det44InterfaceDump{})
	chk(err, "det44_interface_dump")
	ifs, err := drain(s1.Recv)
	chk(err, "det44_interface_dump")
	for _, i := range ifs {
		if _, ok := sc.ifIdx[uint32(i.SwIfIndex)]; ok {
			o.det44If = append(o.det44If, i)
		}
	}
	s2, err := dsvc.Det44MapDump(ctx, &det44.Det44MapDump{})
	chk(err, "det44_map_dump")
	ms, err := drain(s2.Recv)
	chk(err, "det44_map_dump")
	for _, m := range ms {
		if sc.owns4(m.InAddr) || sc.owns4(m.OutAddr) {
			o.det44Map = append(o.det44Map, m)
		}
	}
	s3, err := maps.NewServiceClient(conn).MapDomainDump(ctx, &maps.MapDomainDump{})
	chk(err, "map_domain_dump")
	ds, err := drain(s3.Recv)
	chk(err, "map_domain_dump")
	for _, d := range ds {
		if strings.HasPrefix(strings.TrimRight(d.Tag, "\x00"), sc.tag) {
			o.domains = append(o.domains, d)
		}
	}
	s4, err := cnatapi.NewServiceClient(conn).CnatTranslationDump(ctx, &cnatapi.CnatTranslationDump{})
	chk(err, "cnat_translation_dump")
	trs, err := drain(s4.Recv)
	chk(err, "cnat_translation_dump")
	for _, tr := range trs {
		if tr.Translation.Vip.Addr.Af == ip_types.ADDRESS_IP4 && sc.owns4(tr.Translation.Vip.Addr.Un.GetIP4()) {
			o.trs = append(o.trs, tr)
		}
	}
	s5, err := dsliteapi.NewServiceClient(conn).DsliteAddressDump(ctx, &dsliteapi.DsliteAddressDump{})
	chk(err, "dslite_address_dump")
	pool, err := drain(s5.Recv)
	chk(err, "dslite_address_dump")
	for _, p := range pool {
		if sc.owns4(p.IPAddress) {
			o.pool = append(o.pool, p)
		}
	}
	return o
}

// loss deletes every object of the slot through the binary API (dependents first), the way a crash would lose
// them — never through the agent. Returns one evidence line per call.
func (sc scope) loss(t *testing.T, conn vppapi.Connection) []string {
	t.Helper()
	return sc.lossOpt(t, conn, true)
}

// lossNoPool is loss without any DS-Lite pool delete (D-211: pool delete + re-add crashes VPP 26.06); a pool of the slot
// is only logged.
func (sc scope) lossNoPool(t *testing.T, conn vppapi.Connection) []string {
	t.Helper()
	return sc.lossOpt(t, conn, false)
}

func (sc scope) lossOpt(t *testing.T, conn vppapi.Connection, pools bool) []string {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	o := sc.ours(t, conn)
	var ev []string
	dsvc := det44.NewServiceClient(conn)
	for _, m := range o.det44Map {
		if _, err := dsvc.Det44AddDelMap(ctx, &det44.Det44AddDelMap{IsAdd: false, InAddr: m.InAddr, InPlen: m.InPlen, OutAddr: m.OutAddr, OutPlen: m.OutPlen}); err != nil {
			t.Fatalf("loss: det44_add_del_map del: %v", err)
		}
		ev = append(ev, fmt.Sprintf("det44_add_del_map is_add=false %s/%d → %s/%d → ok", netip.AddrFrom4(m.InAddr), m.InPlen, netip.AddrFrom4(m.OutAddr), m.OutPlen))
	}
	for _, i := range o.det44If {
		if _, err := dsvc.Det44InterfaceAddDelFeature(ctx, &det44.Det44InterfaceAddDelFeature{IsAdd: false, IsInside: i.IsInside, SwIfIndex: i.SwIfIndex}); err != nil {
			t.Fatalf("loss: det44_interface_add_del_feature del: %v", err)
		}
		ev = append(ev, fmt.Sprintf("det44_interface_add_del_feature is_add=false sw_if_index=%d (%s) inside=%v → ok", i.SwIfIndex, sc.ifIdx[uint32(i.SwIfIndex)], i.IsInside))
	}
	// the MAP feature on our interfaces (write-only: no dump) is disabled too, so the resync must re-enable it
	msvc := maps.NewServiceClient(conn)
	for idx, name := range sc.ifIdx {
		if _, err := msvc.MapIfEnableDisable(ctx, &maps.MapIfEnableDisable{SwIfIndex: interface_types.InterfaceIndex(idx), IsEnable: false}); err != nil {
			t.Fatalf("loss: map_if_enable_disable disable %s: %v", name, err)
		}
		ev = append(ev, fmt.Sprintf("map_if_enable_disable is_enable=false sw_if_index=%d (%s) → ok", idx, name))
	}
	for _, d := range o.domains { // deleting a domain drops its rules
		if _, err := msvc.MapDelDomain(ctx, &maps.MapDelDomain{Index: d.DomainIndex}); err != nil {
			t.Fatalf("loss: map_del_domain %d: %v", d.DomainIndex, err)
		}
		ev = append(ev, fmt.Sprintf("map_del_domain index=%d (tag %q) → ok", d.DomainIndex, strings.TrimRight(d.Tag, "\x00")))
	}
	for _, tr := range o.trs {
		if _, err := cnatapi.NewServiceClient(conn).CnatTranslationDel(ctx, &cnatapi.CnatTranslationDel{ID: tr.Translation.ID}); err != nil {
			t.Fatalf("loss: cnat_translation_del %d: %v", tr.Translation.ID, err)
		}
		ev = append(ev, fmt.Sprintf("cnat_translation_del id=%d (vip %s:%d) → ok", tr.Translation.ID, addrString(tr.Translation.Vip.Addr), tr.Translation.Vip.Port))
	}
	for _, p := range o.pool {
		if !pools {
			ev = append(ev, fmt.Sprintf("DS-Lite pool %s left alone (D-211: no pool delete)", netip.AddrFrom4(p.IPAddress)))
			continue
		}
		if _, err := dsliteapi.NewServiceClient(conn).DsliteAddDelPoolAddrRange(ctx, &dsliteapi.DsliteAddDelPoolAddrRange{StartAddr: p.IPAddress, EndAddr: p.IPAddress, IsAdd: false}); err != nil {
			t.Fatalf("loss: dslite_add_del_pool_addr_range del %s: %v", netip.AddrFrom4(p.IPAddress), err)
		}
		ev = append(ev, fmt.Sprintf("dslite_add_del_pool_addr_range is_add=false %s → ok", netip.AddrFrom4(p.IPAddress)))
	}
	return ev
}

// det44Sessions dumps the det44 sessions of one inside user (binary API; the agent's RPC is checked separately).
func det44Sessions(t *testing.T, conn vppapi.Connection, user string) []*det44.Det44SessionDetails {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	a := netip.MustParseAddr(user).As4()
	st, err := det44.NewServiceClient(conn).Det44SessionDump(ctx, &det44.Det44SessionDump{UserAddr: a})
	if err != nil {
		t.Fatalf("det44_session_dump: %v", err)
	}
	out, err := drain(st.Recv)
	if err != nil {
		t.Fatalf("det44_session_dump: %v", err)
	}
	return out
}

// ---- global fixtures (exclusive globals lock held by the caller, D-167) ---------------------------

// enableDet44 is the V9 window step: det44_plugin_enable_disable(enable=1, VRFs 0/0). Returns whether the plugin
// was already enabled (bare retval 1 = "plugin already enabled!", det44.c) — the call then changed nothing. The
// plugin is NEVER disabled (det44_plugin_disable segfaults, V9 / D-068): it stays enabled until the next VPP restart.
func enableDet44(t *testing.T, conn vppapi.Connection) (wasOn bool) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	_, err := det44.NewServiceClient(conn).Det44PluginEnableDisable(ctx, &det44.Det44PluginEnableDisable{Enable: true})
	if err == nil {
		return false
	}
	var ve vppapi.VPPApiError
	if errors.As(err, &ve) && int32(ve) == 1 {
		return true
	}
	t.Fatalf("det44_plugin_enable_disable enable=1: %v", err)
	return false
}

type aftr struct {
	ip4 ip_types.IP4Address
	ip6 ip_types.IP6Address
}

func (a aftr) String() string {
	return fmt.Sprintf("ipv6=%s ipv4=%s", netip.AddrFrom16(a.ip6), netip.AddrFrom4(a.ip4))
}

func getAftr(t *testing.T, conn vppapi.Connection) aftr {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	r, err := dsliteapi.NewServiceClient(conn).DsliteGetAftrAddr(ctx, &dsliteapi.DsliteGetAftrAddr{})
	if err != nil {
		t.Fatalf("dslite_get_aftr_addr: %v", err)
	}
	return aftr{ip4: r.IP4Addr, ip6: r.IP6Addr}
}

func setAftr(t *testing.T, conn vppapi.Connection, a aftr) {
	t.Helper()
	ctx, cancel := ctx10()
	defer cancel()
	if _, err := dsliteapi.NewServiceClient(conn).DsliteSetAftrAddr(ctx, &dsliteapi.DsliteSetAftrAddr{IP4Addr: a.ip4, IP6Addr: a.ip6}); err != nil {
		t.Fatalf("dslite_set_aftr_addr %s: %v", a, err)
	}
}

// aftrFixture sets the slot's AFTR address for the test (only when VPP has none: another owner's is left alone —
// the test then omits nat.dslite.aftr) and restores exactly the previous value in Cleanup (shared-host-rules §7).
// Returns the IPv6 the document may require ("" = omit).
func aftrFixture(t *testing.T, conn vppapi.Connection, want string) string {
	t.Helper()
	prev := getAftr(t, conn)
	if !netip.AddrFrom16(prev.ip6).IsUnspecified() {
		t.Logf("dslite AFTR held by another owner (%s): nat.dslite.aftr omitted from the document", prev)
		return ""
	}
	a := aftr{ip6: netip.MustParseAddr(want).As16()}
	setAftr(t, conn, a)
	t.Logf("fixture: dslite_set_aftr_addr %s (previous %s; restored in Cleanup)", a, prev)
	t.Cleanup(func() {
		if cur := getAftr(t, conn); cur == a {
			setAftr(t, conn, prev)
			t.Logf("fixture: AFTR restored to the previous value (%s); dslite_get_aftr_addr now %s", prev, getAftr(t, conn))
		} else {
			t.Logf("fixture: AFTR is %s (not ours any more): left alone", cur)
		}
	})
	return want
}

// ---- static-route cleanup (D-216) ------------------------------------------------------------------

// dropSlotRoute removes the slot's own CE static route (table 0, exact prefix) through ip_route_add_del when a
// failed step left it behind (the agent's rev6 cleanup never ran). Only the exact slot prefix is touched; a
// lookup that finds nothing exact (retval != 0) means the route is already gone. Returns an evidence line.
func dropSlotRoute(t *testing.T, conn vppapi.Connection, pfx string) string {
	t.Helper()
	vp, err := ip_types.ParsePrefix(pfx)
	if err != nil {
		return fmt.Sprintf("route cleanup %s: bad prefix: %v", pfx, err)
	}
	ctx, cancel := ctx10()
	defer cancel()
	svc := ip.NewServiceClient(conn)
	if _, err := svc.IPRouteLookup(ctx, &ip.IPRouteLookup{TableID: 0, Exact: 1, Prefix: vp}); err != nil {
		return fmt.Sprintf("route cleanup %s: not in table 0 (ip_route_lookup exact: %v)", pfx, err)
	}
	_, err = svc.IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: false, Route: ip.IPRoute{TableID: 0, Prefix: vp}})
	return fmt.Sprintf("route cleanup: ip_route_add_del is_add=0 table 0 %s → err=%v", pfx, err)
}
