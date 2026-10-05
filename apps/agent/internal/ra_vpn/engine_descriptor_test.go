package ravpn

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentEngineRecordPhasesAndLinkedForeignRecords(t *testing.T) {
	dir := t.TempDir()
	store, e := NewFileEngineStore(dir, "w19")
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	_, _, _, units, _, _, spec := lifecycleFixture(t)
	record := EngineRecord{Spec: spec, Unit: units.id}
	if store.Save(record) != nil {
		t.Fatal("pending save")
	}
	record.Ready = true
	if store.Save(record) != nil {
		t.Fatal("ready transition")
	}
	records, e := store.List()
	if e != nil || len(records) != 1 || !records[0].Ready || records[0].Unit != units.id {
		t.Fatal("recovery", e)
	}
	name := filepath.Join(dir, "ra-engine", spec.Instance+".json")
	linked := filepath.Join(dir, "foreign.json")
	if os.Link(name, linked) != nil {
		t.Fatal("link fixture")
	}
	if _, e := store.List(); e == nil {
		t.Fatal("linked ownership record trusted")
	}
	if store.Remove(spec.Instance) == nil {
		t.Fatal("linked record removed")
	}
	if _, e := os.Stat(linked); e != nil {
		t.Fatal("foreign link deleted")
	}
	if os.Remove(linked) != nil {
		t.Fatal("remove owned fixture link")
	}
	if store.Remove(spec.Instance) != nil {
		t.Fatal("safe removal")
	}
}
func TestPersistentEngineRecordRefusesSymlinkStateParent(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "actual")
	if os.Mkdir(actual, 0700) != nil {
		t.Fatal("fixture")
	}
	link := filepath.Join(dir, "link")
	if os.Symlink(actual, link) != nil {
		t.Fatal("fixture")
	}
	if s, e := NewFileEngineStore(link, "w19"); e == nil {
		s.Close()
		t.Fatal("symlink parent trusted")
	}
}
func TestFailedStoreInventoryStillStopsKnownOwnedGeneration(t *testing.T) {
	r, _, _, units, store, events, spec := lifecycleFixture(t)
	if _, e := r.Create(context.Background(), spec); e != nil {
		t.Fatal(e)
	}
	units.stopFail = true
	if r.StopAll(context.Background()) == nil {
		t.Fatal("failed stop ignored")
	}
	if len(store.records) != 1 || len(r.active) != 1 {
		t.Fatal("owned failure forgotten")
	}
	units.stopFail = false
	if r.StopAll(context.Background()) != nil || len(r.active) != 0 || len(*events) == 0 {
		t.Fatal("retry failed")
	}
}
