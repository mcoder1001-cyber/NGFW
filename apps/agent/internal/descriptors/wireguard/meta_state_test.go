package wireguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/wireguard"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	wgd "ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/scheduler"
)

func TestMetaDescriptor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "wireguard-meta-w4.json")
	store, err := wgd.NewFileMetaStore(path)
	if err != nil {
		t.Fatal(err)
	}
	d := wgd.NewMeta(store)
	itf := wgd.MetaSpec{ID: "wg4001", Name: "site-a", Description: "HQ", SecretRef: "key/site-a", UnderlayVrf: "default", RouteAllowedIps: true}
	peer := wgd.MetaSpec{ID: "wg4001/AAAA/B=", Name: "branch-1", SecretRef: "psk/b1"}
	for _, s := range []wgd.MetaSpec{itf, peer} {
		v := wgd.MetaValue(s)
		if k := d.KeyOf(v); k != scheduler.Join(wgd.MetaName, s.ID) {
			t.Fatalf("KeyOf = %s", k)
		}
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	// a fresh descriptor over the same file (agent restart) retrieves both, sorted, equal to desired
	kvs, err := wgd.NewMeta(mustStore(t, path)).Retrieve(ctx)
	if err != nil || len(kvs) != 2 {
		t.Fatalf("Retrieve = %v, %v", kvs, err)
	}
	if !proto.Equal(kvs[0].Value, wgd.MetaValue(itf)) || !proto.Equal(kvs[1].Value, wgd.MetaValue(peer)) {
		t.Fatalf("Retrieve = %v", kvs)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("store file mode %v, %v", info.Mode(), err)
	}
	// update, delete, delete again (no-op)
	itf.Description = "HQ 2"
	if _, err := d.Update(ctx, wgd.MetaValue(itf), wgd.MetaValue(itf), nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, wgd.MetaValue(peer), nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, wgd.MetaValue(peer), nil); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	if len(kvs) != 1 || !proto.Equal(kvs[0].Value, wgd.MetaValue(itf)) {
		t.Fatalf("after update/delete: %v", kvs)
	}
	if _, err := d.Create(ctx, wgd.MetaValue(wgd.MetaSpec{ID: "wg1"})); err == nil {
		t.Fatal("an entry without a name must be refused")
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "VRX_TEST") {
		t.Fatal("the store holds references only")
	}
}

func mustStore(t *testing.T, path string) wgd.MetaStore {
	t.Helper()
	s, err := wgd.NewFileMetaStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDumpState(t *testing.T) {
	e := newEnv(t)
	if _, err := e.itf.Create(ctx, e.itfV()); err != nil {
		t.Fatal(err)
	}
	pv := e.peerV()
	pm, err := e.peer.Create(ctx, pv)
	if err != nil {
		t.Fatal(err)
	}
	// another owner's interface and peer are not reported
	foreign := wgd.Config{Keys: keys, Client: e.v, Owner: "w3", Secrets: e.vec.resolver}
	if _, err := wgd.NewInterface(foreign).Create(ctx, &vpnpb.WireguardInterface{Instance: 3001, PrivateKey: e.ref, Port: 20310, SrcIp: "10.3.8.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := wgd.NewPeer(foreign).Create(ctx, &vpnpb.WireguardPeer{Interface: "wg3001", PublicKey: e.vec.peerPub[1], AllowedIps: []string{"10.3.0.0/16"}}); err != nil {
		t.Fatal(err)
	}
	e.v.flags[pm.(wgd.PeerMeta).PeerIndex] = wireguard.WIREGUARD_PEER_ESTABLISHED

	st, err := wgd.DumpState(ctx, e.v, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 1 || st[0].Name != "wg4001" || st[0].Instance != 4001 || st[0].Port != 20410 || st[0].SrcIP != "10.4.8.1" {
		t.Fatalf("state %+v", st)
	}
	if "x25519:"+st[0].PublicKey != e.ref {
		t.Fatalf("public key %s does not match the reference", st[0].PublicKey)
	}
	if len(st[0].Peers) != 1 {
		t.Fatalf("peers %+v", st[0].Peers)
	}
	p := st[0].Peers[0]
	if p.PublicKey != pv.PublicKey || !p.Established || p.Dead || p.Endpoint != "10.4.8.2" || p.EndpointPort != 20411 ||
		p.PersistentKeepalive != 25 || p.TableID != 4001 || strings.Join(p.AllowedIps, ",") != "10.4.10.0/24,10.4.9.0/24,fd00:4::/64" {
		t.Fatalf("peer %+v", p)
	}
	if st, _ := wgd.DumpState(ctx, e.v, owner, "wg9"); len(st) != 0 {
		t.Fatalf("name filter: %+v", st)
	}
	if calls := e.v.CallsNamed("wireguard_peers_v2_dump"); len(calls) != 0 {
		t.Fatal("state must not dump preshared keys (use wireguard_peers_dump)")
	}
	for _, c := range e.v.CallsNamed("wireguard_interface_dump") {
		if c.(*wireguard.WireguardInterfaceDump).ShowPrivateKey {
			t.Fatal("show_private_key must never be set")
		}
	}
}

func TestOwnershipDeclarations(t *testing.T) {
	type noOwnership interface{ RecordsNoOwnership() }
	type checker interface{ CheckPersistent() error }
	cfg := wgd.Config{Client: newFakeVPP(), Owner: owner, Keys: keys}
	for _, d := range []any{wgd.NewInterface(cfg), wgd.NewPeer(cfg), wgd.NewAsyncMode(cfg)} {
		if _, ok := d.(noOwnership); !ok {
			t.Fatalf("%T must declare RecordsNoOwnership (TD-11b)", d)
		}
		if _, ok := d.(checker); ok {
			t.Fatalf("%T declares both", d)
		}
	}
	if err := wgd.NewMeta(nil).CheckPersistent(); err == nil {
		t.Fatal("an in-memory metadata store must fail the product guard")
	}
	s, err := wgd.NewFileMetaStore(filepath.Join(t.TempDir(), "m.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := wgd.NewMeta(s).CheckPersistent(); err != nil {
		t.Fatal(err)
	}
}
