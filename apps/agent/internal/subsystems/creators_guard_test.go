package subsystems

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	afpacket "ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/gre"
	"ngfw/agent/internal/descriptors/gtpu"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/ipip"
	"ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/l2tp"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/descriptors/memif"
	"ngfw/agent/internal/descriptors/mpls"
	"ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/descriptors/vxlan_gpe"
	"ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// TD-11c fix round 1 (review F3): the delete-order obligation of every interface creator.
//
// 3.1c orders an interface's attributes (admin state, MTU, addresses, VRF binding, memberships)
// before its creator through the retrieved alias interface/<name> — whose Creator is set only when
// the creator is known: its VPP device class is mapped to it (iface.RegisterKind, or the built-in
// table in descriptors/interface/dump.go; sub-interfaces by type), or the creator provides
// interface/<name> itself (scheduler.KeyProvider). A creator with neither falls back to registration
// order, and when it is registered after DF-1 (every df6 family is) it is deleted before its
// attributes: their deletes fail on a stale sw_if_index and the transaction rolls back
// (F-vlan-qinq Q1). Every interface creator must therefore map its device class or provide the
// alias; knownAliasCreatorGaps lists the ones that do not yet, with the row that fixes them, and may
// only shrink.

type creatorCase struct {
	pkg  string // descriptors/<pkg>
	name string // descriptor name
	// devType is VPP 26.06's VNET_DEVICE_CLASS .name (sw_interface_details.interface_dev_type) of the
	// interfaces it creates; "" = a sub-interface (recognised by type); "-" = it creates the interface
	// untagged (no creator can be derived from the tag).
	devType string
	desc    func(c vpp.Client) scheduler.Descriptor
}

func interfaceCreators(t *testing.T) []creatorCase {
	t.Helper()
	coreReg := scheduler.NewRegistry()
	core.Register(coreReg, core.Env{Client: coretest.New(), Owner: "w1", Owned: ownertable.NewMemory()})
	loop, _ := coreReg.Get(core.LoopbackName)
	const o = "w1"
	return []creatorCase{
		{"core", core.LoopbackName, "Loopback", func(vpp.Client) scheduler.Descriptor { return loop }},
		{"af_packet", afpacket.HostInterfaceName, "af-packet", func(c vpp.Client) scheduler.Descriptor { return afpacket.New(c, o) }},
		{"interface", iface.SubinterfaceName, "", func(c vpp.Client) scheduler.Descriptor { return iface.NewSubinterface(c, o) }},
		{"tapv2", iface.TapName, "tap", func(c vpp.Client) scheduler.Descriptor { return tapv2.New(c, o) }},
		{"bond", iface.BondName, "bond", func(c vpp.Client) scheduler.Descriptor { return bond.NewBond(c, o) }},
		{"memif", iface.MemifName, "memif", func(c vpp.Client) scheduler.Descriptor { return memif.NewMemif(c, o) }},
		{"wireguard", wireguard.InterfaceName, "Wireguard Tunnel", func(c vpp.Client) scheduler.Descriptor {
			return wireguard.NewInterface(wireguard.Config{Client: c, Owner: o})
		}},
		{"ipsec", ipsec.ItfName, "IPSEC Tunnel", func(c vpp.Client) scheduler.Descriptor { return ipsec.NewItf(ipsec.Config{Client: c, Owner: o}) }},
		{"mpls", mpls.NameTunnel, "MPLS tunnel device", func(c vpp.Client) scheduler.Descriptor { return mpls.NewTunnel(c, o) }},
		{"gre", gre.TunnelName, "GRE tunnel device", func(c vpp.Client) scheduler.Descriptor { return gre.NewTunnel(c, o) }},
		{"ipip", ipip.TunnelName, "IPIP tunnel device", func(c vpp.Client) scheduler.Descriptor { return ipip.NewTunnel(c, o) }},
		{"ipip", ipip.SixrdName, "ip6ip-6rd", func(c vpp.Client) scheduler.Descriptor { return ipip.NewSixrd(c, o) }},
		{"vxlan", vxlan.TunnelName, "VXLAN", func(c vpp.Client) scheduler.Descriptor { return vxlan.NewTunnel(c, o) }},
		{"vxlan_gpe", vxlan_gpe.TunnelName, "VXLAN_GPE", func(c vpp.Client) scheduler.Descriptor { return vxlan_gpe.NewTunnel(c, o) }},
		{"gtpu", gtpu.TunnelName, "GTPU", func(c vpp.Client) scheduler.Descriptor { return gtpu.NewTunnel(c, o) }},
		{"gtpu", gtpu.ForwardName, "GTPU", func(c vpp.Client) scheduler.Descriptor { return gtpu.NewForward(c, o) }},
		{"l2tp", l2tp.TunnelName, "L2TPv3", func(c vpp.Client) scheduler.Descriptor { return l2tp.NewTunnel(c, o) }},
		{"pppoe", pppoe.SessionName, "PPPoE", func(c vpp.Client) scheduler.Descriptor { return pppoe.NewSession(c, o) }},
		{"lcp", lcp.NameItfPair, "-", func(c vpp.Client) scheduler.Descriptor { return lcp.NewItfPair(c, o) }},
	}
}

// knownAliasCreatorGaps are the interface creators that neither map their device class nor provide
// interface/<name> today (TD-11c fix round 1), with the row that fixes them. Shrink-only: a fixed
// creator must be removed from this list (the test says so).
var knownAliasCreatorGaps = map[string]string{
	mpls.NameTunnel:      "F-mpls-srmpls",
	gre.TunnelName:       "F-tunnels",
	ipip.TunnelName:      "F-tunnels",
	ipip.SixrdName:       "F-tunnels",
	vxlan.TunnelName:     "F-tunnels",
	vxlan_gpe.TunnelName: "F-tunnels",
	gtpu.TunnelName:      "F-tunnels",
	gtpu.ForwardName:     "F-tunnels (shares the GTPU class with gtpu.tunnel: needs a KeyProvider)",
	l2tp.TunnelName:      "F-tunnels",
	pppoe.SessionName:    "F-tunnels",
	lcp.NameItfPair:      "P12 (untagged VPP-side host tap: needs a KeyProvider)",
}

func TestEveryInterfaceCreatorNamesItsAlias(t *testing.T) {
	c := coretest.New()
	var report []string
	for _, cc := range interfaceCreators(t) {
		d := cc.desc(c)
		if d.Name() != cc.name {
			t.Fatalf("table stale: %s constructs %q", cc.name, d.Name())
		}
		_, provider := d.(scheduler.KeyProvider)
		mapped := false
		if cc.devType != "-" {
			det := &ifapi.SwInterfaceDetails{InterfaceDevType: cc.devType, SwIfIndex: 7, SupSwIfIndex: 7}
			if cc.devType == "" {
				det.Type, det.SupSwIfIndex, det.SubID = interface_types.IF_API_TYPE_SUB, 1, 100
			}
			mapped = iface.Kind(det) == cc.name
		}
		how := "—"
		switch {
		case mapped && provider:
			how = "device class mapped + KeyProvider"
		case mapped:
			how = "device class mapped"
		case provider:
			how = "KeyProvider"
		}
		owner, gap := knownAliasCreatorGaps[cc.name]
		switch {
		case !mapped && !provider && !gap:
			t.Errorf("interface creator %s (device class %q) neither maps its device class (iface.RegisterKind) nor provides interface/<name> (scheduler.KeyProvider): its attributes can be deleted after it (TD-11c 3.1c, F-vlan-qinq Q1)", cc.name, cc.devType)
		case (mapped || provider) && gap:
			t.Errorf("%s is fixed (%s): remove it from knownAliasCreatorGaps (shrink-only)", cc.name, how)
		}
		if gap {
			how = "GAP → " + owner
		}
		report = append(report, cc.name+": "+how)
	}
	t.Logf("interface creators and their alias creator:\n  %s", strings.Join(report, "\n  "))

	// Completeness: every descriptor package that creates interfaces (tags or sanitizes a new one)
	// is in the table. interface/ (AcquireAndTag's home), df6/ (the IfSpec library) and vpn/ (the
	// DF-5 tag helper) are libraries; their users are listed by package.
	covered := map[string]bool{"df6": true, "vpn": true}
	for _, cc := range interfaceCreators(t) {
		covered[cc.pkg] = true
	}
	marker := regexp.MustCompile(`AcquireAndTag\(|IfSpec\[|SwInterfaceTagAddDel\(|TagInterface\(|iface\.Tag\(|ifsanitize\.Acquire\(`)
	root := filepath.Join("..", "descriptors")
	var missing []string
	err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if strings.HasSuffix(e.Name(), "test") {
				return filepath.SkipDir // fakes and test helpers
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(p) //nolint:gosec // the repository's own sources
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		pkg := strings.Split(filepath.ToSlash(rel), "/")[0]
		if marker.Match(raw) && !covered[pkg] {
			missing = append(missing, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("interface-creating code outside the creator table (add the creator with its VPP device class): %v", missing)
	}
}
