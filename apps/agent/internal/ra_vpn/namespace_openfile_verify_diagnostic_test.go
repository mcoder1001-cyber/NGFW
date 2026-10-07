package ravpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ngfw/agent/internal/vpp/bootid"
)

func TestNumericPublisherProofDiagnosticClosedValues(t *testing.T) {
	for reason := numericPublisherProofReason(0); reason <= 10; reason++ {
		for artifact := uint8(0); artifact <= 5; artifact++ {
			for _, state := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.New("private-context-marker")} {
				indexed := reason == numericPublisherProofHeldStat || reason == numericPublisherProofFreshOpen || reason == numericPublisherProofStamp || reason == numericPublisherProofClose || reason == numericPublisherProofArtifactContext
				valid := reason >= 1 && reason <= 9 && (indexed && artifact >= 1 && artifact <= 4 || !indexed && artifact == 0) && (state == nil || state == context.Canceled || state == context.DeadlineExceeded)
				line := numericPublisherProofFailureLine(reason, artifact, state)
				if !valid {
					if line != "" {
						t.Fatal("invalid reason/index/context entered journal")
					}
					continue
				}
				want := fmt.Sprintf("remote-access publisher-proof reason=%d artifact_index=%d deadline_exceeded=%t canceled=%t", reason, artifact, state == context.DeadlineExceeded, state == context.Canceled)
				if line != want {
					t.Fatal("closed diagnostic fields changed")
				}
			}
		}
	}
	if numericPublisherProofFailureLine(255, 0, nil) != "" || numericPublisherProofFailureLine(numericPublisherProofStamp, 255, nil) != "" {
		t.Fatal("unbounded enum or artifact accepted")
	}
}

func proofDiagnosticFixture(t *testing.T) *numericPublisherInstallationProof {
	t.Helper()
	root := t.TempDir()
	p := &numericPublisherInstallationProof{source: (bootid.Reader{}).ForPID(os.Getpid())}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	for index := 1; index <= 4; index++ {
		directory := filepath.Join(root, fmt.Sprintf("slot-%d-private-installation-marker", index))
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "private-artifact-marker")
		if err := os.WriteFile(path, []byte("authenticated fixture artifact"), 0600); err != nil {
			t.Fatal(err)
		}
		artifact, err := openNumericPublisherArtifact(path, 1024, false)
		if err != nil {
			t.Fatal(err)
		}
		p.files = append(p.files, artifact)
	}
	return p
}

func captureProofDiagnostic(ctx context.Context, p *numericPublisherInstallationProof) (string, error) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	err := p.Verify(ctx)
	return output.String(), err
}

func TestNumericPublisherProofActualFailureAttributionAndSilence(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned held artifact fixture requires root")
	}
	for _, mode := range []string{"healthy", "canceled", "deadline", "artifact-cancel", "exit-cancel", "changed", "inaccessible"} {
		lastSlot := uint8(1)
		if mode == "changed" || mode == "inaccessible" {
			lastSlot = 4
		}
		for slot := uint8(1); slot <= lastSlot; slot++ {
			t.Run(fmt.Sprintf("%s-%d", mode, slot), func(t *testing.T) {
				p := proofDiagnosticFixture(t)
				if line, err := captureProofDiagnostic(context.Background(), p); err != nil || line != "" {
					t.Fatal("healthy proof refused or emitted diagnostics", err)
				}
				parent, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx := parent
				reason, artifact := numericPublisherProofEntry, uint8(0)
				var state error
				switch mode {
				case "canceled":
					cancel()
					state = context.Canceled
				case "deadline":
					var expiredCancel context.CancelFunc
					ctx, expiredCancel = context.WithDeadline(parent, time.Now().Add(-time.Second))
					defer expiredCancel()
					state = context.DeadlineExceeded
				case "artifact-cancel":
					ctx = &cancelAfterProofSample{Context: parent, cancel: cancel, remaining: 2}
					reason, artifact, state = numericPublisherProofArtifactContext, 2, context.Canceled
				case "exit-cancel":
					ctx = &cancelAfterProofSample{Context: parent, cancel: cancel, remaining: 6}
					reason, state = numericPublisherProofExit, context.Canceled
				case "changed":
					if err := os.WriteFile(p.files[slot-1].path, []byte("different-length changed fixture installation bytes"), 0600); err != nil {
						t.Fatal(err)
					}
					reason, artifact = numericPublisherProofStamp, slot
				case "inaccessible":
					// Renaming only the parent leaves the held file stamp intact;
					// the canonical reopen genuinely fails, independently of stamp checks.
					directory := filepath.Dir(p.files[slot-1].path)
					if err := os.Rename(directory, directory+".hidden"); err != nil {
						t.Fatal(err)
					}
					reason, artifact = numericPublisherProofFreshOpen, slot
				}
				output, err := captureProofDiagnostic(ctx, p)
				// Caller cleanup must not turn a live-context failure record into cancellation.
				cancel()
				if mode == "healthy" {
					if err != nil || output != "" {
						t.Fatal("successful proof logged or refused", err)
					}
				} else {
					want := numericPublisherProofFailureLine(reason, artifact, state)
					if err != ErrBoundary || strings.Count(output, "remote-access publisher-proof ") != 1 || !strings.HasSuffix(output, want+"\n") {
						t.Fatal("failure boundary/context attribution incorrect", err, output)
					}
					for _, private := range []string{"private-installation-marker", "private-artifact-marker", p.source.BootID, fmt.Sprintf("pid=%d", p.source.PID), "no such file", "permission denied"} {
						if strings.Contains(output, private) {
							t.Fatal("installation or process diagnostic exposed")
						}
					}
				}
				for _, held := range p.files {
					if _, err := held.file.Stat(); err != nil {
						t.Fatal("Verify closed caller-owned held descriptor", err)
					}
				}
			})
		}
	}
}
