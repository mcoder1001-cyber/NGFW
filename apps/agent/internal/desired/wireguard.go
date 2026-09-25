package desired

// F-wireguard: the builder of `vpn.wireguard` and its assembler.
//
//	vpn.wireguard.interfaces.<name>               → wireguard.interface/wg<instance>     (DF-5; private key as x25519 ref)
//	                                              + wireguard.meta/wg<instance>          (name, description, key ref, underlay VRF, flag)
//	  .enabled / .mtu / .vrf / .address[]         → interface.admin-state, interface.mtu, interface-ip.table, interface-ip
//	                                                on interface/wg<instance> (D-065 alias the WireGuard interface provides)
//	  .peers.<peer>                               → wireguard.peer/wg<instance>/<public key> (table_id = underlay VRF)
//	                                              + wireguard.meta/wg<instance>/<public key> (name, description, psk ref)
//	  .routeAllowedIps (every allowed IP)         → ip.route/<overlay table>/<prefix> via <next hop in the prefix> wg<instance>
//
// VPP 26.06 WireGuard interfaces are NBMA: a route through wg<N> without a next hop is a drop, and the
// adjacency is bound to the peer whose allowed IP covers the next hop (wireguard_if.c
// wg_if_update_adj → wg_peer_if_adj_change). The automatic routes therefore use the prefix's first
// address as next hop (the second one for 0.0.0.0/0 and ::/0, whose first address would make the path
// attached). Secret references (D-051) are mapped to the DF-5 references through WireguardEnv.SecretRef;
// no material passes through here. The interface-level leaves live in the `interfaces` domain's
// descriptors and the routes in `routing`'s: they are projected only when the transaction includes those
// domains (the API always sends all of them); the assembler moves them back under vpn.wireguard and out
// of `interfaces` / `routing.static`.

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"slices"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/scheduler"
)

// WireguardEnv is what the WireGuard builder needs from the agent wiring.
type WireguardEnv struct {
	// SecretRef maps a D-051 reference ("key/<name>", "psk/<name>") to the DF-5 reference the
	// descriptors take ("x25519:<base64 public key>", "hmac:<hex>"). nil = the agent has no secret
	// material (PENDING-secret-channel): every WireGuard interface fails validation at privateKeyRef.
	SecretRef func(ref string) (string, error)
}

// WireguardDescriptors are the descriptors of the vpn domain this builder produces (subsystems.Domains).
var WireguardDescriptors = []string{wireguard.InterfaceName, wireguard.PeerName, wireguard.MetaName}

// Rule ids of the builder's findings.
const (
	RuleWireguardSecret   = "agent.secret-unavailable"
	RuleWireguardValue    = "agent.unsupported-value"
	RuleWireguardDomain   = "agent.cross-domain"
	ruleUnsupportedField  = "agent.unsupported-field"
	ruleWireguardInstance = "vpn.wireguard-instance"
)

// Wireguard emits the objects of vpn.wireguard (see the file comment). in is the set of domains of the
// transaction; vrfID maps a VRF name to its table id (false: unknown).
func Wireguard(s Sink, ds *vrxv1.DesiredState, in map[string]bool, vrfID func(string) (uint32, bool), env WireguardEnv) {
	if !in["vpn"] {
		return
	}
	v := ds.GetVpn()
	// P11 (IPsec), F-pki and F-ra-vpn implement the rest of the domain; until they land, their leaves are
	// reported like P08's routing protocols (the drift view skips them). Each of those rows removes its
	// own pointer from this list.
	if v.GetIpsec() != nil && proto.Size(v.GetIpsec()) > 0 {
		s.Warnf(Ptr("vpn", "ipsec"), ruleUnsupportedField, "vpn.ipsec is not implemented by this agent build (P11)")
	}
	if v.GetPki() != nil && proto.Size(v.GetPki()) > 0 {
		s.Warnf(Ptr("vpn", "pki"), ruleUnsupportedField, "vpn.pki is not implemented by this agent build (F-pki)")
	}
	if len(v.GetRemoteAccess()) > 0 {
		s.Warnf(Ptr("vpn", "remoteAccess"), ruleUnsupportedField, "vpn.remoteAccess is not implemented by this agent build (F-ra-vpn)")
	}
	ifs := v.GetWireguard().GetInterfaces()
	for _, name := range sortedKeys(ifs) {
		wireguardInterface(s, name, ifs[name], in, vrfID, env)
	}
}

// secretRef maps a D-051 reference to its DF-5 reference. Without material in the agent
// (PENDING-secret-channel) it reports a WARNING at pointer and returns the wireguard.UnavailableRef
// marker: validation (DryRun) passes, and the apply fails loudly at that object (Create refuses the
// marker) — a peer is never created without its preshared key.
func secretRef(s Sink, env WireguardEnv, ref, pointer string) string {
	var err error
	if env.SecretRef != nil {
		var r string
		if r, err = env.SecretRef(ref); err == nil {
			return r
		}
	} else {
		err = fmt.Errorf("%s: %w", vpn.Redact(ref), wireguard.ErrSecretUnavailable)
	}
	s.Warnf(pointer, RuleWireguardSecret, "%v; the commit fails when it reaches this object", err)
	return wireguard.UnavailableRef(ref)
}

func vrfName(n string) string {
	if n == "" {
		return "default"
	}
	return n
}

func wireguardInterface(s Sink, name string, w *vrxv1.WireguardInterface, in map[string]bool, vrfID func(string) (uint32, bool), env WireguardEnv) {
	pt := Ptr("vpn", "wireguard", "interfaces", name)
	if w.Instance == nil {
		s.Errorf(pt+"/instance", ruleWireguardInstance, "WireGuard interface %q has no instance", name)
		return
	}
	wg := wireguard.ItfName(w.GetInstance())
	src, err := vpn.CanonicalAddress(w.GetListenAddress())
	if err != nil {
		s.Errorf(pt+"/listenAddress", RuleWireguardValue, "listen address: %v", err)
		return
	}
	port := uint32(51820)
	if w.ListenPort != nil {
		port = w.GetListenPort()
	}
	underlayName, overlayName := vrfName(w.GetUnderlayVrf()), vrfName(w.GetVrf())
	underlay, ok := vrfID(underlayName)
	if !ok {
		s.Errorf(pt+"/underlayVrf", "vpn.vrf-exists", "VRF %q does not exist", underlayName)
		return
	}
	overlay, ok := vrfID(overlayName)
	if !ok {
		s.Errorf(pt+"/vrf", "vpn.vrf-exists", "VRF %q does not exist", overlayName)
		return
	}
	priv := secretRef(s, env, w.GetPrivateKeyRef(), pt+"/privateKeyRef")
	itf := &vpnpb.WireguardInterface{Instance: w.GetInstance(), PrivateKey: priv, Port: port, SrcIp: src}
	s.Add(scheduler.Join(wireguard.InterfaceName, wg), itf, pt)
	s.Add(wireguard.MetaKey(wg), wireguard.MetaValue(wireguard.MetaSpec{
		ID: wg, Name: name, Description: w.GetDescription(), SecretRef: w.GetPrivateKeyRef(),
		UnderlayVrf: underlayName, RouteAllowedIps: w.GetRouteAllowedIps(),
	}), pt)

	enabled := w.Enabled == nil || w.GetEnabled()
	ref := string(vpn.InterfaceKey(wg))
	if in["interfaces"] {
		if enabled {
			s.Add(scheduler.Join(iface.AdminStateName, wg), &iface.AdminState{Interface: ref}, pt+"/enabled")
		}
		if w.Mtu != nil {
			s.Add(scheduler.Join(iface.MtuName, wg), &iface.Mtu{Interface: ref, Mtu: w.GetMtu()}, pt+"/mtu")
		}
		if overlay != 0 {
			s.Add(core.InterfaceTableKey(wg), &core.InterfaceTable{Interface: wg, TableId: overlay}, pt+"/vrf")
		}
		for i, a := range w.GetAddress() {
			c, err := core.CanonAddrPrefix(a)
			if err != nil {
				s.Errorf(pt+"/address/"+strconv.Itoa(i), RuleWireguardValue, "%v", err)
				continue
			}
			s.Add(core.InterfaceAddrKey(wg, c), &core.InterfaceAddress{Interface: wg, Prefix: c}, pt+"/address/"+strconv.Itoa(i))
		}
	} else if enabled || w.Mtu != nil || overlay != 0 || len(w.GetAddress()) > 0 {
		s.Warnf(pt, RuleWireguardDomain, "%s: enabled, mtu, vrf and address are applied only in a transaction that includes `interfaces`", pt)
	}

	for _, pname := range sortedKeys(w.GetPeers()) {
		wireguardPeer(s, pt, wg, pname, w.GetPeers()[pname], underlay, overlay, w.GetRouteAllowedIps() && in["routing"], env)
	}
	if w.GetRouteAllowedIps() && !in["routing"] && len(w.GetPeers()) > 0 {
		s.Warnf(pt+"/routeAllowedIps", RuleWireguardDomain, "%s/routeAllowedIps is applied only in a transaction that includes `routing`", pt)
	}
}

func wireguardPeer(s Sink, ipt, wg, name string, p *vrxv1.WireguardPeer, underlay, overlay uint32, routes bool, env WireguardEnv) {
	pt := ipt + "/peers/" + Ptr(name)[1:]
	pub := p.GetPublicKey()
	if raw, err := base64.StdEncoding.DecodeString(pub); err != nil || len(raw) != vpn.X25519KeyLen {
		s.Errorf(pt+"/publicKey", RuleWireguardValue, "public key must be the base64 of %d bytes", vpn.X25519KeyLen)
		return
	}
	peer := &vpnpb.WireguardPeer{Interface: wg, PublicKey: pub, PersistentKeepalive: p.GetPersistentKeepaliveSec(), TableId: underlay}
	if ep := p.GetEndpoint(); ep != nil && ep.GetAddress() != "" {
		a, err := vpn.CanonicalAddress(ep.GetAddress())
		if err != nil {
			s.Errorf(pt+"/endpoint/address", RuleWireguardValue, "endpoint %q: this agent build does not resolve hostnames; use an IP address", ep.GetAddress())
			return
		}
		peer.Endpoint, peer.Port = a, ep.GetPort()
	}
	for i, a := range p.GetAllowedIps() {
		c, err := vpn.CanonicalPrefix(a)
		if err != nil {
			s.Errorf(pt+"/allowedIps/"+strconv.Itoa(i), RuleWireguardValue, "%v", err)
			return
		}
		peer.AllowedIps = append(peer.AllowedIps, c)
	}
	slices.Sort(peer.AllowedIps)
	peer.AllowedIps = slices.Compact(peer.AllowedIps)
	if p.PresharedKeyRef != nil {
		peer.PresharedKey = secretRef(s, env, p.GetPresharedKeyRef(), pt+"/presharedKeyRef")
	}
	s.Add(scheduler.Join(wireguard.PeerName, wg, pub), peer, pt)
	s.Add(wireguard.MetaKey(wg+"/"+pub), wireguard.MetaValue(wireguard.MetaSpec{
		ID: wg + "/" + pub, Name: name, Description: p.GetDescription(), SecretRef: p.GetPresharedKeyRef(),
	}), pt)
	if !routes {
		return
	}
	for i, a := range p.GetAllowedIps() {
		c, _ := vpn.CanonicalPrefix(a) // checked above
		nh, err := WireguardRouteNextHop(c)
		if err != nil {
			s.Errorf(pt+"/allowedIps/"+strconv.Itoa(i), RuleWireguardValue, "%v", err)
			continue
		}
		s.Add(core.RouteKey(overlay, c), &core.Route{TableId: overlay, Prefix: c, Paths: []*core.RoutePath{{Address: nh, Interface: wg, Weight: 1}}},
			pt+"/allowedIps/"+strconv.Itoa(i))
	}
}

// WireguardRouteNextHop is the next hop of the automatic route of an allowed IP: the prefix's first
// address (covered by the allowed IP, so VPP binds the adjacency to that peer), or the second one when
// the first is the unspecified address (a zero next hop would make the path attached — a drop on NBMA).
func WireguardRouteNextHop(prefix string) (string, error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return "", err
	}
	a := p.Masked().Addr()
	if a.IsUnspecified() {
		a = a.Next()
	}
	return a.String(), nil
}

// ---- assembler ---------------------------------------------------------------------------------

// AssembleWireguard adds vpn.wireguard to ds from the retrieved objects (when in["vpn"]) and removes
// what P08's assemblers derived from WireGuard objects: interfaces.wg<N> (unless the stored document
// names it) and the automatic allowed-IP routes in routing.static. The configuration's names,
// descriptions and secret references come from wireguard.meta (D-073b: VPP cannot hold them) for
// objects that exist in VPP; an object without metadata is named by its VPP identity (wg<N>, the
// public key), which the drift view shows. tableName names a FIB table.
func AssembleWireguard(ds *vrxv1.DesiredState, kvs []scheduler.KV, in map[string]bool, tableName func(uint32) string, stored map[string]*vrxv1.Interface) {
	itfs := map[string]*vpnpb.WireguardInterface{}
	peers := map[string][]*vpnpb.WireguardPeer{}
	metas := map[string]wireguard.MetaSpec{}
	admin := map[string]bool{}
	mtu := map[string]uint32{}
	table := map[string]uint32{}
	addrs := map[string][]string{}
	routes := map[string]*core.Route{} // key "<table>|<prefix>"
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *vpnpb.WireguardInterface:
			itfs[wireguard.ItfName(v.GetInstance())] = v
		case *vpnpb.WireguardPeer:
			peers[v.GetInterface()] = append(peers[v.GetInterface()], v)
		case *iface.AdminState:
			admin[iface.RefID(v.GetInterface())] = true
		case *iface.Mtu:
			mtu[iface.RefID(v.GetInterface())] = v.GetMtu()
		case *core.InterfaceTable:
			table[v.GetInterface()] = v.GetTableId()
		case *core.InterfaceAddress:
			addrs[v.GetInterface()] = append(addrs[v.GetInterface()], v.GetPrefix())
		case *core.Route:
			routes[strconv.FormatUint(uint64(v.GetTableId()), 10)+"|"+v.GetPrefix()] = v
		}
		if kv.Key.Descriptor() == wireguard.MetaName {
			if m, err := wireguard.DecodeMeta(kv.Value); err == nil {
				metas[m.ID] = m
			}
		}
	}
	if len(itfs) == 0 && !in["vpn"] {
		return
	}
	autoRoute := map[string]bool{} // routes that are wg allowed-IP routes (removed from routing.static)
	out := map[string]*vrxv1.WireguardInterface{}
	for _, wg := range sortedKeys(itfs) {
		w := itfs[wg]
		m, hasMeta := metas[wg]
		name := wg
		if hasMeta {
			name = m.Name
		}
		ps := peers[wg]
		slices.SortFunc(ps, func(a, b *vpnpb.WireguardPeer) int {
			switch {
			case a.GetPublicKey() < b.GetPublicKey():
				return -1
			case a.GetPublicKey() > b.GetPublicKey():
				return 1
			}
			return 0
		})
		wi := &vrxv1.WireguardInterface{Instance: proto.Uint32(w.GetInstance()), ListenPort: proto.Uint32(w.GetPort())}
		if w.GetSrcIp() != "" {
			wi.ListenAddress = proto.String(w.GetSrcIp())
		}
		if hasMeta {
			if m.Description != "" {
				wi.Description = proto.String(m.Description)
			}
			if m.SecretRef != "" {
				wi.PrivateKeyRef = proto.String(m.SecretRef)
			}
		}
		switch {
		case len(ps) > 0:
			wi.UnderlayVrf = proto.String(tableName(ps[0].GetTableId()))
		case hasMeta && m.UnderlayVrf != "":
			wi.UnderlayVrf = proto.String(m.UnderlayVrf)
		default:
			wi.UnderlayVrf = proto.String(tableName(0))
		}
		if in["interfaces"] {
			wi.Enabled = proto.Bool(admin[wg])
			if v, ok := mtu[wg]; ok {
				wi.Mtu = proto.Uint32(v)
			}
			wi.Vrf = proto.String(tableName(table[wg]))
			a := append([]string(nil), addrs[wg]...)
			sortAddrs(a)
			wi.Address = a
		}
		// routeAllowedIps: configured (metadata) and every automatic route present
		allRoutes := hasMeta && m.RouteAllowedIps
		for _, p := range ps {
			pm, pHas := metas[wg+"/"+p.GetPublicKey()]
			pname := p.GetPublicKey()
			if pHas {
				pname = pm.Name
			}
			wp := &vrxv1.WireguardPeer{PublicKey: proto.String(p.GetPublicKey()), AllowedIps: append([]string(nil), p.GetAllowedIps()...),
				PersistentKeepaliveSec: proto.Uint32(p.GetPersistentKeepalive())}
			if p.GetEndpoint() != "" {
				wp.Endpoint = &vrxv1.WireguardPeer_Endpoint{Address: proto.String(p.GetEndpoint()), Port: proto.Uint32(p.GetPort())}
			}
			if pHas && pm.Description != "" {
				wp.Description = proto.String(pm.Description)
			}
			if p.GetPresharedKey() != "" {
				if pHas && pm.SecretRef != "" {
					wp.PresharedKeyRef = proto.String(pm.SecretRef)
				} else {
					wp.PresharedKeyRef = proto.String(vpn.Redact(p.GetPresharedKey())) // no reference known: shown as drift
				}
			}
			if wi.Peers == nil {
				wi.Peers = map[string]*vrxv1.WireguardPeer{}
			}
			wi.Peers[pname] = wp
			if hasMeta && m.RouteAllowedIps {
				for _, pfx := range p.GetAllowedIps() {
					k := strconv.FormatUint(uint64(table[wg]), 10) + "|" + pfx
					if r, ok := routes[k]; ok && isWireguardRoute(r, wg) {
						autoRoute[tableName(table[wg])+"|"+pfx] = true
					} else {
						allRoutes = false
					}
				}
			}
		}
		wi.RouteAllowedIps = proto.Bool(allRoutes)
		out[name] = wi
	}
	if in["vpn"] && len(out) > 0 { // nothing retrieved: the domain stays unset (empty and absent are the same, proto.md §5)
		if ds.Vpn == nil {
			ds.Vpn = &vrxv1.VpnConfig{}
		}
		ds.Vpn.Wireguard = &vrxv1.WireguardConfig{Interfaces: out}
	}
	// what P08's assemblers derived from WireGuard objects belongs to vpn.wireguard
	for wg := range itfs {
		if _, named := stored[wg]; !named {
			delete(ds.Interfaces, wg)
		}
	}
	if st := ds.GetRouting(); st != nil && len(autoRoute) > 0 {
		kept := st.Static[:0]
		for _, r := range st.GetStatic() {
			if autoRoute[r.GetVrf()+"|"+r.GetPrefix()] && len(r.GetNextHops()) == 1 {
				continue
			}
			kept = append(kept, r)
		}
		st.Static = kept
	}
}

// isWireguardRoute reports whether r is an automatic allowed-IP route through wg.
func isWireguardRoute(r *core.Route, wg string) bool {
	if len(r.GetPaths()) != 1 || r.GetPaths()[0].GetInterface() != wg {
		return false
	}
	nh, err := WireguardRouteNextHop(r.GetPrefix())
	return err == nil && r.GetPaths()[0].GetAddress() == nh
}
