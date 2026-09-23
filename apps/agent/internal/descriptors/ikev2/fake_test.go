package ikev2_test

// A stateful fake VPP for the ikev2 descriptors: profiles live in a map so a dump reflects earlier
// setters. It models the VPP behaviour the descriptors rely on: set-only udp_encap/natt, the
// ipsec-over-udp port that must be unset before it is set again, the PSK returned in clear by
// ikev2_profile_dump, derived keys in the SA dumps, and govpp cutting id.data (string[64]) at the
// first NUL byte.

import (
	"bytes"
	"sort"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/binapi/ikev2_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/fake"
)

const (
	rvUnspecified = -1   // VNET_API_ERROR_UNSPECIFIED
	rvInvalid     = -73  // VNET_API_ERROR_INVALID_VALUE
	rvExists      = -103 // VNET_API_ERROR_VALUE_EXIST (any non-zero works for the descriptor)
)

type fakeProfile struct {
	p       ikev2_types.Ikev2Profile
	locData []byte // full id payloads as VPP stores them
	remData []byte
}

type fakeVPP struct {
	*fake.Client
	ifaces   map[uint32]*interfaces.SwInterfaceDetails
	profiles map[string]*fakeProfile
	order    []string
	sleep    float64
	liveness [2]uint32
	localKey string
	sas      []ikev2_types.Ikev2SaV3
	children map[uint32][]ikev2_types.Ikev2ChildSaV2
	failAuth bool
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{
		Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		ifaces: map[uint32]*interfaces.SwInterfaceDetails{
			0: {SwIfIndex: 0, InterfaceName: "local0"},
			1: {SwIfIndex: 1, InterfaceName: "loop401", Tag: "w4:loop401"},
			2: {SwIfIndex: 2, InterfaceName: "ipsec4001", Tag: "w4:ipsec4001"},
		},
		profiles: map[string]*fakeProfile{},
		sleep:    2,
		liveness: [2]uint32{30, 3},
		children: map[uint32][]ikev2_types.Ikev2ChildSaV2{},
	}
	get := func(name string) (*fakeProfile, bool) { p, ok := v.profiles[name]; return p, ok }
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		idx := make([]int, 0, len(v.ifaces))
		for i := range v.ifaces {
			idx = append(idx, int(i))
		}
		sort.Ints(idx)
		out := make([]api.Message, 0, len(idx))
		for _, i := range idx {
			out = append(out, v.ifaces[uint32(i)])
		}
		return out, nil
	})
	v.On("ikev2_profile_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2ProfileAddDel)
		_, exists := v.profiles[r.Name]
		if r.IsAdd == exists {
			return []api.Message{&ikev2.Ikev2ProfileAddDelReply{Retval: rvUnspecified}}, nil
		}
		if r.IsAdd {
			v.profiles[r.Name] = &fakeProfile{p: ikev2_types.Ikev2Profile{
				Name: r.Name, IpsecOverUDPPort: 0xffff, TunItf: ^uint32(0),
				Responder: ikev2_types.Ikev2Responder{SwIfIndex: ^interface_types.InterfaceIndex(0)},
			}}
			v.order = append(v.order, r.Name)
		} else {
			delete(v.profiles, r.Name)
		}
		return []api.Message{&ikev2.Ikev2ProfileAddDelReply{}}, nil
	})
	v.On("ikev2_profile_set_auth", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2ProfileSetAuth)
		p, ok := get(r.Name)
		if !ok || v.failAuth || r.DataLen == 0 {
			return []api.Message{&ikev2.Ikev2ProfileSetAuthReply{Retval: rvUnspecified}}, nil
		}
		data := append([]byte(nil), r.Data...) // VPP keeps its own copy; the request is zeroed
		if r.AuthMethod == 1 {
			data = append(data, 0) // rsa-sig: vec_add1 (p->auth.data, 0)
		}
		p.p.Auth = ikev2_types.Ikev2Auth{Method: r.AuthMethod, DataLen: uint32(len(data)), Data: data}
		return []api.Message{&ikev2.Ikev2ProfileSetAuthReply{}}, nil
	})
	v.On("ikev2_profile_set_id", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2ProfileSetID)
		p, ok := get(r.Name)
		if !ok || (r.IDType != 1 && r.IDType != 2 && r.IDType != 3 && r.IDType != 5) {
			return []api.Message{&ikev2.Ikev2ProfileSetIDReply{Retval: rvUnspecified}}, nil
		}
		id := ikev2_types.Ikev2ID{Type: r.IDType, DataLen: uint8(len(r.Data))}
		if r.IsLocal {
			p.p.LocID, p.locData = id, append([]byte(nil), r.Data...)
		} else {
			p.p.RemID, p.remData = id, append([]byte(nil), r.Data...)
		}
		return []api.Message{&ikev2.Ikev2ProfileSetIDReply{}}, nil
	})
	v.On("ikev2_profile_set_ts", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2ProfileSetTs)
		p, ok := get(r.Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2ProfileSetTsReply{Retval: rvUnspecified}}, nil
		}
		if r.Ts.IsLocal {
			p.p.LocTs = r.Ts
		} else {
			p.p.RemTs = r.Ts
		}
		return []api.Message{&ikev2.Ikev2ProfileSetTsReply{}}, nil
	})
	v.On("ikev2_set_responder", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2SetResponder)
		p, ok := get(r.Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2SetResponderReply{Retval: rvUnspecified}}, nil
		}
		p.p.Responder = r.Responder
		return []api.Message{&ikev2.Ikev2SetResponderReply{}}, nil
	})
	v.On("ikev2_set_responder_hostname", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2SetResponderHostname)
		p, ok := get(r.Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2SetResponderHostnameReply{Retval: rvUnspecified}}, nil
		}
		p.p.Responder.SwIfIndex = r.SwIfIndex // the hostname itself is not dumped
		return []api.Message{&ikev2.Ikev2SetResponderHostnameReply{}}, nil
	})
	v.On("ikev2_set_ike_transforms", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2SetIkeTransforms)
		p, ok := get(r.Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2SetIkeTransformsReply{Retval: rvUnspecified}}, nil
		}
		p.p.IkeTs = r.Tr
		return []api.Message{&ikev2.Ikev2SetIkeTransformsReply{}}, nil
	})
	v.On("ikev2_set_esp_transforms", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2SetEspTransforms)
		p, ok := get(r.Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2SetEspTransformsReply{Retval: rvUnspecified}}, nil
		}
		p.p.EspTs = r.Tr
		return []api.Message{&ikev2.Ikev2SetEspTransformsReply{}}, nil
	})
	v.On("ikev2_set_sa_lifetime", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2SetSaLifetime)
		p, ok := get(r.Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2SetSaLifetimeReply{Retval: rvUnspecified}}, nil
		}
		p.p.Lifetime, p.p.LifetimeJitter, p.p.Handover, p.p.LifetimeMaxdata = r.Lifetime, r.LifetimeJitter, r.Handover, r.LifetimeMaxdata
		return []api.Message{&ikev2.Ikev2SetSaLifetimeReply{}}, nil
	})
	v.On("ikev2_profile_set_udp_encap", func(req api.Message) ([]api.Message, error) {
		p, ok := get(req.(*ikev2.Ikev2ProfileSetUDPEncap).Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2ProfileSetUDPEncapReply{Retval: rvUnspecified}}, nil
		}
		p.p.UDPEncap = true
		return []api.Message{&ikev2.Ikev2ProfileSetUDPEncapReply{}}, nil
	})
	v.On("ikev2_profile_set_ipsec_udp_port", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2ProfileSetIpsecUDPPort)
		p, ok := get(r.Name)
		switch {
		case !ok:
			return []api.Message{&ikev2.Ikev2ProfileSetIpsecUDPPortReply{Retval: rvInvalid}}, nil
		case r.IsSet != 0 && p.p.IpsecOverUDPPort != 0xffff:
			return []api.Message{&ikev2.Ikev2ProfileSetIpsecUDPPortReply{Retval: rvExists}}, nil
		case r.IsSet == 0 && p.p.IpsecOverUDPPort == 0xffff:
			return []api.Message{&ikev2.Ikev2ProfileSetIpsecUDPPortReply{Retval: rvInvalid}}, nil
		case r.IsSet != 0:
			p.p.IpsecOverUDPPort = r.Port
		default:
			p.p.IpsecOverUDPPort = 0xffff
		}
		return []api.Message{&ikev2.Ikev2ProfileSetIpsecUDPPortReply{}}, nil
	})
	v.On("ikev2_set_tunnel_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2SetTunnelInterface)
		p, ok := get(r.Name)
		if _, valid := v.ifaces[uint32(r.SwIfIndex)]; !ok || !valid {
			return []api.Message{&ikev2.Ikev2SetTunnelInterfaceReply{Retval: rvUnspecified}}, nil
		}
		p.p.TunItf = uint32(r.SwIfIndex)
		return []api.Message{&ikev2.Ikev2SetTunnelInterfaceReply{}}, nil
	})
	v.On("ikev2_profile_disable_natt", func(req api.Message) ([]api.Message, error) {
		p, ok := get(req.(*ikev2.Ikev2ProfileDisableNatt).Name)
		if !ok {
			return []api.Message{&ikev2.Ikev2ProfileDisableNattReply{Retval: rvUnspecified}}, nil
		}
		p.p.NattDisabled = true
		return []api.Message{&ikev2.Ikev2ProfileDisableNattReply{}}, nil
	})
	v.On("ikev2_profile_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.profiles))
		for _, name := range v.order {
			p, ok := v.profiles[name]
			if !ok {
				continue
			}
			d := p.p
			d.Auth.Data = append([]byte(nil), p.p.Auth.Data...) // the dump carries the PSK in clear
			d.LocID.Data, d.RemID.Data = govppString(p.locData), govppString(p.remData)
			out = append(out, &ikev2.Ikev2ProfileDetails{Profile: d})
		}
		return out, nil
	})
	v.On("ikev2_get_sleep_interval", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ikev2.Ikev2GetSleepIntervalReply{SleepInterval: v.sleep}}, nil
	})
	v.On("ikev2_plugin_set_sleep_interval", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2PluginSetSleepInterval)
		if r.Timeout == 0 {
			return []api.Message{&ikev2.Ikev2PluginSetSleepIntervalReply{Retval: rvUnspecified}}, nil
		}
		v.sleep = r.Timeout
		return []api.Message{&ikev2.Ikev2PluginSetSleepIntervalReply{}}, nil
	})
	v.On("ikev2_profile_set_liveness", func(req api.Message) ([]api.Message, error) {
		r := req.(*ikev2.Ikev2ProfileSetLiveness)
		v.liveness = [2]uint32{r.Period, r.MaxRetries}
		return []api.Message{&ikev2.Ikev2ProfileSetLivenessReply{}}, nil
	})
	v.On("ikev2_set_local_key", func(req api.Message) ([]api.Message, error) {
		v.localKey = req.(*ikev2.Ikev2SetLocalKey).KeyFile
		return []api.Message{&ikev2.Ikev2SetLocalKeyReply{}}, nil
	})
	v.On("ikev2_sa_v3_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.sas))
		for _, sa := range v.sas {
			out = append(out, &ikev2.Ikev2SaV3Details{Sa: sa})
		}
		return out, nil
	})
	v.On("ikev2_child_sa_v2_dump", func(req api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, ch := range v.children[req.(*ikev2.Ikev2ChildSaV2Dump).SaIndex] {
			out = append(out, &ikev2.Ikev2ChildSaV2Details{ChildSa: ch})
		}
		return out, nil
	})
	for _, n := range []string{"ikev2_initiate_sa_init", "ikev2_initiate_del_ike_sa", "ikev2_initiate_del_child_sa", "ikev2_initiate_rekey_child_sa"} {
		name := n
		v.On(name, func(api.Message) ([]api.Message, error) {
			switch name {
			case "ikev2_initiate_sa_init":
				return []api.Message{&ikev2.Ikev2InitiateSaInitReply{}}, nil
			case "ikev2_initiate_del_ike_sa":
				return []api.Message{&ikev2.Ikev2InitiateDelIkeSaReply{}}, nil
			case "ikev2_initiate_del_child_sa":
				return []api.Message{&ikev2.Ikev2InitiateDelChildSaReply{}}, nil
			default:
				return []api.Message{&ikev2.Ikev2InitiateRekeyChildSaReply{Retval: rvInvalid}}, nil
			}
		})
	}
	return v
}

// govppString is what govpp's DecodeString(64) makes of a fixed string field: cut at the first NUL.
func govppString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
