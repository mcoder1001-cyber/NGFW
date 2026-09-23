package bfd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/bfd"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

func fakeBFD() (*df7test.Fake, map[uint32]*bfd.BfdAuthKeysDetails, map[string]*bfd.BfdUDPSessionDetails) {
	f := df7test.NewFake()
	keys := map[uint32]*bfd.BfdAuthKeysDetails{}
	sess := map[string]*bfd.BfdUDPSessionDetails{}
	skey := func(idx interface_types.InterfaceIndex, l, p string) string {
		return l + ">" + p + "@" + fmt.Sprint(idx)
	}
	f.On("bfd_auth_set_key", func(m api.Message) ([]api.Message, error) {
		r := m.(*bfd.BfdAuthSetKey)
		keys[r.ConfKeyID] = &bfd.BfdAuthKeysDetails{ConfKeyID: r.ConfKeyID, AuthType: r.AuthType}
		return []api.Message{&bfd.BfdAuthSetKeyReply{}}, nil
	})
	f.On("bfd_auth_del_key", func(m api.Message) ([]api.Message, error) {
		delete(keys, m.(*bfd.BfdAuthDelKey).ConfKeyID)
		return []api.Message{&bfd.BfdAuthDelKeyReply{}}, nil
	})
	f.On("bfd_auth_keys_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, k := range keys {
			out = append(out, k)
		}
		return out, nil
	})
	f.On("bfd_udp_add", func(m api.Message) ([]api.Message, error) {
		r := m.(*bfd.BfdUDPAdd)
		sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())] = &bfd.BfdUDPSessionDetails{SwIfIndex: r.SwIfIndex, LocalAddr: r.LocalAddr, PeerAddr: r.PeerAddr,
			State: bfd.BFD_STATE_API_DOWN, IsAuthenticated: r.IsAuthenticated, BfdKeyID: r.BfdKeyID, ConfKeyID: r.ConfKeyID,
			RequiredMinRx: r.RequiredMinRx, DesiredMinTx: r.DesiredMinTx, DetectMult: r.DetectMult}
		return []api.Message{&bfd.BfdUDPAddReply{}}, nil
	})
	f.On("bfd_udp_mod", func(m api.Message) ([]api.Message, error) {
		r := m.(*bfd.BfdUDPMod)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.DesiredMinTx, s.RequiredMinRx, s.DetectMult = r.DesiredMinTx, r.RequiredMinRx, r.DetectMult
		return []api.Message{&bfd.BfdUDPModReply{}}, nil
	})
	f.On("bfd_udp_session_set_flags", func(m api.Message) ([]api.Message, error) {
		r := m.(*bfd.BfdUDPSessionSetFlags)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.State = bfd.BFD_STATE_API_DOWN
		if r.Flags == 0 {
			s.State = bfd.BFD_STATE_API_ADMIN_DOWN
		}
		return []api.Message{&bfd.BfdUDPSessionSetFlagsReply{}}, nil
	})
	f.On("bfd_udp_auth_activate", func(m api.Message) ([]api.Message, error) {
		r := m.(*bfd.BfdUDPAuthActivate)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.IsAuthenticated, s.ConfKeyID, s.BfdKeyID = true, r.ConfKeyID, r.BfdKeyID
		return []api.Message{&bfd.BfdUDPAuthActivateReply{}}, nil
	})
	f.On("bfd_udp_auth_deactivate", func(m api.Message) ([]api.Message, error) {
		r := m.(*bfd.BfdUDPAuthDeactivate)
		s := sess[skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String())]
		s.IsAuthenticated, s.ConfKeyID, s.BfdKeyID = false, 0, 0
		return []api.Message{&bfd.BfdUDPAuthDeactivateReply{}}, nil
	})
	f.On("bfd_udp_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*bfd.BfdUDPDel)
		delete(sess, skey(r.SwIfIndex, r.LocalAddr.String(), r.PeerAddr.String()))
		return []api.Message{&bfd.BfdUDPDelReply{}}, nil
	})
	f.On("bfd_udp_session_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, s := range sess {
			out = append(out, s)
		}
		return out, nil
	})
	return f, keys, sess
}

func secret(_ context.Context, id uint32) ([]byte, error) {
	if id == 13 {
		return nil, errors.New("vault sealed")
	}
	return []byte("VRX_TEST_PSK_7"), nil
}

func TestAuthKey(t *testing.T) {
	f, keys, _ := fakeBFD()
	ctx := t.Context()
	d := NewAuthKey(f, df7test.Owner, secret, df7.WithIDRange(1, 99))
	v := df7test.Desired(d, df7.Encode(AuthKey{ID: 7, Type: AuthKeyedSHA1}))
	if v.Key != "bfd.auth-key/7" || d.Dependencies(v.Value) != nil {
		t.Fatal(v.Key)
	}
	if _, err := d.Create(ctx, v.Value); err != nil {
		t.Fatal(err)
	}
	r := df7test.Last[*bfd.BfdAuthSetKey](t, f, "bfd_auth_set_key")
	if r.ConfKeyID != 7 || r.AuthType != 4 || r.KeyLen != 14 || len(r.Key) != 20 {
		t.Fatalf("%+v", r)
	}
	for _, b := range r.Key {
		if b != 0 {
			t.Fatal("the request buffer must be wiped after sending")
		}
	}
	keys[500] = &bfd.BfdAuthKeysDetails{ConfKeyID: 500, AuthType: 4} // outside our range
	df7test.AssertEmptyPlan(t, d, v)
	if _, err := d.Update(ctx, v.Value, v.Value, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, v.Value, nil); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d)
	if _, err := d.Create(ctx, df7.Encode(AuthKey{ID: 13, Type: AuthKeyedSHA1})); !errors.Is(err, ErrNoSecret) || strings.Contains(err.Error(), "PSK") {
		t.Fatalf("resolver error: %v", err)
	}
	if _, err := NewAuthKey(f, df7test.Owner, nil).Create(ctx, v.Value); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("no resolver: %v", err)
	}
	if _, err := d.Create(ctx, df7.Encode(AuthKey{ID: 100, Type: AuthKeyedSHA1})); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("id outside range: %v", err)
	}
	if _, err := d.Create(ctx, df7.Encode(AuthKey{ID: 8, Type: "md5"})); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("md5: %v", err)
	}
	long := NewAuthKey(f, df7test.Owner, func(context.Context, uint32) ([]byte, error) { return make([]byte, 21), nil })
	if _, err := long.Create(ctx, v.Value); !errors.Is(err, df7.ErrSpec) {
		t.Fatalf("21-byte secret: %v", err)
	}
}

func TestSession(t *testing.T) {
	f, _, sess := fakeBFD()
	ctx := t.Context()
	d := NewSession(f, df7test.Owner)
	s := Session{Interface: "loop0", Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 100000, RequiredMinRx: 100000, DetectMult: 3,
		Auth: &SessionAuth{ConfKeyID: 7, BFDKeyID: 1}}
	v := df7test.Desired(d, df7.Encode(s))
	if v.Key != "bfd.udp-session/loop0/10.0.0.1/10.0.0.2" {
		t.Fatal(v.Key)
	}
	deps := d.Dependencies(v.Value)
	if len(deps) != 2 || deps[0].Key != "interface/loop0" || deps[1].Key != "bfd.auth-key/7" {
		t.Fatalf("deps %v", deps)
	}
	meta, err := d.Create(ctx, v.Value)
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*bfd.BfdUDPAdd](t, f, "bfd_udp_add"); !r.IsAuthenticated || r.ConfKeyID != 7 || r.BfdKeyID != 1 || r.SwIfIndex != 1 {
		t.Fatalf("%+v", r)
	}
	if len(f.CallsNamed("bfd_udp_session_set_flags")) != 0 {
		t.Fatal("admin-up sessions need no set_flags")
	}
	// another owner's session and a multihop session are never reported
	sess["x"] = &bfd.BfdUDPSessionDetails{SwIfIndex: 3, LocalAddr: mustAddr("10.9.0.1"), PeerAddr: mustAddr("10.9.0.2")}
	sess["mh"] = &bfd.BfdUDPSessionDetails{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex), LocalAddr: mustAddr("10.9.0.1"), PeerAddr: mustAddr("10.9.0.3")}
	df7test.AssertEmptyPlan(t, d, v)

	// timers + auth off + admin down, in place
	s2 := s
	s2.DesiredMinTx, s2.Auth, s2.AdminDown = 200000, nil, true
	if _, err := d.Update(ctx, v.Value, df7.Encode(s2), meta); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"bfd_udp_mod", "bfd_udp_auth_deactivate", "bfd_udp_session_set_flags"} {
		if len(f.CallsNamed(n)) != 1 {
			t.Fatalf("%s not sent once", n)
		}
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, df7.Encode(s2)))
	s3 := s2
	s3.Auth, s3.AdminDown = &SessionAuth{ConfKeyID: 8, BFDKeyID: 2}, false
	if _, err := d.Update(ctx, df7.Encode(s2), df7.Encode(s3), meta); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*bfd.BfdUDPAuthActivate](t, f, "bfd_udp_auth_activate"); r.ConfKeyID != 8 || r.IsDelayed {
		t.Fatalf("%+v", r)
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, df7.Encode(s3)))
	s4 := s3
	s4.Peer = "10.0.0.9"
	if _, err := d.Update(ctx, df7.Encode(s3), df7.Encode(s4), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, df7.Encode(s3), meta); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d)

	// admin-down create sends set_flags(0)
	s5 := s
	s5.Auth, s5.AdminDown = nil, true
	if _, err := d.Create(ctx, df7.Encode(s5)); err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*bfd.BfdUDPSessionSetFlags](t, f, "bfd_udp_session_set_flags"); r.Flags != 0 {
		t.Fatalf("%+v", r)
	}
	df7test.AssertEmptyPlan(t, d, df7test.Desired(d, df7.Encode(s5)))
	for i, bad := range []Session{
		{Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 1, RequiredMinRx: 1, DetectMult: 1},
		{Interface: "loop0", Local: "10.0.0.1", Peer: "::2", DesiredMinTx: 1, RequiredMinRx: 1, DetectMult: 1},
		{Interface: "loop0", Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 1, RequiredMinRx: 1},
		{Interface: "loop0", Local: "2001:DB8::1", Peer: "2001:db8::2", DesiredMinTx: 1, RequiredMinRx: 1, DetectMult: 1},
	} {
		if err := bad.Validate(); !errors.Is(err, df7.ErrSpec) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if _, err := d.Create(ctx, df7.Encode(Session{Interface: "loop9", Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 1, RequiredMinRx: 1, DetectMult: 1})); !errors.Is(err, df7.ErrForeignInterface) {
		t.Fatal(err)
	}
	f.Reply("bfd_udp_add", &bfd.BfdUDPAddReply{Retval: int32(api.BFD_EEXIST)})
	if _, err := d.Create(ctx, df7.Encode(s5)); !df7.IsVPPError(err, api.BFD_EEXIST) {
		t.Fatalf("vpp error: %v", err)
	}
}

func TestEchoSource(t *testing.T) {
	f := df7test.NewFake()
	ctx := t.Context()
	set := uint32(0)
	f.On("bfd_udp_set_echo_source", func(m api.Message) ([]api.Message, error) {
		set = uint32(m.(*bfd.BfdUDPSetEchoSource).SwIfIndex)
		return []api.Message{&bfd.BfdUDPSetEchoSourceReply{}}, nil
	})
	f.On("bfd_udp_del_echo_source", func(api.Message) ([]api.Message, error) {
		set = 0
		return []api.Message{&bfd.BfdUDPDelEchoSourceReply{}}, nil
	})
	f.On("bfd_udp_get_echo_source", func(api.Message) ([]api.Message, error) {
		return []api.Message{&bfd.BfdUDPGetEchoSourceReply{IsSet: set != 0, SwIfIndex: interface_types.InterfaceIndex(set)}}, nil
	})
	d := NewEchoSource(f, df7test.Owner)
	v := df7test.Desired(d, df7.Encode(EchoSource{Interface: "loop1"}))
	if v.Key != "bfd.echo-source/global" || d.Dependencies(v.Value)[0].Key != "interface/loop1" {
		t.Fatal(v.Key)
	}
	df7test.AssertEmptyPlan(t, d)
	if _, err := d.Create(ctx, v.Value); err != nil || set != 2 {
		t.Fatal(err, set)
	}
	df7test.AssertEmptyPlan(t, d, v)
	set = 3 // another owner's interface: not ours
	df7test.AssertEmptyPlan(t, d)
	if _, err := d.Update(ctx, v.Value, df7.Encode(EchoSource{Interface: "loop0"}), nil); err != nil || set != 1 {
		t.Fatal(err, set)
	}
	if err := d.Delete(ctx, v.Value, nil); err != nil || set != 0 {
		t.Fatal(err)
	}
	r := scheduler.NewRegistry()
	Register(r, f, df7test.Owner, secret)
	if r.Len() != 2 {
		t.Fatal("non-owners register no globals (D-071):", r.Names())
	}
	RegisterGlobals(r, f, df7test.Owner)
	if r.Len() != 3 {
		t.Fatal(r.Names())
	}
	// Delete re-verifies: the echo source now points at another owner's interface → untouched
	set = 3
	f.Reset()
	if err := d.Delete(ctx, v.Value, nil); err != nil || len(f.CallsNamed("bfd_udp_del_echo_source")) != 0 {
		t.Fatal("another owner's echo source must not be deleted", err)
	}
}

func TestEvents(t *testing.T) {
	f := df7test.NewFake()
	f.Reply("want_bfd_events", &bfd.WantBfdEventsReply{})
	ctx, cancel := context.WithCancel(t.Context())
	ch, err := WatchEvents(ctx, f, df7test.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if r := df7test.Last[*bfd.WantBfdEvents](t, f, "want_bfd_events"); !r.EnableDisable || r.PID == 0 {
		t.Fatalf("%+v", r)
	}
	f.Emit(&bfd.BfdUDPSessionEvent{SwIfIndex: 3, LocalAddr: mustAddr("10.9.0.1"), PeerAddr: mustAddr("10.9.0.2"), State: bfd.BFD_STATE_API_UP}) // other owner
	f.Emit(&bfd.BfdUDPSessionEvent{SwIfIndex: 1, LocalAddr: mustAddr("10.0.0.1"), PeerAddr: mustAddr("10.0.0.2"), State: bfd.BFD_STATE_API_UP})
	select {
	case e := <-ch:
		if e.Key != "bfd.udp-session/loop0/10.0.0.1/10.0.0.2" || e.State != StateUp || e.Interface != "loop0" {
			t.Fatalf("%+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	cancel()
	for e := range ch {
		t.Logf("late event %+v", e)
	}
	if r := df7test.Last[*bfd.WantBfdEvents](t, f, "want_bfd_events"); r.EnableDisable {
		t.Fatal("events must be disabled when the watch ends")
	}
	for s, n := range map[bfd.BfdState]string{0: StateAdminDown, 1: StateDown, 2: StateInit, 3: StateUp, 9: "#9"} {
		if StateName(s) != n {
			t.Errorf("%d → %s", s, StateName(s))
		}
	}
}
