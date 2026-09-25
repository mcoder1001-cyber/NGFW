package subsystems

import (
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dns"
	"ngfw/agent/internal/scheduler"
)

// registerDNSCache registers DF-8's VPP DNS cache descriptors (dns.name-server, dns.enable — VPP-global and
// write-only): as the globals owner through dns.RegisterGlobalsReady (D-071); every other agent registers them in
// require mode, so a document that enables the VPP cache fails there with dfkit.ErrNotGlobalsOwner (the plugin has
// no getter to check the requirement) instead of being silently ignored — and nothing VPP-wide is ever set.
//
// It returns the D-137 readiness fact (IPv4 server added + enabled on the running VPP), nil for a non-owner, which is
// never ready.
func registerDNSCache(r scheduler.Registry, env Env) *dns.Readiness {
	if env.GlobalsOwner {
		return dns.RegisterGlobalsReady(r, env.Client)
	}
	g := dfkit.GlobalsOwner(false)
	r.Register(dns.NewNameServer(env.Client, g))
	r.Register(dns.NewEnable(env.Client, g))
	return nil
}
