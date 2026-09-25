package desired

// F-loopback-bvi-gso-lldp-span: services.lldp → DF-7's lldp descriptors.
//
//	services.lldp (enabled)                 → lldp.global/global     (systemName, txHold, txIntervalSec;
//	                                                                  globals owner only, D-071)
//	services.lldp.interfaces[i] (enabled)   → lldp.interface/<if>    (port description, management
//	                                                                  IPv4/IPv6/OID; depends on
//	                                                                  interface/<if>)
//
// Both are write-only in VPP 26.06 (no getter for lldp_config; lldp_dump reports neighbours, not the
// configuration — D-063): DryRun notes `/services/lldp` as agent.write-only, Retrieve never reports
// it, and the live view is the LldpNeighbors RPC. A slot agent (not the globals owner) applies only
// the per-interface part and reports the VPP-global fields as agent.unsupported-field. `enabled:
// false` applies nothing.
//
// An unset systemName leaves VPP's current system name (VPP's default is none; the API cannot unset it).
// It is not taken from system.hostname here: the agent stores only its implemented domains, so a resync
// after a restart would not know the hostname and would change the value. Set systemName explicitly; neither
// the agent nor the API fills it in (questions file Q5, review L1).

import (
	"net/netip"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/lldp"
)

// Members of `services` this feature implements (F-rpf-adl-pbr's append-only ServicesMembers seam).
func init() {
	ServicesMembers["lldp"] = true
	ServicesMembers["nsim"] = true
}

// Rules of the feature's DryRun notes (the F-rpf-adl-pbr / P08 coverage rules the API's drift view skips).
const (
	lbgsWriteOnly   = "agent.write-only"
	lbgsUnsupported = "agent.unsupported-field"
)

// canonIP returns the canonical text of an IP address of the given family ("" when it is not one).
func canonIP(s string, v6 bool) string {
	a, err := netip.ParseAddr(s)
	if err != nil || a.Is6() != v6 || a.Zone() != "" {
		return ""
	}
	return a.String()
}

// Lldp emits the LLDP objects of l (see the file comment); globalsOwner is D-071's flag.
func Lldp(s Sink, l *vrxv1.LldpService, globalsOwner bool) {
	if l == nil || proto.Size(l) == 0 {
		return
	}
	pt := Ptr("services", "lldp")
	if !l.GetEnabled() {
		s.Warnf(pt, lbgsWriteOnly, "services.lldp is disabled: nothing is applied (VPP has no getter for LLDP either way; D-063)")
		return
	}
	if globalsOwner {
		g := lldp.Global{SystemName: l.GetSystemName(), TxHold: l.GetTxHold(), TxInterval: l.GetTxIntervalSec()}
		if err := g.Validate(); err != nil {
			s.Errorf(pt, "services.loopback-bvi-gso-lldp-span-lldp-global", "%v", err)
		} else {
			s.Add(lldp.KeyGlobal(), df7.Encode(g), pt)
		}
	} else {
		for _, f := range []struct {
			field string
			set   bool
		}{{"systemName", l.SystemName != nil}, {"txHold", l.TxHold != nil}, {"txIntervalSec", l.TxIntervalSec != nil}} {
			if f.set {
				s.Warnf(Ptr("services", "lldp", f.field), lbgsUnsupported,
					"services.lldp.%s is a VPP-global setting (lldp_config) that only the globals owner applies (D-071); this agent is not the globals owner, VPP keeps its current value", f.field)
			}
		}
	}
	seen := map[string]bool{}
	for i, e := range l.GetInterfaces() {
		ept := Ptr("services", "lldp", "interfaces", strconv.Itoa(i))
		name := e.GetInterface()
		if name == "" || seen[name] {
			s.Errorf(ept+"/interface", "services.loopback-bvi-gso-lldp-span-lldp-interface", "LLDP interface %q is empty or listed twice", name)
			continue
		}
		seen[name] = true
		v := lldp.Interface{Interface: name, PortDesc: e.GetPortDescription(), MgmtOID: e.GetMgmtOid()}
		if e.MgmtIpv4 != nil {
			if v.MgmtIP4 = canonIP(e.GetMgmtIpv4(), false); v.MgmtIP4 == "" {
				s.Errorf(ept+"/mgmtIpv4", "services.loopback-bvi-gso-lldp-span-lldp-interface", "%q is not an IPv4 address", e.GetMgmtIpv4())
				continue
			}
		}
		if e.MgmtIpv6 != nil {
			if v.MgmtIP6 = canonIP(e.GetMgmtIpv6(), true); v.MgmtIP6 == "" {
				s.Errorf(ept+"/mgmtIpv6", "services.loopback-bvi-gso-lldp-span-lldp-interface", "%q is not an IPv6 address", e.GetMgmtIpv6())
				continue
			}
		}
		if err := v.Validate(); err != nil {
			s.Errorf(ept, "services.loopback-bvi-gso-lldp-span-lldp-interface", "%v", err)
			continue
		}
		s.Add(lldp.KeyInterface(name), df7.Encode(v), ept)
	}
	s.Warnf(pt, lbgsWriteOnly, "services.lldp is applied write-only: VPP 26.06 has no getter for lldp_config or an interface's LLDP settings (D-063); the live view is GET /api/v1/state/lldp/neighbors")
}
