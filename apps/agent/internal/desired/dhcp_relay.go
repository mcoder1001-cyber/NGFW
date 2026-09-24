package desired

// F-kea-dhcp-relay: services.dhcp.relays → VPP DHCP relay (dhcp plugin proxy, DF-8).
//
// VPP relays per rx VRF (D-077 Q7), not per interface: one source address per rx VRF and family and a list of
// servers, each reached in its server VRF. A relay of the document therefore becomes
//
//	dhcp.proxy/<rx table>/<server table>/<server>   {rx_vrf, server_vrf, server, src}  one per server (enabled only)
//	dhcp.relay/<name>                               the relay record: the document relay + the VPP tuple it needs
//
// `interfaces` (the client interfaces) documents which interfaces of the rx VRF are served; VPP relays DHCP on every
// interface of the rx VRF (dhcp_proxy_config has no interface), which the user guide states.

import (
	"encoding/base64"
	"slices"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dhcp"
	"ngfw/agent/internal/scheduler"
)

// DHCP is the projection of `services` (F-kea-dhcp-relay): the Kea servers, the relays and the unsupported-field
// notes of the other sub-keys. vrfID maps a VRF name to its table id (false: unknown VRF).
func DHCP(s Sink, ds *vrxv1.DesiredState, vrfID func(string) (uint32, bool)) {
	ServicesUnsupported(s, ds.GetServices())
	Kea(s, ds)
	DHCPRelays(s, ds.GetServices().GetDhcp().GetRelays(), vrfID)
}

// AssembleDHCP adds services.dhcp (servers and relays) of the retrieved objects to ds. `services.dhcp` is always
// present (empty maps) once the domain is assembled.
func AssembleDHCP(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	dhcpOf(ds)
	AssembleKea(ds, kvs)
	AssembleDHCPRelays(ds, kvs)
}

// DHCPRelays projects the relays.
func DHCPRelays(s Sink, relays map[string]*vrxv1.DhcpRelay, vrfID func(string) (uint32, bool)) {
	type srcOwner struct{ relay, src string }
	srcs := map[string]srcOwner{} // "<rx>|<family>" → first relay and its src
	for _, name := range sortedKeys(relays) {
		r := relays[name]
		pt := Ptr("services", "dhcp", "relays", name)
		enabled := r.Enabled == nil || r.GetEnabled()
		vrf := r.GetVrf()
		if vrf == "" {
			vrf = "default"
		}
		serverVrf := r.GetServerVrf()
		if serverVrf == "" {
			serverVrf = vrf
		}
		rx, ok := vrfID(vrf)
		if !ok {
			s.Errorf(pt+"/vrf", "services.vrf-exists", "VRF %q does not exist", vrf)
			continue
		}
		srvTable, ok := vrfID(serverVrf)
		if !ok {
			s.Errorf(pt+"/serverVrf", "services.vrf-exists", "VRF %q does not exist", serverVrf)
			continue
		}
		doc, err := proto.MarshalOptions{Deterministic: true}.Marshal(r)
		if err != nil {
			s.Errorf(pt, "services.dhcp-relay", "encode relay: %v", err)
			continue
		}
		rec := dhcp.Relay{Name: name, Doc: base64.StdEncoding.EncodeToString(doc), Enabled: enabled, RxVRF: rx, ServerVRF: srvTable}
		src, err := dfkit.ParseAddr(r.GetSourceAddress())
		if err != nil {
			s.Errorf(pt+"/sourceAddress", "services.dhcp-relay-address", "%v", err)
			continue
		}
		rec.Src = src.String()
		want4 := r.GetFamily() != "ipv6"
		if src.Is4() != want4 {
			s.Errorf(pt+"/sourceAddress", "services.dhcp-relay-address", "%s does not match family %s", src, r.GetFamily())
			continue
		}
		bad := false
		for i, a := range r.GetServers() {
			srv, err := dfkit.ParseAddr(a)
			if err == nil && srv.Is4() != want4 {
				s.Errorf(pt+"/servers/"+strconv.Itoa(i), "services.dhcp-relay-address", "%s does not match family %s", srv, r.GetFamily())
				bad = true
				continue
			}
			if err != nil {
				s.Errorf(pt+"/servers/"+strconv.Itoa(i), "services.dhcp-relay-address", "%v", err)
				bad = true
				continue
			}
			rec.Servers = append(rec.Servers, srv.String())
		}
		if bad {
			continue
		}
		if enabled {
			key := strconv.FormatUint(uint64(rx), 10) + "|" + strconv.FormatBool(want4)
			if first, seen := srcs[key]; seen && first.src != rec.Src {
				s.Errorf(pt+"/sourceAddress", "services.kea-dhcp-relay-relay-source-per-vrf",
					"VPP keeps one relay source address per client VRF and family: relay %q already uses %s in VRF %q", first.relay, first.src, vrf)
				continue
			} else if !seen {
				srcs[key] = srcOwner{name, rec.Src}
			}
			for i, srv := range rec.Servers {
				p := dhcp.Proxy{RxVRF: rx, ServerVRF: srvTable, Server: srv, Src: rec.Src}
				s.Add(dhcp.ProxyKey(rx, srvTable, srv), p.Proto(), pt+"/servers/"+strconv.Itoa(i))
			}
		}
		s.Add(dhcp.RelayKey(name), rec.Proto(), pt)
	}
}

// AssembleDHCPRelays adds the relays of the retrieved dhcp.relay records to ds: the document relay the record carries,
// with the servers VPP actually has when they differ (drift stays visible).
func AssembleDHCPRelays(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key.Descriptor() != dhcp.NameRelay {
			continue
		}
		var rec dhcp.Relay
		if dfkit.Decode(kv.Value, &rec) != nil {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(rec.Doc)
		if err != nil {
			continue
		}
		r := &vrxv1.DhcpRelay{}
		if proto.Unmarshal(raw, r) != nil {
			continue
		}
		if rec.Enabled {
			var docServers []string
			for _, a := range r.GetServers() {
				if p, err := dfkit.ParseAddr(a); err == nil {
					docServers = append(docServers, p.String())
				}
			}
			sort.Strings(docServers)
			got := append([]string(nil), rec.Servers...)
			sort.Strings(got)
			if !slices.Equal(docServers, got) {
				r.Servers = got
			}
		}
		dhcpOf(ds).Relays[rec.Name] = r
	}
}
