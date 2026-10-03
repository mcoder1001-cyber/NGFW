package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunWritesStateFile(t *testing.T) {
	dir := t.TempDir()
	nowFunc = func() time.Time { return time.Date(2026, 9, 24, 1, 2, 3, 4, time.UTC) }
	if err := run([]string{dir, "INSTANCE", "vi1", "MASTER", "150"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "vi1.state")) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil || rec != (Record{Name: "vi1", Type: "INSTANCE", State: "MASTER", Time: "2026-09-24T01:02:03.000000004Z"}) {
		t.Fatalf("%s %v", b, err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("temp files left: %v", ents)
	}
}

func TestRunRejectsHostileArguments(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{dir, "INSTANCE", "../x", "MASTER"},
		{dir, "INSTANCE", "vi1", "MASTER; rm -rf /"},
		{dir, "INSTANCE", "vi 1", "MASTER"},
		{dir + "/../etc", "INSTANCE", "vi1", "MASTER"},
		{"relative", "INSTANCE", "vi1", "MASTER"},
		{dir, "SCRIPT", "vi1", "MASTER"},
		{dir, "INSTANCE", "vi1"},
		{dir + "/missing", "INSTANCE", "vi1", "BACKUP"},
	} {
		if err := run(args); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("files written: %v", ents)
	}
}
