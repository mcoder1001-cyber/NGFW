package coretest

import (
	"fmt"
	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/bfd"
	"ngfw/agent/binapi/interface_types"
)

func init() { RegisterExtension("bfd-redistribution", (*VPP).installBfdRedistribution) }
func (v *VPP) installBfdRedistribution() {
	v.On("bfd_udp_enable_multihop", func(api.Message) ([]api.Message, error) { return []api.Message{&bfd.BfdUDPEnableMultihopReply{}}, nil })
	keys := map[uint32]*bfd.BfdAuthKeysDetails{}
	sess := map[string]*bfd.BfdUDPSessionDetails{}
	skey := func(idx interface_types.InterfaceIndex, l, p string) string {
		return l + ">" + p + "@" + fmt.Sprint(idx)
	}
	v.On("bfd_auth_set_key", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*bfd.BfdAuthSetKey)
		keys[r.ConfKeyID] = &bfd.BfdAuthKeysDetails{ConfKeyID: r.ConfKeyID, AuthType: r.AuthType}
		return []api.Message{&bfd.BfdAuthSetKeyReply{}}, nil
	})
	v.On("bfd_auth_del_key", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		delete(keys, m.(*bfd.BfdAuthDelKey).ConfKeyID)
		return []api.Message{&bfd.BfdAuthDelKeyReply{}}, nil
	})
	v.On("bfd_auth_keys_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, k := range keys {
			out = append(out, k)
		}
		return out, nil
	})
	v.On("bfd_udp_add", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*bfd.BfdUDPAdd)
		sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())] = &bfd.BfdUDPSessionDetails{SwIfIndex: r.SwIfIndex, LocalAddr: r.LocalAddr, PeerAddr: r.PeerAddr,
			State: bfd.BFD_STATE_API_DOWN, IsAuthenticated: r.IsAuthenticated, BfdKeyID: r.BfdKeyID, ConfKeyID: r.ConfKeyID,
			RequiredMinRx: r.RequiredMinRx, DesiredMinTx: r.DesiredMinTx, DetectMult: r.DetectMult}
		return []api.Message{&bfd.BfdUDPAddReply{}}, nil
	})
	v.On("bfd_udp_mod", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*bfd.BfdUDPMod)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.DesiredMinTx, s.RequiredMinRx, s.DetectMult = r.DesiredMinTx, r.RequiredMinRx, r.DetectMult
		return []api.Message{&bfd.BfdUDPModReply{}}, nil
	})
	v.On("bfd_udp_session_set_flags", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*bfd.BfdUDPSessionSetFlags)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.State = bfd.BFD_STATE_API_DOWN
		if r.Flags == 0 {
			s.State = bfd.BFD_STATE_API_ADMIN_DOWN
		}
		return []api.Message{&bfd.BfdUDPSessionSetFlagsReply{}}, nil
	})
	v.On("bfd_udp_auth_activate", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*bfd.BfdUDPAuthActivate)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.IsAuthenticated, s.ConfKeyID, s.BfdKeyID = true, r.ConfKeyID, r.BfdKeyID
		return []api.Message{&bfd.BfdUDPAuthActivateReply{}}, nil
	})
	v.On("bfd_udp_auth_deactivate", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*bfd.BfdUDPAuthDeactivate)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.IsAuthenticated, s.ConfKeyID, s.BfdKeyID = false, 0, 0
		return []api.Message{&bfd.BfdUDPAuthDeactivateReply{}}, nil
	})
	v.On("bfd_udp_del", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*bfd.BfdUDPDel)
		delete(sess, skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String()))
		return []api.Message{&bfd.BfdUDPDelReply{}}, nil
	})
	v.On("bfd_udp_session_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for _, s := range sess {
			out = append(out, s)
		}
		return out, nil
	})
	v.On("want_bfd_events", func(api.Message) ([]api.Message, error) { return []api.Message{&bfd.WantBfdEventsReply{}}, nil })
	v.On("bfd_udp_get_echo_source", func(api.Message) ([]api.Message, error) { return []api.Message{&bfd.BfdUDPGetEchoSourceReply{}}, nil })
}
