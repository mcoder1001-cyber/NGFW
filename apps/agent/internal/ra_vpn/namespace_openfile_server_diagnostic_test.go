package ravpn

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestNumericPublisherServerStageClosedRange(t *testing.T) {
	for value := 0; value <= 255; value++ {
		stage := NumericPublisherServerStage(value)
		if stage.Valid() != (value >= 1 && value <= 20) {
			t.Fatalf("stage validity %d", value)
		}
	}
}

func TestNumericPublisherServerDiagnosticRedactsManagerInput(t *testing.T) {
	t.Setenv("LISTEN_FDS", "foreign-request-value")
	t.Setenv("LISTEN_FDNAMES", "foreign-role-value")
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	if RunNumericOpenFilePublisher() == nil {
		t.Fatal("invalid manager roles accepted")
	}
	text := output.String()
	if strings.Count(text, "publisher-server entry_stage=1 ") != 1 || strings.Count(text, "\n") != 2 {
		t.Fatal("entry marker missing or unbounded refusal log")
	}
	if !strings.Contains(text, "publisher-server stage=1 deadline_exceeded=false elapsed_ms=") {
		t.Fatal("bounded stage missing")
	}
	if strings.Contains(text, "foreign-request-value") || strings.Contains(text, "foreign-role-value") {
		t.Fatal("untrusted manager input exposed")
	}
}
