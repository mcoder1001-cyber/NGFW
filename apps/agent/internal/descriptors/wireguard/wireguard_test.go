package wireguard_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	wgd "ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

const owner = "w4"

var ctx = context.Background()

type env struct {
	v    *fakeVPP
	vec  vectors
	ref  string
	cfg  wgd.Config
	itf  *wgd.Interface
	peer *wgd.Peer
}

func newEnv(t *testing.T) env {
	t.Helper()
	v := newFakeVPP()
	vec, ref := slotVectors(t, 4)
	cfg := wgd.Config{Client: v, Owner: owner, Secrets: vec.resolver}
	return env{v: v, vec: vec, ref: ref, cfg: cfg, itf: wgd.NewInterface(cfg), peer: wgd.NewPeer(cfg)}
}

func (e env) itfV() *vpnpb.WireguardInterface {
	return &vpnpb.WireguardInterface{Instance: 4001, PrivateKey: e.ref, Port: 20410, SrcIp: "10.4.8.1"}
}

func (e env) peerV() *vpnpb.WireguardPeer {
	return &vpnpb.WireguardPeer{Interface: "wg4001", PublicKey: e.vec.peerPub[0], Endpoint: "10.4.8.2", Port: 20411,
		AllowedIps: []string{"10.4.10.0/24", "10.4.9.0/24", "fd00:4::/64"}, PersistentKeepalive: 25, TableId: 4001, PresharedKey: e.vec.pskRef}
}

func mustRetrieve(t *testing.T, d scheduler.Descriptor, want ...proto.Message) []scheduler.KV {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(kvs) != len(want) {
		t.Fatalf("Retrieve returned %d objects, want %d: %v", len(kvs), len(want), kvs)
	}
	for i := range want {
		if !proto.Equal(kvs[i].Value, want[i]) {
			t.Fatalf("Retrieve[%d] = %v\nwant %v", i, kvs[i].Value, want[i])
		}
	}
	return kvs
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	p := wgd.Register(reg, newFakeVPP(), owner)
	if got := strings.Join(reg.Names(), ","); got != "wireguard.interface,wireguard.peer,wireguard.async-mode" {
		t.Fatalf("registered %s", got)
	}
	if p == nil || p.Name() != wgd.PeerName || wgd.InterfaceName != "wireguard.interface" {
		t.Fatal("Register returns the peer descriptor (event source)")
	}
	// D-071: the async-mode global is a requirement unless this agent is the globals owner
	for _, owns := range []bool{false, true} {
		reg := scheduler.NewRegistry()
		wgd.Register(reg, newFakeVPP(), owner, wgd.WithGlobalsOwner(owns))
		d, _ := reg.Get(wgd.AsyncModeName)
		if _, isReq := d.(*vpn.Require); isReq == owns {
			t.Fatalf("globals owner=%v registered %T", owns, d)
		}
		if a, ok := d.(scheduler.AbsenceDeleter); !ok || a.DeleteOnAbsence() {
			t.Fatal("a global is never deleted on absence")
		}
	}
}

func TestInterface(t *testing.T) {
	e := newEnv(t)
	want := e.itfV()
	if e.itf.KeyOf(want) != "wireguard.interface/wg4001" || e.itf.Dependencies(want) != nil {
		t.Fatalf("key/deps %s", e.itf.KeyOf(want))
	}
	// D-065: peers reference the interface by its alias, which the interface provides
	if pk := e.itf.ProvidedKeys(want); len(pk) != 1 || pk[0] != e.peer.Dependencies(e.peerV())[0].Key {
		t.Fatalf("provided %v", pk)
	}
	meta, err := e.itf.Create(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	req := e.v.CallsNamed("wireguard_interface_create")[0].(*wireguard.WireguardInterfaceCreate)
	if req.GenerateKey || req.Interface.Port != 20410 || req.Interface.UserInstance != 4001 || bytes.Contains(req.Interface.PrivateKey, e.vec.itfPriv) {
		t.Fatalf("create request: generate %v port %d; the private key buffer must be zeroed after the call", req.GenerateKey, req.Interface.Port)
	}
	if e.v.ifaces[meta.(wgd.InterfaceMeta).SwIfIndex].Tag != "w4:wg4001" {
		t.Fatal("owner tag")
	}
	for _, c := range e.v.CallsNamed("wireguard_interface_dump") {
		if c.(*wireguard.WireguardInterfaceDump).ShowPrivateKey {
			t.Fatal("show_private_key must never be set")
		}
	}
	// Retrieve rebuilds the x25519 reference from the public key: equal, also after a restart
	mustRetrieve(t, e.itf, want)
	mustRetrieve(t, wgd.NewInterface(e.cfg), want)
	// another owner's interface is invisible
	foreign := wgd.NewInterface(wgd.Config{Client: e.v, Owner: "w3", Secrets: e.vec.resolver})
	if _, err := foreign.Create(ctx, &vpnpb.WireguardInterface{Instance: 3001, PrivateKey: e.ref, Port: 20310, SrcIp: "10.3.8.1"}); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, e.itf, want)
	if _, err := e.itf.Update(ctx, want, want, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := e.itf.Delete(ctx, want, meta); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, e.itf)
	if err := e.itf.Delete(ctx, want, meta); err != nil {
		t.Fatalf("second delete is a no-op (D-074): %v", err)
	}
	// the index now held by another owner's interface is never deleted (D-071)
	var fidx uint32
	for idx, i := range e.v.ifaces {
		if i.Tag == "w3:wg3001" {
			fidx = idx
		}
	}
	if err := e.itf.Delete(ctx, want, wgd.InterfaceMeta{SwIfIndex: fidx}); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("stale index: %v", err)
	}
	if n := len(e.v.CallsNamed("wireguard_interface_delete")); n != 1 {
		t.Fatalf("%d deletes reached VPP", n)
	}
}

func TestInterfaceValidation(t *testing.T) {
	e := newEnv(t)
	other, _ := vpn.X25519Ref(bytes.Repeat([]byte{7}, 32))
	for name, o := range map[string]*vpnpb.WireguardInterface{
		"no port":       {Instance: 1, PrivateKey: e.ref, SrcIp: "10.4.0.1"},
		"port too big":  {Instance: 1, PrivateKey: e.ref, Port: 70000, SrcIp: "10.4.0.1"},
		"no key":        {Instance: 1, Port: 1, SrcIp: "10.4.0.1"},
		"any instance":  {Instance: ^uint32(0), PrivateKey: e.ref, Port: 1, SrcIp: "10.4.0.1"},
		"bad src":       {Instance: 1, PrivateKey: e.ref, Port: 1, SrcIp: "nope"},
		"unknown key":   {Instance: 1, PrivateKey: other, Port: 1, SrcIp: "10.4.0.1"},
		"sha256 as key": {Instance: 1, PrivateKey: e.vec.pskRef, Port: 1, SrcIp: "10.4.0.1"},
	} {
		if _, err := e.itf.Create(ctx, o); err == nil {
			t.Fatalf("%s: Create must fail", name)
		}
	}
	e.v.SetConnected(false)
	if _, err := e.itf.Create(ctx, e.itfV()); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
}

func TestPeer(t *testing.T) {
	e := newEnv(t)
	if _, err := e.itf.Create(ctx, e.itfV()); err != nil {
		t.Fatal(err)
	}
	want := e.peerV()
	want.AllowedIps = []string{"10.4.10.0/24", "10.4.9.0/24", "fd00:4::/64"} // string order
	if got := e.peer.KeyOf(want); got != scheduler.Key("wireguard.peer/wg4001/"+e.vec.peerPub[0]) {
		t.Fatalf("key %s", got)
	}
	deps := e.peer.Dependencies(want)
	if fmt.Sprint(deps) != fmt.Sprint([]scheduler.Dependency{{Key: "interface/wg4001"}, {Key: "vrf/4001", Optional: true}}) {
		t.Fatalf("deps %v", deps)
	}
	meta, err := e.peer.Create(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	req := e.v.CallsNamed("wireguard_peer_add_v2")[0].(*wireguard.WireguardPeerAddV2)
	if !req.Peer.PresharedKeySet || bytes.Contains(req.Peer.PresharedKey, e.vec.psk) || req.Peer.NAllowedIps != 3 {
		t.Fatal("peer_add_v2: psk set, request buffer zeroed after the call, 3 allowed ips")
	}
	// VPP reorders allowed ips (the fake reverses them): Retrieve canonicalises; psk → reference
	mustRetrieve(t, e.peer, want)
	mustRetrieve(t, wgd.NewPeer(e.cfg), want)

	minimal := &vpnpb.WireguardPeer{Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.11.0/24"}}
	m2, err := e.peer.Create(ctx, minimal)
	if err != nil {
		t.Fatal(err)
	}
	if d := e.peer.Dependencies(minimal); len(d) != 1 {
		t.Fatalf("table 0: no vrf dependency, got %v", d)
	}
	kvs := mustRetrieve(t, e.peer, want, minimal) // sorted by key (public key D2Mq… < kNb/…)
	if kvs[1].Meta.(wgd.PeerMeta).PeerIndex != m2.(wgd.PeerMeta).PeerIndex {
		t.Fatalf("meta %+v", kvs[1].Meta)
	}
	if _, err := e.peer.Create(ctx, minimal); err == nil {
		t.Fatal("duplicate public key must fail")
	}
	if _, err := e.peer.Update(ctx, want, minimal, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	// a stale index (the peer is gone, the index now holds another peer) touches nothing (D-071)
	stale := wgd.PeerMeta{PeerIndex: m2.(wgd.PeerMeta).PeerIndex, SwIfIndex: meta.(wgd.PeerMeta).SwIfIndex}
	if err := e.peer.Delete(ctx, want, stale); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, e.peer, want, minimal)
	for _, x := range []struct {
		v *vpnpb.WireguardPeer
		m any
	}{{want, meta}, {minimal, m2}} {
		if err := e.peer.Delete(ctx, x.v, x.m); err != nil {
			t.Fatal(err)
		}
	}
	mustRetrieve(t, e.peer)
	if err := e.peer.Delete(ctx, want, meta); err != nil {
		t.Fatalf("second delete is a no-op (D-074): %v", err)
	}
	if n := len(e.v.CallsNamed("wireguard_peer_remove")); n != 2 {
		t.Fatalf("%d removes reached VPP, want 2", n)
	}
}

func TestPeerOwnershipAndValidation(t *testing.T) {
	e := newEnv(t)
	foreign := wgd.Config{Client: e.v, Owner: "w3", Secrets: e.vec.resolver}
	if _, err := wgd.NewInterface(foreign).Create(ctx, &vpnpb.WireguardInterface{Instance: 3001, PrivateKey: e.ref, Port: 20310, SrcIp: "10.3.8.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := wgd.NewPeer(foreign).Create(ctx, &vpnpb.WireguardPeer{Interface: "wg3001", PublicKey: e.vec.peerPub[0], AllowedIps: []string{"10.3.0.0/16"}, PresharedKey: e.vec.pskRef}); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, e.peer) // the w3 peer is invisible to w4

	if _, err := e.itf.Create(ctx, e.itfV()); err != nil {
		t.Fatal(err)
	}
	// D-069: peers only on our own WireGuard interfaces, by logical name
	untagged := e.v.addIface("wg4002", "")
	e.v.wgs[untagged] = wireguard.WireguardInterface{UserInstance: 4002, SwIfIndex: interfaceIndex(untagged)}
	for name, want := range map[string]error{"wg3001": vpn.ErrForeignInterface, "wg4002": vpn.ErrNotOurs, "wg4999": vpn.ErrNoInterface} {
		p := &vpnpb.WireguardPeer{Interface: name, PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.0.0/16"}}
		if _, err := e.peer.Create(ctx, p); !errors.Is(err, want) {
			t.Fatalf("%s: %v, want %v", name, err, want)
		}
	}
	for name, p := range map[string]*vpnpb.WireguardPeer{
		"bad pubkey":      {Interface: "wg4001", PublicKey: "short", AllowedIps: []string{"10.4.0.0/16"}},
		"no allowed ips":  {Interface: "wg4001", PublicKey: e.vec.peerPub[1]},
		"host bits":       {Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.0.1/16"}},
		"unsorted":        {Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.9.0/24", "10.4.10.0/24"}},
		"duplicate":       {Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.9.0/24", "10.4.9.0/24"}},
		"not canonical":   {Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"fd00:0004::/64"}},
		"bad endpoint":    {Interface: "wg4001", PublicKey: e.vec.peerPub[1], Endpoint: "x", AllowedIps: []string{"10.4.0.0/16"}},
		"no interface":    {Interface: "wg4999", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.0.0/16"}},
		"unknown psk":     {Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.0.0/16"}, PresharedKey: vpn.Ref([]byte("x"))},
		"keepalive range": {Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.0.0/16"}, PersistentKeepalive: 70000},
	} {
		if _, err := e.peer.Create(ctx, p); err == nil {
			t.Fatalf("%s: Create must fail", name)
		}
	}
}

func TestPeerEvents(t *testing.T) {
	e := newEnv(t)
	if _, err := e.itf.Create(ctx, e.itfV()); err != nil {
		t.Fatal(err)
	}
	existing := e.peerV()
	existing.AllowedIps = []string{"10.4.9.0/24"}
	em, err := e.peer.Create(ctx, existing)
	if err != nil {
		t.Fatal(err)
	}
	// a peer another owner has on its own interface: its events must not surface
	foreign := wgd.Config{Client: e.v, Owner: "w3", Secrets: e.vec.resolver}
	if _, err := wgd.NewInterface(foreign).Create(ctx, &vpnpb.WireguardInterface{Instance: 3001, PrivateKey: e.ref, Port: 20310, SrcIp: "10.3.8.1"}); err != nil {
		t.Fatal(err)
	}
	fm, err := wgd.NewPeer(foreign).Create(ctx, &vpnpb.WireguardPeer{Interface: "wg3001", PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", AllowedIps: []string{"10.3.0.0/16"}})
	if err != nil {
		t.Fatal(err)
	}

	evCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := e.peer.Events(evCtx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.peer.Events(evCtx); !errors.Is(err, wgd.ErrEventsActive) {
		t.Fatalf("second subscription: %v", err)
	}
	// a peer created while subscribed is registered individually (VPP registers existing peers only)
	later := &vpnpb.WireguardPeer{Interface: "wg4001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.4.11.0/24"}}
	lm, err := e.peer.Create(ctx, later)
	if err != nil {
		t.Fatal(err)
	}
	if !e.v.registered[lm.(wgd.PeerMeta).PeerIndex] {
		t.Fatal("a peer created during a subscription must be registered for events")
	}

	next := func() wgd.PeerEvent {
		t.Helper()
		select {
		case ev := <-events:
			return ev
		case <-time.After(2 * time.Second):
			t.Fatal("no event")
		}
		return wgd.PeerEvent{}
	}
	e.v.emit(fm.(wgd.PeerMeta).PeerIndex, wireguard.WIREGUARD_PEER_ESTABLISHED) // foreign: filtered
	if !e.v.emit(em.(wgd.PeerMeta).PeerIndex, wireguard.WIREGUARD_PEER_ESTABLISHED) {
		t.Fatal("existing peer not registered")
	}
	if ev := next(); ev.Key != e.peer.KeyOf(existing) || !ev.Established || ev.Dead || ev.Interface != "wg4001" {
		t.Fatalf("event %+v", ev)
	}
	e.v.emit(lm.(wgd.PeerMeta).PeerIndex, wireguard.WIREGUARD_PEER_STATUS_DEAD)
	if ev := next(); ev.Key != e.peer.KeyOf(later) || ev.Established || !ev.Dead {
		t.Fatalf("event %+v", ev)
	}
	// an index this process never saw is resolved through wireguard_peers_dump (after a restart)
	fresh := wgd.NewPeer(e.cfg)
	fctx, fcancel := context.WithCancel(ctx)
	cancel()
	for range events { //nolint:revive // drain until closed
	}
	fev, err := fresh.Events(fctx)
	if err != nil {
		t.Fatal(err)
	}
	e.v.emit(em.(wgd.PeerMeta).PeerIndex, wireguard.WIREGUARD_PEER_ESTABLISHED)
	select {
	case ev := <-fev:
		if ev.Key != e.peer.KeyOf(existing) || ev.PublicKey != existing.PublicKey {
			t.Fatalf("resolved event %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event after restart")
	}
	fcancel()
	for range fev { //nolint:revive // drain until closed
	}
	if calls := e.v.CallsNamed("wireguard_peers_v2_dump"); len(calls) != 0 {
		t.Fatal("events must not dump preshared keys (use wireguard_peers_dump)")
	}
}

func TestAsyncMode(t *testing.T) {
	v := newFakeVPP()
	d := wgd.NewAsyncMode(wgd.Config{Client: v, Owner: owner})
	// D-063: no getter → write-only, never an echo of what was applied
	if kvs, err := d.Retrieve(ctx); !errors.Is(err, vpn.ErrRetrieveUnsupported) || kvs != nil {
		t.Fatalf("Retrieve: %v %v", kvs, err)
	}
	if _, err := d.Create(ctx, &vpnpb.WireguardAsyncMode{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, vpn.ErrRetrieveUnsupported) {
		t.Fatal("still write-only after Create")
	}
	if _, err := d.Update(ctx, nil, &vpnpb.WireguardAsyncMode{}, nil); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(v.async) != "[true false]" || d.KeyOf(nil) != "wireguard.async-mode/global" {
		t.Fatalf("calls %v", v.async)
	}
	if err := d.Delete(ctx, nil, nil); err != nil || len(v.async) != 2 {
		t.Fatal("Delete leaves VPP alone")
	}
}

// TestNoMaterialInOutput formats every value, key, meta and error the descriptors produce with
// %v/%+v and through slog and asserts neither the private key nor the preshared key appears.
func TestNoMaterialInOutput(t *testing.T) {
	e := newEnv(t)
	im, err := e.itf.Create(ctx, e.itfV())
	if err != nil {
		t.Fatal(err)
	}
	pm, err := e.peer.Create(ctx, e.peerV())
	if err != nil {
		t.Fatal(err)
	}
	ikvs, _ := e.itf.Retrieve(ctx)
	pkvs, _ := e.peer.Retrieve(ctx)
	_, errDup := e.peer.Create(ctx, e.peerV())
	_, errItf := e.itf.Create(ctx, e.itfV())
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("wg", "itf", e.itfV(), "peer", e.peerV(), "metas", []any{im, pm}, "retrieved", append(ikvs, pkvs...), "err", errDup, "err2", errItf)
	fmt.Fprintf(&buf, "%v %+v %+v %v %v %+v %+v", e.peerV(), ikvs, pkvs, errDup, errItf, e.itf, e.peer)
	for _, secret := range [][]byte{e.vec.itfPriv, e.vec.psk} {
		for _, enc := range []string{string(secret), fmt.Sprintf("%d", secret), fmt.Sprintf("%x", secret)} {
			if strings.Contains(buf.String(), enc) {
				t.Fatalf("key material leaked into formatted output:\n%s", buf.String())
			}
		}
	}
}
