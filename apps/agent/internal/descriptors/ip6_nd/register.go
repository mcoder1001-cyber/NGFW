package ip6nd

import (
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the default ip6_nd descriptors (ra-config, ra-prefix, dad) with r.
// ip6-nd.proxy is deliberately NOT registered here (D-064): its first host run aborted the
// shared VPP 26.06 (DF-2-questions.md #1); it is opt-in through RegisterProxyNd until the
// crash is understood.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(NewRaConfig(c, owner, opts...))
	r.Register(NewRaPrefix(c, owner, opts...))
	r.Register(NewDad(c))
}

// RegisterProxyNd registers the opt-in ip6-nd.proxy descriptor (unverified on the host,
// see Register).
func RegisterProxyNd(r scheduler.Registry, c vpp.Client, owner string, opts ...df2.Option) {
	r.Register(NewProxyNd(c, owner, opts...))
}
