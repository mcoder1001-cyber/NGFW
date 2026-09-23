package bfd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ngfw/agent/binapi/bfd"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
)

// testSecret is the fixture secret of a conf key: the literal VRX_TEST_PSK_<id> (00-CONTEXT:
// test fixtures never carry real secrets).
func testSecret(_ context.Context, id uint32) ([]byte, error) {
	return []byte(fmt.Sprintf("VRX_TEST_PSK_%d", id)), nil
}

// Host test: sessions on this slot's loopback (loop<slot>60, 10.<slot>.60.1/24) towards peers
// that do not exist — they stay down; the test asserts configuration and state shape only.
// The conf-key id comes from this slot's range. The echo source is VPP-wide: it is set only
// when nobody else has set it, and deleted afterwards.
func TestBFDOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	opts := []df7.Option{df7.WithIDRange(h.TableB, h.TableB+999)}
	kd, sd, ed := NewAuthKey(h.C, h.Owner, testSecret, opts...), NewSession(h.C, h.Owner, opts...), NewEchoSource(h.C, h.Owner, opts...)
	ifA, idxA := h.Loopback(60, true, true)
	h.Address(idxA, h.Addr(60, 1)+"/24")
	h.CleanupOwned(kd)
	h.CleanupOwned(sd) // registered after the key → cleaned first

	events, err := WatchEvents(h.Ctx, h.C, h.Owner, opts...)
	h.Must("want_bfd_events", err)

	keyID := h.TableB + 60
	keys := []scheduler.KV{df7test.Desired(kd, df7.Encode(AuthKey{ID: keyID, Type: AuthMeticulousKeyedSHA1}))}
	sessions := []scheduler.KV{
		df7test.Desired(sd, df7.Encode(Session{Interface: ifA, Local: h.Addr(60, 1), Peer: h.Addr(60, 2), DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3,
			Auth: &SessionAuth{ConfKeyID: keyID, BFDKeyID: 1}})),
		df7test.Desired(sd, df7.Encode(Session{Interface: ifA, Local: h.Addr(60, 1), Peer: h.Addr(60, 3), DesiredMinTx: 500000, RequiredMinRx: 400000, DetectMult: 5, AdminDown: true})),
	}
	ck := h.Apply(kd, keys...)
	cs := h.Apply(sd, sessions...)
	h.ExpectRetrieved(kd, keys...)
	h.ExpectRetrieved(sd, sessions...)

	t.Run("update in place", func(t *testing.T) {
		s := Session{Interface: ifA, Local: h.Addr(60, 1), Peer: h.Addr(60, 3), DesiredMinTx: 200000, RequiredMinRx: 200000, DetectMult: 4,
			Auth: &SessionAuth{ConfKeyID: keyID, BFDKeyID: 2}}
		m, err := sd.Update(h.Ctx, sessions[1].Value, df7.Encode(s), cs[1].Meta)
		h.Must("update", err)
		sessions[1].Value, cs[1].Value, cs[1].Meta = df7.Encode(s), df7.Encode(s), m
		h.ExpectRetrieved(sd, sessions...)
	})

	t.Run("events", func(t *testing.T) {
		deadline := time.After(3 * time.Second)
		for {
			select {
			case e := <-events:
				t.Logf("bfd event: %+v", e)
				return
			case <-deadline:
				t.Log("no bfd_udp_session_event within 3s (sessions without a peer may not change state)")
				return
			}
		}
	})

	t.Run("echo source", func(t *testing.T) {
		rep, err := bfd.NewServiceClient(h.C).BfdUDPGetEchoSource(h.Ctx, &bfd.BfdUDPGetEchoSource{})
		h.Must("bfd_udp_get_echo_source", err)
		if rep.IsSet {
			t.Skipf("skip: the VPP-wide echo source is already set (sw_if_index %d) — not ours to change", rep.SwIfIndex)
		}
		v := df7.Encode(EchoSource{Interface: ifA})
		ce := h.Apply(ed, df7test.Desired(ed, v))
		t.Cleanup(func() { h.DeleteAll(ed, ce) })
		h.ExpectRetrieved(ed, df7test.Desired(ed, v))
		h.DeleteAll(ed, ce)
		ce = nil
		h.ExpectNone(ed)
	})

	st, err := Sessions(h.Ctx, h.C, h.Owner, opts...)
	h.Must("sessions", err)
	t.Logf("live state: %+v", st)
	h.Hold("bfd sessions")

	t.Run("restart simulation", func(t *testing.T) {
		c := df7test.Connect(t)
		h.ExpectRetrieved(NewAuthKey(c, h.Owner, nil, opts...), keys...)
		h.ExpectRetrieved(NewSession(c, h.Owner, opts...), sessions...)
	})

	h.DeleteAll(sd, cs)
	h.DeleteAll(kd, ck)
	h.ExpectNone(sd)
	h.ExpectNone(kd)
}
