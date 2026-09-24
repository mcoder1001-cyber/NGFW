package desired

// F-loopback-bvi-gso-lldp-span: services.nsim → descriptors/nsim (a lab tool, WBS D1.9).
//
//	services.nsim                      → nsim.config/global         (delay µs, bandwidth bit/s,
//	                                                                   packet size, packets per drop)
//	services.nsim.crossConnect {a, b}  → nsim.cross-connect/global  (depends on the model and both
//	                                                                   interface aliases)
//	services.nsim.outputInterfaces[i]  → nsim.output/<if>           (depends on the model and the alias)
//
// VPP holds one model and one cross-connect pair, so only the globals owner projects any of it (D-071);
// every other agent reports `/services/nsim` as agent.unsupported-field and applies nothing. All three
// are write-only (no getter; D-063): DryRun notes `/services/nsim` as agent.write-only.

import (
	"math"
	"strconv"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/nsim"
)

// NsimConfigOf converts the configuration's units to VPP's (delay ms → µs, Mbit/s → bit/s, drop
// fraction → packets per drop; 0 = no loss).
func NsimConfigOf(n *vrxv1.NsimService) nsim.Config {
	c := nsim.Config{
		DelayUsec:    uint32(math.Round(n.GetDelayMs() * 1000)),
		BandwidthBps: math.Round(n.GetBandwidthMbps() * 1e6),
		PacketSize:   n.GetPacketSize(),
	}
	if n.PacketSize == nil {
		c.PacketSize = 1500 // Zod default
	}
	if f := n.GetDropFraction(); f > 0 {
		if ppd := math.Round(1 / f); ppd <= math.MaxUint32 {
			c.PacketsPerDrop = uint32(ppd)
		}
	}
	return c
}

// Nsim emits the nsim objects of n (see the file comment).
func Nsim(s Sink, n *vrxv1.NsimService, globalsOwner bool) {
	if n == nil {
		return
	}
	pt := Ptr("services", "nsim")
	if !globalsOwner {
		s.Warnf(pt, lbgsUnsupported, "services.nsim is VPP-global (one delay model, one cross-connect pair) and only the globals owner applies it (D-071); this agent is not the globals owner")
		return
	}
	c := NsimConfigOf(n)
	if err := c.Validate(); err != nil {
		s.Errorf(pt, "services.loopback-bvi-gso-lldp-span-nsim-range", "%v", err)
		return
	}
	s.Add(nsim.ConfigKey(), c.Proto(), pt)
	if x := n.GetCrossConnect(); x != nil {
		xc := nsim.CrossConnect{A: x.GetA(), B: x.GetB()}
		if err := xc.Validate(); err != nil {
			s.Errorf(pt+"/crossConnect", "services.loopback-bvi-gso-lldp-span-nsim-interfaces", "%v", err)
		} else {
			s.Add(nsim.CrossConnectKey(), xc.Proto(), pt+"/crossConnect")
		}
	}
	for i, name := range n.GetOutputInterfaces() {
		s.Add(nsim.OutputKey(name), nsim.Output{Interface: name}.Proto(), Ptr("services", "nsim", "outputInterfaces", strconv.Itoa(i)))
	}
	s.Warnf(pt, lbgsWriteOnly, "services.nsim is applied write-only: VPP 26.06 has no getter for the nsim model, cross-connect or output feature (D-063)")
}
