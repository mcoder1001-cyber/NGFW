package subsystems

// F-ipfix-sflow wiring (WBS D7.6): DF-8's ipfix, flowprobe and sflow descriptors (D-104, used as
// they are). The VPP-global singletons (ipfix.default-exporter = exporter 0, flowprobe.params,
// sflow.global) are registered with the globals role only in the globals owner's agent (D-071,
// ipfix/flowprobe/sflow.RegisterGlobals); every other agent registers them without the role, so it
// only *requires* them (Create succeeds when VPP already has the value, Delete is a no-op, Retrieve
// is write-only). ipfix.classify-* are registered (DF-2's classify store, P08) but belong to no
// domain: they are write-only (V16, D-063) and the configuration has no leaf for them.

import (
	"ngfw/agent/internal/descriptors/flowprobe"
	"ngfw/agent/internal/descriptors/ipfix"
	"ngfw/agent/internal/descriptors/sflow"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// ipfixSflowDescriptors are the descriptors of the `services` domain this build implements (D-041:
// Health.subsystems lists "services" once any of it is implemented).
var ipfixSflowDescriptors = []string{
	ipfix.NameDefaultExporter, ipfix.NameExporter,
	flowprobe.NameParams, flowprobe.NameInterface,
	sflow.NameGlobal, sflow.NameInterface,
}

// registerIpfixSflow registers the flow-export descriptors (called once from register).
func registerIpfixSflow(r scheduler.Registry, w *Wiring) error {
	c, owner := w.env.Client, w.env.Owner
	store, err := w.ClassifyStore()
	if err != nil {
		return err
	}
	desired.SetIpfixGlobalsOwner(w.env.GlobalsOwner)
	if w.env.GlobalsOwner {
		ipfix.RegisterGlobals(r, c)
		flowprobe.RegisterGlobals(r, c)
		sflow.RegisterGlobals(r, c)
	} else {
		r.Register(ipfix.NewDefaultExporter(c)) // requirement only (D-071)
		r.Register(flowprobe.NewParams(c))
		r.Register(sflow.NewGlobal(c))
	}
	ipfix.Register(r, c, store)
	flowprobe.Register(r, c, owner)
	sflow.Register(r, c, owner)
	return nil
}

// IpfixGlobalsOwner reports whether this agent sets exporter 0, flowprobe.params and sflow.global.
func (w *Wiring) IpfixGlobalsOwner() bool { return w.env.GlobalsOwner }
