package subsystems

import (
	"encoding/base64"
	"fmt"
	"ngfw/agent/internal/vpp/bootid"
	"testing"
)

func TestBfdEndpointIndexCapacityRecovery(t *testing.T) {
	dir := t.TempDir()
	id := &Identity{}
	id.Set(bootid.Identity{BootID: "b", PID: 10, StartTime: 100})
	store, err := OpenKeyedClaims(dir, "bfd-endpoints", "w14", id)
	if err != nil {
		t.Fatal(err)
	}
	c := &bfdEndpointClaims{store: store}
	store.Begin()
	for i := 0; i < 1024; i++ {
		if err := c.Claim("local", fmt.Sprint(i), "w14-lan"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	before := store.endpointIndexVisits
	for pass := 0; pass < 2; pass++ {
		for i := 0; i < 1024; i++ {
			if name, ok := c.Lookup("local", fmt.Sprint(i)); !ok || name != "w14-lan" {
				t.Fatal(i)
			}
		}
	}
	if got := store.endpointIndexVisits - before; got != 2048 {
		t.Fatalf("visits %d, want 2048", got)
	}
	reopened, err := OpenKeyedClaims(dir, "bfd-endpoints", "w14", id)
	if err != nil {
		t.Fatal(err)
	}
	c = &bfdEndpointClaims{store: reopened}
	if _, ok := c.Lookup("local", "3"); !ok {
		t.Fatal("restart")
	}
	if reopened.endpointIndexVisits != 1025 {
		t.Fatal("nonlinear rebuild")
	}
	key := bfdEndpointPrefix("local", "3") + base64.RawURLEncoding.EncodeToString([]byte("foreign"))
	if err := reopened.Claim(key); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup("local", "3"); ok {
		t.Fatal("ambiguity adopted")
	}
	if err := c.Claim("local", "3", "w14-lan"); err == nil {
		t.Fatal("conflict accepted")
	}
	if err := reopened.Release(key); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup("local", "3"); !ok {
		t.Fatal("stale release")
	}
	id.Set(bootid.Identity{BootID: "b", PID: 11, StartTime: 200})
	if _, ok := c.Lookup("local", "3"); ok {
		t.Fatal("old boot adopted")
	}
	if _, err := reopened.Prune(); err != nil {
		t.Fatal(err)
	}
	if len(reopened.endpointIndex) != 0 {
		t.Fatal("stale prune")
	}
}
func TestBfdObservationBoundedLifecycle(t *testing.T) {
	const owner = "churn"
	const other = "other"
	t.Cleanup(func() { clearBfdObservations(owner); clearBfdObservations(other) })
	createBfdObservation(other, "same")
	recordBfdState(other, "same", "down")
	recordBfdState(other, "same", "up")
	otherFlap := BfdLastFlap(other, "same")
	if otherFlap.IsZero() {
		t.Fatal("transition missing")
	}
	for i := 0; i < 2048; i++ {
		key := fmt.Sprint(i)
		createBfdObservation(owner, key)
		recordBfdState(owner, key, "down")
		recordBfdState(owner, key, "up")
		deleteBfdObservation(owner, key)
		recordBfdState(owner, key, "down")
	}
	if len(bfdLiveKeys[owner]) != 0 {
		t.Fatal("historical keys retained")
	}
	count := 0
	bfdObservations.Range(func(key, _ any) bool {
		if len(key.(string)) >= len(owner)+1 && key.(string)[:len(owner)+1] == owner+"\x00" {
			count++
		}
		return true
	})
	if count != 0 {
		t.Fatal("observations retained", count)
	}
	createBfdObservation(owner, "same")
	recordBfdState(owner, "same", "down")
	recordBfdState(owner, "same", "up")
	deleteBfdObservation(owner, "same")
	createBfdObservation(owner, "same")
	recordBfdState(owner, "same", "down")
	if !BfdLastFlap(owner, "same").IsZero() {
		t.Fatal("recreated old flap")
	}
	recordBfdState(owner, "same", "up")
	if BfdLastFlap(owner, "same").IsZero() {
		t.Fatal("real transition missing")
	}
	pruneBfdObservations(owner, nil)
	recordBfdState(owner, "same", "down")
	if !BfdLastFlap(owner, "same").IsZero() {
		t.Fatal("stale event adopted")
	}
	if !otherFlap.Equal(BfdLastFlap(other, "same")) {
		t.Fatal("foreign owner cleared")
	}
	clearBfdObservations(owner)
	pruneBfdObservations(owner, []string{"same"})
	recordBfdState(owner, "same", "up")
	if !BfdLastFlap(owner, "same").IsZero() {
		t.Fatal("restart inherited flap")
	}
}
