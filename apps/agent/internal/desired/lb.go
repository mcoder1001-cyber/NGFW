package desired

// F-lb: services.lb → descriptors/lb (DF-7; D-104: use, do not rebuild).
//
//	services.lb.settings             → lb.conf/global                          (globals owner only, D-071)
//	services.lb.vips.<name>          → lb.vip/<prefix>/<protocol>/<port>       (depends on lb.conf, optional)
//	services.lb.vips.<name>.servers  → lb.as/<prefix>/<protocol>/<port>/<addr> (depends on its VIP)
//	services.lb.natInterfaces[i]     → lb.intf-nat/<if>/<ip4|ip6>             (depends on interface/<if>)
//
// Every lb object is write-only (VPP 26.06 corrupts lb_vip_details, V20, D-063): DryRun notes /services/lb as
// agent.write-only and Retrieve never reports services.lb; the live view is the LbState RPC (internal/agent/rpc_lb.go).
// A non-owner agent (VRX_GLOBALS_OWNER=0) never sets lb_conf: settings are reported as agent.unsupported-field there,
// and VIPs use whatever the globals owner configured (D-071).

import (
	"fmt"
	"net/netip"
	"strconv"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/lb"
)

// Rules of the lb projection (ValidationIssue.rule).
const (
	lbRuleSpec       = "services.lb-spec"
	lbRuleReserved   = "services.lb-vip-reserved"
	lbRuleWriteOnly  = "agent.write-only"
	lbRuleUnsupport  = "agent.unsupported-field"
	lbServicesMember = "lb"
)

func init() { ServicesMembers[lbServicesMember] = true }

// lbReportUnsupportedServices reports the services members no feature of this build implements; set by the merge
// seam (lb_services_seam.go) until F-rpf-adl-pbr's projectServices is on the base.
var lbReportUnsupportedServices func(Sink, *vrxv1.ServicesConfig)

// LbEnv is what the lb projection needs from the wiring.
type LbEnv struct {
	// GlobalsOwner: this agent applies VPP-global settings (lb_conf, D-071).
	GlobalsOwner bool
}

// LbVIPOf converts one configured VIP into the descriptor's spec, canonical (masked prefix, lower-case IPv6).
func LbVIPOf(v *vrxv1.LbVip) (lb.VIPSpec, error) {
	p, err := netip.ParsePrefix(v.GetPrefix())
	if err != nil {
		return lb.VIPSpec{}, fmt.Errorf("prefix %q: %w", v.GetPrefix(), err)
	}
	if p != p.Masked() {
		return lb.VIPSpec{}, fmt.Errorf("prefix %s has host bits set (want %s)", p, p.Masked())
	}
	if p.Addr().Is4() && lb.GCSentinelRange.Overlaps(p) {
		return lb.VIPSpec{}, errReservedVIP
	}
	proto := v.GetProtocol()
	if proto == "" {
		proto = lb.ProtoAny
	}
	if v.GetPort() > 65535 || v.GetTargetPort() > 65535 || v.GetNodePort() > 65535 || v.GetDscp() > 63 {
		return lb.VIPSpec{}, fmt.Errorf("port, target port, node port or dscp out of range")
	}
	s := lb.VIPSpec{
		VIP:                 lb.VIP{Prefix: p.String(), Protocol: proto, Port: uint16(v.GetPort())}, //nolint:gosec // range-checked above
		Encap:               v.GetEncap(),
		DSCP:                uint8(v.GetDscp()),        //nolint:gosec // range-checked above
		TargetPort:          uint16(v.GetTargetPort()), //nolint:gosec // range-checked above
		NodePort:            uint16(v.GetNodePort()),   //nolint:gosec // range-checked above
		NewFlowsTableLength: v.GetNewFlowsTableLength(),
		SrcIPSticky:         v.GetSrcIpSticky(),
	}
	if s.Encap == lb.EncapNAT4 || s.Encap == lb.EncapNAT6 {
		s.SrvType = v.GetSrvType()
		if s.SrvType == "" {
			s.SrvType = lb.SrvClusterIP // schema default
		}
	}
	if s.NewFlowsTableLength == 0 {
		s.NewFlowsTableLength = 1024 // schema default
	}
	return s, s.Validate()
}

var errReservedVIP = fmt.Errorf("VIPs in %s are reserved (0.0.0.0/32 is the agent's lb garbage-collection sentinel, D-090)", lb.GCSentinelRange)

// Lb emits the lb objects of svc.lb (see the file comment) and, until F-rpf-adl-pbr's seam is on the base, the
// agent.unsupported-field notes of the services members no feature implements.
func Lb(s Sink, svc *vrxv1.ServicesConfig, env LbEnv) {
	if lbReportUnsupportedServices != nil {
		lbReportUnsupportedServices(s, svc)
	}
	l := svc.GetLb()
	if l == nil {
		return
	}
	root := Ptr("services", "lb")
	if st := l.GetSettings(); st != nil {
		pt := Ptr("services", "lb", "settings")
		if !env.GlobalsOwner {
			s.Warnf(pt, lbRuleUnsupport, "services.lb.settings is VPP-global (lb_conf) and only the globals owner applies it (D-071); this agent is not the globals owner, VPP keeps its current values")
		} else {
			c := lb.Conf{IP4Src: st.GetIp4Source(), IP6Src: st.GetIp6Source(), StickyBucketsPerCore: st.GetFlowBuckets(), FlowTimeout: st.GetFlowTimeoutSec()}
			if err := c.Validate(); err != nil {
				s.Errorf(pt, lbRuleSpec, "%v", err)
			} else {
				s.Add(lb.KeyConf(), df7.Encode(c), pt)
			}
		}
	}
	for _, name := range sortedKeys(l.GetVips()) {
		v := l.GetVips()[name]
		pt := Ptr("services", "lb", "vips", name)
		spec, err := LbVIPOf(v)
		switch {
		case err == errReservedVIP:
			s.Errorf(Ptr("services", "lb", "vips", name, "prefix"), lbRuleReserved, "%v", err)
			continue
		case err != nil:
			s.Errorf(pt, lbRuleSpec, "VIP %q: %v", name, err)
			continue
		}
		s.Add(lb.KeyVIP(spec.VIP), df7.Encode(spec), pt)
		for i, srv := range v.GetServers() {
			sp := Ptr("services", "lb", "vips", name, "servers", strconv.Itoa(i))
			a, err := netip.ParseAddr(srv.GetAddress())
			if err != nil {
				s.Errorf(sp+"/address", lbRuleSpec, "server %q: %v", srv.GetAddress(), err)
				continue
			}
			as := lb.AS{VIP: spec.VIP, Address: a.String(), FlushOnDelete: srv.GetFlushOnDelete()}
			if err := as.Validate(); err != nil {
				s.Errorf(sp, lbRuleSpec, "%v", err)
				continue
			}
			s.Add(lb.KeyAS(spec.VIP, as.Address), df7.Encode(as), sp)
		}
	}
	for i, n := range l.GetNatInterfaces() {
		pt := Ptr("services", "lb", "natInterfaces", strconv.Itoa(i))
		in := lb.IntfNat{Interface: n.GetInterface(), Family: n.GetFamily()}
		if err := in.Validate(); err != nil {
			s.Errorf(pt, lbRuleSpec, "%v", err)
			continue
		}
		s.Add(lb.KeyIntfNat(in.Interface, in.Family), df7.Encode(in), pt)
	}
	if len(l.GetVips()) > 0 || len(l.GetNatInterfaces()) > 0 || l.GetSettings() != nil {
		s.Warnf(root, lbRuleWriteOnly, "services.lb is applied write-only: VPP 26.06 cannot report lb objects (V20, D-063); see GET /api/v1/state/lb/vips for the live view")
	}
}
