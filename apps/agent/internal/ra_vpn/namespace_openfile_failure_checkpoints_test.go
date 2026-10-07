package ravpn

import (
	"bytes"
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNumericPublisherFailureCheckpointsClosedFieldsAndContext(t *testing.T) {
	deadline, stopDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stopDeadline()
	canceled, stopCanceled := context.WithCancel(context.Background())
	stopCanceled()
	for _, test := range []struct {
		ctx  context.Context
		want string
	}{
		{context.Background(), "deadline_exceeded=false canceled=false"},
		{deadline, "deadline_exceeded=true canceled=false"},
		{canceled, "deadline_exceeded=false canceled=true"},
	} {
		for _, kind := range []uint8{1, 2} {
			line := numericPublisherCheckpointLine(test.ctx, kind, 2, 27*time.Millisecond, true)
			if !strings.Contains(line, test.want) || !strings.HasSuffix(line, "elapsed_ms=27") {
				t.Fatal("actual context flags missing", line)
			}
			if !regexp.MustCompile(`^remote-access publisher-(manager-query checkpoint|server checkpoint16)=2 deadline_exceeded=(true|false) canceled=(true|false) elapsed_ms=27$`).MatchString(line) {
				t.Fatal("nonclosed diagnostic fields", line)
			}
		}
	}
	for _, kind := range []uint8{1, 2} {
		if line := numericPublisherCheckpointLine(context.Background(), kind, 1, time.Hour, true); !strings.HasSuffix(line, "elapsed_ms=40000") {
			t.Fatal("elapsed upper bound missing")
		}
		if line := numericPublisherCheckpointLine(context.Background(), kind, 1, -time.Second, true); !strings.HasSuffix(line, "elapsed_ms=0") {
			t.Fatal("elapsed lower bound missing")
		}
		if numericPublisherCheckpointLine(context.Background(), kind, 1, time.Second, false) != "" {
			t.Fatal("healthy path emitted diagnostic")
		}
	}
	for _, test := range []struct{ kind, checkpoint uint8 }{{0, 1}, {3, 1}, {1, 0}, {1, 11}, {2, 0}, {2, 19}, {255, 255}} {
		if numericPublisherCheckpointLine(context.Background(), test.kind, test.checkpoint, time.Second, true) != "" {
			t.Fatal("unknown diagnostic enum accepted")
		}
	}
}

type numericPublisherCheckpointCapture struct {
	sync.Mutex
	bytes.Buffer
}

func (capture *numericPublisherCheckpointCapture) Write(data []byte) (int, error) {
	capture.Lock()
	defer capture.Unlock()
	return capture.Buffer.Write(data)
}
func (capture *numericPublisherCheckpointCapture) text() string {
	capture.Lock()
	defer capture.Unlock()
	return capture.String()
}

func TestNumericPublisherFailureCheckpointActualCanceledQueryDoesNotReachPID1(t *testing.T) {
	var output numericPublisherCheckpointCapture
	prior := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(prior)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := readManagerDBusRoles(ctx, managerDBusPublisherRoles)
	if err != ErrBoundary || result != nil {
		t.Fatal("canceled native query changed failure classification")
	}
	text := output.text()
	if !strings.Contains(text, "publisher-manager-query checkpoint=1 deadline_exceeded=false canceled=true elapsed_ms=") {
		t.Fatal("actual query failure missing closed context checkpoint", text)
	}
	if strings.Contains(text, "ngfw-ra-openfile") || strings.Contains(text, "/run/") || strings.Contains(text, "context canceled") {
		t.Fatal("query diagnostic exposed unit/path/error")
	}
}

func TestNumericPublisherFailureCheckpointCleanupDoesNotInventCancellation(t *testing.T) {
	for _, mode := range []string{"live", "deadline", "canceled"} {
		parent := context.Background()
		if mode == "deadline" {
			expired, stop := context.WithDeadline(parent, time.Now().Add(-time.Second))
			defer stop()
			parent = expired
		}
		ctx, cancel := context.WithCancel(parent)
		if mode == "canceled" {
			cancel()
		}
		observed := ""
		func() {
			var state numericPublisherDiagnosticState
			defer func() { observed = state.line(2, 2, time.Millisecond, true) }()
			defer func() { state.captureBeforeCancel(ctx, true); cancel() }()
		}()
		if ctx.Err() == nil {
			t.Fatal("original cleanup cancel did not run")
		}
		wanted := "deadline_exceeded=false canceled=false"
		if mode == "deadline" {
			wanted = "deadline_exceeded=true canceled=false"
		}
		if mode == "canceled" {
			wanted = "deadline_exceeded=false canceled=true"
		}
		if !strings.Contains(observed, wanted) {
			t.Fatal("cleanup cancellation masqueraded as failure cause", mode, observed)
		}
	}
}
