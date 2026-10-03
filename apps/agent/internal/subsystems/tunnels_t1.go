package subsystems

// S-tunnels-contract (T1): the tunnel kinds VPP names itself — VXLAN-GPE, GTP-U, L2TPv3, PPPoE
// server sessions — and IPIP 6RD, appended to F-tunnels' `tunnels` family. Their DF-6 descriptors
// key a tunnel by its configuration name (the owner tag); the interface alias interface/<name> is
// derived from the device class (iface.RegisterKind, TD-11c 3.1c) so an interface's attributes are
// deleted before the tunnel. gtpu.forward shares the GTPU class with gtpu.tunnel and stays a
// TD-11c gap (it needs a KeyProvider in descriptors/gtpu; not projected by this row).

import (
	"ngfw/agent/internal/descriptors/gtpu"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/ipip"
	"ngfw/agent/internal/descriptors/l2tp"
	"ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/descriptors/vxlan_gpe"
	"ngfw/agent/internal/scheduler"
)

// tunnelsT1Descriptors are the T1 descriptors of the `tunnels` domain.
var tunnelsT1Descriptors = []string{ipip.SixrdName, vxlan_gpe.TunnelName, gtpu.TunnelName, l2tp.TunnelName, pppoe.SessionName}

func init() {
	Domains[Tunnels] = append(Domains[Tunnels], tunnelsT1Descriptors...)
	// VPP 26.06 device classes (VNET_DEVICE_CLASS .name) of the interfaces these descriptors create.
	iface.RegisterKind("VXLAN_GPE", vxlan_gpe.TunnelName)
	iface.RegisterKind("GTPU", gtpu.TunnelName)
	iface.RegisterKind("L2TPv3", l2tp.TunnelName)
	iface.RegisterKind("PPPoE", pppoe.SessionName)
}

// registerTunnelsT1 registers the T1 tunnel descriptors (tolerant Retrieve when the plugin is absent,
// as the F-tunnels kinds). Called from register() under the F-tunnels anchor (subsystems.go).
func registerTunnelsT1(r scheduler.Registry, w *Wiring) {
	c, owner := w.env.Client, w.env.Owner
	r.Register(tagOwned{tunnelTolerant{Descriptor: ipip.NewSixrd(c, owner), c: c, dump: "ipip_tunnel_dump"}})
	r.Register(tagOwned{tunnelTolerant{Descriptor: vxlan_gpe.NewTunnel(c, owner), c: c, dump: "vxlan_gpe_tunnel_v2_dump"}})
	r.Register(tagOwned{tunnelTolerant{Descriptor: gtpu.NewTunnel(c, owner), c: c, dump: "gtpu_tunnel_v2_dump"}})
	r.Register(tagOwned{tunnelTolerant{Descriptor: l2tp.NewTunnel(c, owner), c: c, dump: "sw_if_l2tpv3_tunnel_dump"}})
	r.Register(tagOwned{tunnelTolerant{Descriptor: pppoe.NewSession(c, owner), c: c, dump: "pppoe_session_dump"}})
}

// tagOwned declares the TD-11b protocol (dfkit/persist) for a DF-6 interface descriptor whose package
// does not declare it itself: ownership is the owner tag VPP carries on the interface ("<owner>:<id>"),
// no claim or boot store is written — the same protocol gre/ipip/vxlan declare in their packages.
type tagOwned struct{ tunnelTolerant }

// RecordsNoOwnership implements the dfkit/persist protocol.
func (tagOwned) RecordsNoOwnership() {}

// Unwrap exposes the decorated descriptor.
func (t tagOwned) Unwrap() scheduler.Descriptor { return t.tunnelTolerant }
