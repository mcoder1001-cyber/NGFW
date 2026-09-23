package memif_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/memif"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func keyOf(kv scheduler.KV) string { return string(kv.Key) }

func has(kvs []scheduler.KV, key string) bool {
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return true
		}
	}
	return false
}

func create(t *testing.T, d scheduler.Descriptor, desired proto.Message) any {
	t.Helper()
	ctx := context.Background()
	key := string(d.KeyOf(desired))
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatalf("%s Create %s: %v", d.Name(), key, err)
	}
	t.Cleanup(func() {
		if err := d.Delete(ctx, desired, meta); err != nil {
			t.Errorf("%s Delete %s: %v", d.Name(), key, err)
			return
		}
		if kvs, _ := d.Retrieve(ctx); has(kvs, key) {
			t.Errorf("%s: %s still retrieved after Delete", d.Name(), key)
		}
	})
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	got := ifacetest.Find(t, kvs, key, keyOf)
	if !proto.Equal(got.Value, desired) {
		t.Fatalf("%s Retrieve %s = %v, want %v", d.Name(), key, got.Value, desired)
	}
	t.Logf("%s: Retrieve == desired: %s %v", d.Name(), key, got.Value)
	return meta
}

func TestMemifOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	c := ifacetest.Connect(t)
	dir := memif.DefaultSocketDir(owner) // /run/vrx-test/<prefix>/memif
	sd := memif.NewSocket(c, owner, dir)
	sock := &memif.Socket{Id: vpptest.TableBase(t) + 40, Filename: filepath.Join(dir, vpptest.Name(t, "memif40")+".sock")}
	t.Cleanup(func() { _ = os.Remove(sock.Filename) })
	create(t, sd, sock)
	create(t, memif.NewMemif(c, owner), &memif.Memif{Name: vpptest.Name(t, "memif40"), Id: 40, Socket: sock.Id, Role: memif.Role_ROLE_MASTER,
		Mode: memif.Mode_MODE_ETHERNET})
	t.Logf("memif socket %d %s + master memif configured; vppctl show memif", sock.Id, sock.Filename)
	ifacetest.Hold(t)
}
