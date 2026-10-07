package bfd

import (
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"testing"
)

func TestOwnedSessionLifecycleCallbacks(t *testing.T) {
	f, _, sessions := fakeBFD()
	var created, deleted string
	var live []string
	SetMultihopEnvironment(df7test.Owner, &MultihopEnvironment{
		Created: func(key string) { created = key }, Deleted: func(key string) { deleted = key }, Snapshot: func(keys []string) { live = keys },
	})
	t.Cleanup(func() { SetMultihopEnvironment(df7test.Owner, nil) })
	d := NewSession(f, df7test.Owner)
	s := Session{Interface: "loop0", Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3}
	key := string(KeySession(s.Interface, s.Local, s.Peer))
	meta, err := d.Create(t.Context(), df7.Encode(s))
	if err != nil {
		t.Fatal(err)
	}
	if created != key {
		t.Fatal("create lifecycle missing")
	}
	if _, err := d.Retrieve(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || live[0] != key {
		t.Fatal("owned snapshot missing", live)
	}
	if err := d.Delete(t.Context(), df7.Encode(s), meta); err != nil {
		t.Fatal(err)
	}
	if deleted != key || len(sessions) != 0 {
		t.Fatal("delete lifecycle missing")
	}
	if _, err := d.Retrieve(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Fatal("deleted snapshot retained")
	}
}
