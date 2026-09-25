package dhcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
)

// TestRelayRecord: dhcp.relay records are reported only as far as VPP has their proxies; a lost proxy is visible as
// a record with fewer servers; disabled relays need no proxy; the file store survives a new descriptor (restart).
func TestRelayRecord(t *testing.T) {
	ctx := context.Background()
	f, _ := newProxyFake()
	store := &FileRelayStore{Path: filepath.Join(t.TempDir(), "relays.json")}
	proxies := NewProxy(f, WithVRFScope(inScope))
	d := NewRelay(f, store, WithVRFScope(inScope))

	rec := Relay{Name: "lan", Doc: "ZG9j", Enabled: true, RxVRF: 5001, ServerVRF: 0, Src: "10.5.2.1", Servers: []string{"10.5.2.3", "10.5.2.2"}}
	if got := d.KeyOf(rec.Proto()); got != "dhcp.relay/lan" {
		t.Fatalf("key %s", got)
	}
	deps := d.Dependencies(rec.Proto())
	if len(deps) != 2 || deps[0].Key != ProxyKey(5001, 0, "10.5.2.2") || deps[0].Optional {
		t.Fatalf("deps %v", deps)
	}
	for _, srv := range rec.Servers {
		if _, err := proxies.Create(ctx, Proxy{RxVRF: 5001, Server: srv, Src: "10.5.2.1"}.Proto()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Create(ctx, rec.Proto()); err != nil {
		t.Fatal(err)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, rec.Proto()) {
		t.Fatalf("retrieve after create: %v %v", kvs, err)
	}

	// a proxy lost behind the agent's back: the record reports the server VPP still has → drift
	if err := proxies.Delete(ctx, Proxy{RxVRF: 5001, Server: "10.5.2.3", Src: "10.5.2.1"}.Proto(), nil); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	var got Relay
	if err := dfkit.Decode(kvs[0].Value, &got); err != nil || len(got.Servers) != 1 || got.Servers[0] != "10.5.2.2" {
		t.Fatalf("after proxy loss: %+v %v", got, err)
	}

	// restart: a new descriptor over the same file sees the record
	d2 := NewRelay(f, &FileRelayStore{Path: store.Path}, WithVRFScope(inScope))
	if kvs, _ := d2.Retrieve(ctx); len(kvs) != 1 || kvs[0].Key != "dhcp.relay/lan" {
		t.Fatalf("restart: %v", kvs)
	}
	if fi, err := os.Stat(store.Path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("store file %v %v", fi, err)
	}

	// disabled relay: no proxies, record as stored
	off := Relay{Name: "off", Doc: "b2Zm", RxVRF: 5002, Src: "10.5.3.1", Servers: []string{"10.5.3.2"}}
	if d.Dependencies(off.Proto()) != nil {
		t.Fatal("a disabled relay depends on no proxy")
	}
	if _, err := d.Update(ctx, nil, off.Proto(), nil); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	if len(kvs) != 2 || !proto.Equal(kvs[1].Value, off.Proto()) {
		t.Fatalf("disabled: %v", kvs)
	}

	// delete removes the record
	if err := d.Delete(ctx, rec.Proto(), nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, off.Proto(), nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete: %v", kvs)
	}

	// out of scope rx VRF is refused
	if _, err := d.Create(ctx, Relay{Name: "x", Enabled: true, RxVRF: 7, Src: "10.0.0.1", Servers: []string{"10.0.0.2"}}.Proto()); err == nil {
		t.Fatal("rx vrf outside the scope must be refused")
	}
	var _ scheduler.Descriptor = d
}

func TestMemRelayStore(t *testing.T) {
	s := &MemRelayStore{}
	if err := s.Put(Relay{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	m, _ := s.Load()
	if len(m) != 1 {
		t.Fatal(m)
	}
	_ = s.Delete("a")
	if m, _ := s.Load(); len(m) != 0 {
		t.Fatal(m)
	}
	d := NewRelay(nil, nil)
	if d.store == nil {
		t.Fatal("default store")
	}
}

// TestOwnershipDeclarations: the ownership guard (TD-11b) needs every registered descriptor to declare how it records
// ownership; the relay families record none (VRF scope, key).
func TestOwnershipDeclarations(t *testing.T) {
	type noOwnership interface{ RecordsNoOwnership() }
	for _, d := range []any{NewProxy(nil), NewProxyVSS(nil), NewRelay(nil, nil)} {
		if _, ok := d.(noOwnership); !ok {
			t.Errorf("%T declares no ownership mode", d)
		}
	}
	if !(&FileRelayStore{}).Persistent() {
		t.Fatal("file store")
	}
}
