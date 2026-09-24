package subsystems

// F-bridge-l2 wiring (wave-A-hotspots A1): the l2 / l3xc descriptor families of DF-1 and the
// mactime family, all in the `interfaces` domain — the per-port leaves live on interfaces.<if>.l2
// and the records in routing.l2 (D-109 c); the API sends both domains in every transaction.
//
//	interfaces  F-bridge-l2: l2.bridge-domain, l2.bridge-domain-member, l2.xconnect, l2.fib-entry,
//	            l2.flags (no leaf: an owned leftover is removed), l2.vlan-tag-rewrite, l3xc.l3xc,
//	            mactime.range, mactime.enable (applied-once records in the persisted BootStore)

import (
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/descriptors/l3xc"
	"ngfw/agent/internal/descriptors/mactime"
	"ngfw/agent/internal/scheduler"
)

// Descriptor names of F-bridge-l2 (listed one per line in Domains[Interfaces]).
const (
	bridgeL2Domain     = l2.BridgeDomainName
	bridgeL2Member     = l2.MemberName
	bridgeL2Xconnect   = l2.XconnectName
	bridgeL2FibEntry   = l2.FibEntryName
	bridgeL2Flags      = l2.FlagsName
	bridgeL2TagRewrite = l2.VlanTagRewriteName
	bridgeL2L3xc       = l3xc.L3xcName
	bridgeL2MacRange   = mactime.RangeName
	bridgeL2MacEnable  = mactime.EnableName
)

// registerBridgeL2 registers the F-bridge-l2 families; mactime.enable keeps its D-076/D-080
// applied-once records in the owner's persisted BootStore (never in memory).
func (w *Wiring) registerBridgeL2(r scheduler.Registry) {
	c, owner := w.env.Client, w.env.Owner
	l2.Register(r, c, owner)
	l3xc.Register(r, c, owner)
	mactime.Register(r, c, owner, w.BootStore())
}
