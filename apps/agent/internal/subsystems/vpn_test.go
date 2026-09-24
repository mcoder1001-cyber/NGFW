package subsystems

import (
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

// DF-5 / D-096 (Q12): the product agent hands the VPN families the owner's persisted record store and the
// agent-local 0600 fingerprint key — never the in-memory defaults.
func TestVPNOptionsArePersisted(t *testing.T) {
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "w1")
	if err != nil {
		t.Fatal(err)
	}
	w, err := Register(scheduler.NewRegistry(), Env{Client: coretest.New(), Owner: "w1", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	iopts, err := w.IPsecOptions()
	if err != nil {
		t.Fatal(err)
	}
	var ic ipsec.Config
	for _, o := range iopts {
		o(&ic)
	}
	if _, ok := ic.Boot.(*dfkit.FileBootStore); !ok || ic.Keys == nil || ic.GlobalsOwner {
		t.Fatalf("ipsec options: boot %T keys %v globals %v", ic.Boot, ic.Keys != nil, ic.GlobalsOwner)
	}
	kopts, err := w.IKEv2Options()
	if err != nil {
		t.Fatal(err)
	}
	var kc ikev2.Config
	for _, o := range kopts {
		o(&kc)
	}
	if kc.Boot != ic.Boot || kc.Keys != ic.Keys {
		t.Fatal("ikev2 must share the owner's store and key with ipsec")
	}
	fi, err := os.Stat(filepath.Join(dir, "vpn-w1.key"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", fi, err)
	}
}
