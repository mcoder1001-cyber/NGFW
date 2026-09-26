package nsim_test

import (
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/nsim"
)

type failClaims struct{}

func (failClaims) Claim(string, string) error   { return errors.New("claim store down") }
func (failClaims) Release(string, string) error { return nil }
func (failClaims) Claimed(string, string) bool  { return false }

type failPut struct{ dfkit.BootStore }

func (failPut) Put(dfkit.BootRecord) error { return errors.New("state dir read-only") }

func configured(t *testing.T, f *fakeNsim) {
	t.Helper()
	if _, err := nsim.NewConfig(f, dfkit.NewMemoryBootStore()).Create(ctx, nsim.Config{DelayUsec: 1000, BandwidthBps: 1e6, PacketSize: 1500}.Proto()); err != nil {
		t.Fatal(err)
	}
}

// Review M3 (TD-11b claim first): the cross-connect and the output feature on an untagged interface (eth0) claim before
// the VPP enable; a claim that cannot be recorded sends nothing (the old code enabled first and claimed after).
func TestCreateClaimsFirst(t *testing.T) {
	f := newFake()
	configured(t, f)
	iface.SetClaimStore(owner, failClaims{})
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	store := dfkit.NewMemoryBootStore()
	if _, err := nsim.NewCrossConnect(f, owner, store).Create(ctx, nsim.CrossConnect{A: "loop7001", B: "eth0"}.Proto()); err == nil {
		t.Fatal("cross-connect created without its claim")
	}
	if _, err := nsim.NewOutput(f, owner, store).Create(ctx, nsim.Output{Interface: "eth0"}.Proto()); err == nil {
		t.Fatal("output feature created without its claim")
	}
	if n := len(f.CallsNamed("nsim_cross_connect_enable_disable")) + len(f.CallsNamed("nsim_output_feature_enable_disable")); n != 0 {
		t.Fatalf("%d nsim enables before the claim", n)
	}
}

// An enable whose applied-once record cannot be written is taken back (it would stack on the next Create).
func TestCreateUndoesEnableWithoutRecord(t *testing.T) {
	f := newFake()
	configured(t, f)
	iface.SetClaimStore(owner, nil)
	store := failPut{dfkit.NewMemoryBootStore()}
	if _, err := nsim.NewCrossConnect(f, owner, store).Create(ctx, nsim.CrossConnect{A: "loop7001", B: "eth0"}.Proto()); err == nil {
		t.Fatal("cross-connect without its record")
	}
	if _, err := nsim.NewOutput(f, owner, store).Create(ctx, nsim.Output{Interface: "eth0"}.Proto()); err == nil {
		t.Fatal("output feature without its record")
	}
	if f.cross != 0 || f.output[4] != 0 {
		t.Fatalf("enables left without a record: cross %d output %v", f.cross, f.output)
	}
}
