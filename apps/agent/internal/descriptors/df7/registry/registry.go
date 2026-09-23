// Package registry is the DF-7 entry in the agent's descriptor registry list: one call that
// registers every DF-7 plugin (policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls) and — only
// for the globals owner (D-071) — their VPP-global descriptors (lb.conf, lldp.global,
// bfd.echo-source, igmp.group-prefix). P05/P08 wire it next to acl.Register and DF-1…DF-8.
package registry

import (
	"ngfw/agent/internal/descriptors/bfd"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/igmp"
	"ngfw/agent/internal/descriptors/lb"
	"ngfw/agent/internal/descriptors/lldp"
	"ngfw/agent/internal/descriptors/mpls"
	"ngfw/agent/internal/descriptors/policer"
	"ngfw/agent/internal/descriptors/qos"
	"ngfw/agent/internal/descriptors/span"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Config is what the agent passes to Register.
type Config struct {
	// Owner is VRX_OWNER (tests: VRX_TEST_PREFIX).
	Owner string
	// GlobalsOwner registers the VPP-global descriptors (agent config globalsOwner: true; never
	// a test slot on the shared host).
	GlobalsOwner bool
	// BFDSecrets resolves BFD conf-key secrets (the agent's encrypted store).
	BFDSecrets bfd.Secrets
	// Options are passed to every plugin (interface key scheme, id range, classify resolver).
	Options []df7.Option
}

// Register registers every DF-7 descriptor in dependency-friendly order.
func Register(r scheduler.Registry, c vpp.Client, cfg Config) {
	o := cfg.Options
	if cfg.GlobalsOwner {
		lb.RegisterGlobals(r, c, cfg.Owner, o...)
		lldp.RegisterGlobals(r, c, cfg.Owner, o...)
		bfd.RegisterGlobals(r, c, cfg.Owner, o...)
		igmp.RegisterGlobals(r, c, cfg.Owner, o...)
	}
	mpls.Register(r, c, cfg.Owner, o...)
	policer.Register(r, c, cfg.Owner, o...)
	qos.Register(r, c, cfg.Owner, o...)
	lb.Register(r, c, cfg.Owner, o...)
	span.Register(r, c, cfg.Owner, o...)
	lldp.Register(r, c, cfg.Owner, o...)
	bfd.Register(r, c, cfg.Owner, cfg.BFDSecrets, o...)
	vrrp.Register(r, c, cfg.Owner, o...)
	igmp.Register(r, c, cfg.Owner, o...)
}
