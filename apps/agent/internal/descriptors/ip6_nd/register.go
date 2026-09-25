package ip6nd

import (
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the default per-owner ip6_nd descriptors (ra-config, ra-prefix) with r.
// The VPP-global ip6-nd.dad is NOT registered here (D-071): see RegisterGlobals.
// ip6-nd.proxy is deliberately NOT registered here (D-064): its first host run aborted the
// shared VPP 26.06 (DF-2-questions.md #1); it is opt-in through RegisterProxyNd until the
// crash is understood.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(NewRaConfig(c, owner, opts...))
	r.Register(NewRaPrefix(c, owner, opts...))
}

// RegisterGlobals registers ip6-nd.dad, a VPP-global switch. Only the designated globals
// owner (agent config globalsOwner: true, never a test slot on the shared host) may call it
// (D-071); a non-owner would set DAD for everyone and its Delete would disable it.
func RegisterGlobals(r scheduler.Registry, c vpp.Client) {
	r.Register(NewDad(c))
}

// RegisterProxyNd registers the opt-in ip6-nd.proxy descriptor (unverified on the host,
// see Register).
func RegisterProxyNd(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(NewProxyNd(c, owner, opts...))
}
