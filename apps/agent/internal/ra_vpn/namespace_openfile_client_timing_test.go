package ravpn

import (
	"strings"
	"testing"
	"time"
)

func TestNumericPublisherClientTimingHealthySilenceAndUnsetPhases(t *testing.T) {
	trace := newNumericPublisherClientTiming()
	if lines := trace.failureLines(false, false); lines != nil {
		t.Fatal("healthy call emitted diagnostics")
	}
	lines := trace.failureLines(true, true)
	if len(lines) != 3 || !strings.Contains(lines[0], "phase=preflight caller_deadline_exceeded=true enter_ms=0 proof_done_ms=-1 peer_reference_done_ms=-1 manager_done_ms=-1") {
		t.Fatal("fixed preflight failure representation changed")
	}
	for index, phase := range []string{"probe", "publish"} {
		expected := "remote-access publisher-client phase=" + phase + " before_connect_ms=-1 after_connect_ms=-1 ready_received_ms=-1 ready_proof_done_ms=-1 request_sent_ms=-1"
		if lines[index+1] != expected {
			t.Fatal("unvisited exchange fabricated a milestone")
		}
	}
}

func TestNumericPublisherClientTimingMonotonicAndPhaseIsolated(t *testing.T) {
	trace := newNumericPublisherClientTiming()
	trace.markPreflight(1)
	trace.markPreflight(2)
	trace.markPreflight(3)
	for i := 0; i < 5; i++ {
		trace.markExchange(0, i)
	}
	time.Sleep(2 * time.Millisecond)
	trace.markExchange(1, 0)
	for i := 1; i < len(trace.preflight); i++ {
		if trace.preflight[i] < trace.preflight[i-1] {
			t.Fatal("preflight clock regressed")
		}
	}
	for i := 1; i < 5; i++ {
		if trace.exchange[0][i] < trace.exchange[0][i-1] {
			t.Fatal("exchange clock regressed")
		}
	}
	if trace.exchange[1][0] < trace.exchange[0][4] || trace.exchange[1][1] != -1 || trace.exchange[1][4] != -1 {
		t.Fatal("phases contaminated or fabricated")
	}
	before := trace.preflight
	trace.markPreflight(-1)
	trace.markPreflight(4)
	trace.markExchange(-1, 0)
	trace.markExchange(2, 0)
	trace.markExchange(0, -1)
	trace.markExchange(0, 5)
	if trace.preflight != before {
		t.Fatal("foreign milestone accepted")
	}
}

func TestNumericPublisherClientTimingClosedBoundsAndLabels(t *testing.T) {
	trace := newNumericPublisherClientTiming()
	trace.started = time.Now().Add(-24 * time.Hour)
	trace.markPreflight(1)
	trace.markExchange(0, 0)
	if trace.preflight[1] != NumericOpenFilePublicationBudget.Milliseconds() || trace.exchange[0][0] != NumericOpenFilePublicationBudget.Milliseconds() {
		t.Fatal("elapsed values unbounded")
	}
	lines := trace.failureLines(true, false)
	permitted := "abcdefghijklmnopqrstuvwxyz0123456789 -_=%"
	for _, line := range lines {
		if len(line) > 300 || strings.Contains(line, "/") || strings.Contains(line, "@") {
			t.Fatal("diagnostic exposes unbounded or identity-like text")
		}
		for _, value := range line {
			if !strings.ContainsRune(permitted, value) {
				t.Fatal("diagnostic contains a nonclosed character")
			}
		}
	}
	trace.started = time.Now().Add(time.Hour)
	if trace.elapsed() != 0 {
		t.Fatal("future start produced negative recorded time")
	}
}
