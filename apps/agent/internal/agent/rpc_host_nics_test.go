package agent

import (
	"slices"
	"testing"
)

func TestReasonOnlyNotesDropsPeerAddress(t *testing.T) {
	got := reasonOnlyNotes([]string{
		"ens192 → 0000:0b:00.0 (control connection from 172.30.1.9)",
		"ens192 → 0000:0b:00.0 (control connection from 2001:db8::5)",
		"ens192 → 0000:0b:00.0 (default route)",
	})
	want := []string{
		"ens192 → 0000:0b:00.0 (control connection)",
		"ens192 → 0000:0b:00.0 (control connection)",
		"ens192 → 0000:0b:00.0 (default route)",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}
