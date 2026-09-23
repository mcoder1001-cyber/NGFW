package ip6nd

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register registers the ip6_nd descriptors (ra-config, ra-prefix, proxy, dad) with r.
func Register(r scheduler.Registry, c vpp.Client, owner string) {
	r.Register(NewRaConfig(c, owner))
	r.Register(NewRaPrefix(c, owner))
	r.Register(NewProxyNd(c, owner))
	r.Register(NewDad(c))
}
