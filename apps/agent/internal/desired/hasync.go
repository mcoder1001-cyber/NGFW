package desired

import (
	"net/netip"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/hasync"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// HaSync projects supported HA endpoints and reports explicit native support gaps.
func HaSync(s Sink, ds *ngfwv1.DesiredState, in map[string]bool) {
	if !in["ha"] {
		return
	}
	cluster := ds.GetHa().GetCluster()
	sync := cluster.GetStateSync()
	if sync == nil {
		return
	}
	for _, kind := range []struct {
		name    string
		enabled bool
	}{{"nat", sync.GetNat()}, {"acl", sync.GetAcl()}, {"ipsec", sync.GetIpsec()}} {
		if kind.enabled && !cluster.GetEnabled() {
			s.Errorf("/ha/cluster/stateSync/"+kind.name, "ha.ha-state-sync-cluster", "state synchronisation requires an enabled HA cluster")
		}
	}
	if !cluster.GetEnabled() {
		return
	}
	if sync.GetAcl() {
		s.Warnf("/ha/cluster/stateSync/acl", "ha.ha-state-sync-acl", "ACL session sync not supported by VPP (V2)")
	}
	if sync.GetIpsec() {
		s.Warnf("/ha/cluster/stateSync/ipsec", "ha.ha-state-sync-ipsec", "IPsec sequence/replay state sync not supported by VPP; native IKEv2 must re-key on failover")
	}
	if !sync.GetNat() {
		return
	}
	if ds.GetNat().GetMode() != "ei" {
		s.Warnf("/ha/cluster/stateSync/nat", "ha.ha-state-sync-ed", "NAT44-ED session sync not supported by VPP (V2)")
		return
	}
	l, f := sync.GetNatListener(), sync.GetNatFailover()
	if l == nil || f == nil {
		s.Errorf("/ha/cluster/stateSync", "ha.ha-state-sync-endpoints", "NAT44-EI requires explicit listener and failover endpoints")
		return
	}
	if l.GetPort() > 65535 || f.GetPort() > 65535 {
		s.Errorf("/ha/cluster/stateSync", "ha.ha-state-sync-port", "HA UDP port must be 1..65535")
		return
	}
	listener := hasync.Listener{Address: l.GetAddress(), Port: uint16(l.GetPort()), PathMtu: l.GetPathMtu()}                     //nolint:gosec // range checked above
	failover := hasync.Failover{Address: f.GetAddress(), Port: uint16(f.GetPort()), SessionRefreshSec: f.GetSessionRefreshSec()} //nolint:gosec // range checked above
	if err := listener.Validate(); err != nil {
		s.Errorf("/ha/cluster/stateSync/natListener", "ha.ha-state-sync-listener", "%v", err)
		return
	}
	if err := failover.Validate(); err != nil {
		s.Errorf("/ha/cluster/stateSync/natFailover", "ha.ha-state-sync-peer", "%v", err)
		return
	}
	if listener.Address == failover.Address {
		s.Errorf("/ha/cluster/stateSync/natFailover/address", "ha.ha-state-sync-peer", "HA peer must differ from listener")
		return
	}
	if cluster.GetInterface() == "" {
		s.Errorf("/ha/cluster/interface", "ha.ha-state-sync-interface", "dedicated sync interface required")
		return
	}
	owns := false
	for _, prefix := range ds.GetInterfaces()[cluster.GetInterface()].GetIpv4() {
		p, err := netip.ParsePrefix(prefix)
		if err == nil && p.Addr().String() == listener.Address {
			owns = true
		}
	}
	if !owns {
		s.Errorf("/ha/cluster/stateSync/natListener/address", "ha.ha-state-sync-local", "listener must be an address on the dedicated sync interface")
		return
	}
	if cluster.GetVrf() != "" && cluster.GetVrf() != "default" {
		s.Warnf("/ha/cluster/vrf", "ha.ha-state-sync-vrf", "native NAT HA API has no VRF binding; non-default sync VRF is not supported and endpoints are not applied")
		return
	}
	s.Warnf("/ha/cluster/stateSync/nat", "ha.ha-state-sync-security", "native NAT HA uses unauthenticated UDP; cluster secretRef protects config sync only")
	s.Add(hasync.ListenerKey, dfkit.Encode(listener), "/ha/cluster/stateSync/natListener")
	s.Add(hasync.FailoverKey, dfkit.Encode(failover), "/ha/cluster/stateSync/natFailover")
}

// AssembleHaSync updates only observable endpoint leaves; membership and flags belong to config sync.
func AssembleHaSync(ds *ngfwv1.DesiredState, kvs []scheduler.KV, in map[string]bool) {
	if !in["ha"] {
		return
	}
	ensure := func() *ngfwv1.HaCluster_StateSync {
		if ds.Ha == nil {
			ds.Ha = &ngfwv1.HaConfig{}
		}
		if ds.Ha.Cluster == nil {
			ds.Ha.Cluster = &ngfwv1.HaCluster{}
		}
		if ds.Ha.Cluster.StateSync == nil {
			ds.Ha.Cluster.StateSync = &ngfwv1.HaCluster_StateSync{}
		}
		return ds.Ha.Cluster.StateSync
	}
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case hasync.NameListener:
			l, err := natcommon.Decode[hasync.Listener](kv.Value)
			if err == nil {
				ensure().NatListener = &ngfwv1.HaNatListener{Address: proto.String(l.Address), Port: proto.Uint32(uint32(l.Port)), PathMtu: proto.Uint32(l.PathMtu)}
			}
		case hasync.NameFailover:
			f, err := natcommon.Decode[hasync.Failover](kv.Value)
			if err == nil {
				ensure().NatFailover = &ngfwv1.HaNatFailover{Address: proto.String(f.Address), Port: proto.Uint32(uint32(f.Port)), SessionRefreshSec: proto.Uint32(f.SessionRefreshSec)}
			}
		}
	}
}
