package hasync

import (
	"context"
	"errors"
	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/ip_types"
	nat "ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/vpp/fake"
	"testing"
	"time"
)

func TestOwnerLifecycleAndSlotRequireOnly(t *testing.T) {
	ctx := context.Background()
	f := fake.New()
	live := Listener{Address: "192.0.2.1", Port: 8750, PathMtu: 1500}
	f.On("nat44_ei_ha_get_listener", func(api.Message) ([]api.Message, error) {
		ip, _ := Address(live.Address)
		return []api.Message{&nat.Nat44EiHaGetListenerReply{IPAddress: ip, Port: live.Port, PathMtu: live.PathMtu}}, nil
	})
	f.On("nat44_ei_ha_set_listener", func(m api.Message) ([]api.Message, error) {
		q := m.(*nat.Nat44EiHaSetListener)
		live = Listener{q.IPAddress.String(), q.Port, q.PathMtu}
		return []api.Message{&nat.Nat44EiHaSetListenerReply{}}, nil
	})
	owner := NewListener(f, natcommon.WithGlobalsOwner(true), natcommon.WithLockDir(t.TempDir()))
	slot := NewListener(f)
	want := dfkit.Encode(live)
	if _, err := slot.Create(ctx, want); err != nil {
		t.Fatal(err)
	}
	if len(f.CallsNamed("nat44_ei_ha_set_listener")) != 0 {
		t.Fatal("slot changed globals")
	}
	wrong := dfkit.Encode(Listener{"192.0.2.9", 8750, 1500})
	if _, err := slot.Create(ctx, wrong); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("%v", err)
	}
	if _, err := owner.Create(ctx, wrong); err != nil {
		t.Fatal(err)
	}
	kvs, err := owner.Retrieve(ctx)
	if err != nil || len(kvs) != 1 {
		t.Fatalf("%v %v", kvs, err)
	}
	got, _ := natcommon.Decode[Listener](kvs[0].Value)
	if got.Address != "192.0.2.9" {
		t.Fatal(got)
	}
	// Restart reads the exact native singleton; rollback restores the prior spec.
	restarted := NewListener(f, natcommon.WithGlobalsOwner(true), natcommon.WithLockDir(t.TempDir()))
	if observed, err := restarted.Retrieve(ctx); err != nil || len(observed) != 1 {
		t.Fatalf("restart retrieve %v %v", observed, err)
	}
	if _, err := owner.Update(ctx, wrong, want, nil); err != nil || live.Address != "192.0.2.1" {
		t.Fatalf("rollback %+v %v", live, err)
	}
	if err := slot.Delete(ctx, wrong, nil); err != nil || live.Port == 0 {
		t.Fatalf("slot reset: %v", err)
	}
	if err := owner.Delete(ctx, wrong, nil); err != nil || live.Port != 0 || live.Address != (ip_types.IP4Address{}).String() {
		t.Fatalf("reset %+v %v", live, err)
	}
}
func TestValidationAndCanceledLock(t *testing.T) {
	for _, addr := range []string{"0.0.0.0", "127.0.0.1", "224.0.0.1", "::1", "255.255.255.255"} {
		if _, err := Address(addr); err == nil {
			t.Fatal(addr)
		}
	}
	dir := t.TempDir()
	release, err := Lock(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := Lock(ctx, dir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestFailoverTypedSetterAndReset(t *testing.T) {
	f := fake.New()
	f.Reply("nat44_ei_ha_set_failover", &nat.Nat44EiHaSetFailoverReply{})
	d := NewFailover(f, natcommon.WithGlobalsOwner(true), natcommon.WithLockDir(t.TempDir()))
	spec := dfkit.Encode(Failover{Address: "192.0.2.2", Port: 8750, SessionRefreshSec: 10})
	if _, err := d.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	q := f.CallsNamed("nat44_ei_ha_set_failover")[0].(*nat.Nat44EiHaSetFailover)
	if q.IPAddress.String() != "192.0.2.2" || q.Port != 8750 || q.SessionRefreshInterval != 10 {
		t.Fatalf("%+v", q)
	}
	if err := d.Delete(context.Background(), spec, nil); err != nil {
		t.Fatal(err)
	}
	q = f.CallsNamed("nat44_ei_ha_set_failover")[1].(*nat.Nat44EiHaSetFailover)
	if q.IPAddress != (ip_types.IP4Address{}) || q.Port != 0 {
		t.Fatalf("reset %+v", q)
	}
	f.Reply("nat44_ei_ha_set_failover", &nat.Nat44EiHaSetFailoverReply{Retval: -1})
	if _, err := d.Create(context.Background(), spec); err == nil {
		t.Fatal("native setter failure hidden")
	}
}
