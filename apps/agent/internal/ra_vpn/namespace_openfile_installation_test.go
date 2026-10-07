package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ngfw/agent/internal/vpp/bootid"
)

func TestNumericPublisherHeldInstallationRejectsChanges(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned held artifact fixture requires root")
	}
	for _, change := range []string{"replacement", "content", "mode", "hardlink", "symlink"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			p := &numericPublisherInstallationProof{source: (bootid.Reader{}).ForPID(os.Getpid())}
			for i := 0; i < 4; i++ {
				path := filepath.Join(root, string(rune('a'+i)))
				if err := os.WriteFile(path, []byte("authenticated artifact"), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := openNumericPublisherArtifact(path, 1024, false)
				if err != nil {
					t.Fatal(err)
				}
				p.files = append(p.files, a)
			}
			defer func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			}()
			if err := p.Verify(context.Background()); err != nil {
				t.Fatal("unchanged held fixture refused")
			}
			path := p.files[0].path
			switch change {
			case "replacement":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("authenticated artifact"), 0600); err != nil {
					t.Fatal(err)
				}
			case "content":
				if err := os.WriteFile(path, []byte("changed artifact bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0400); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".alias"); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".old", path); err != nil {
					t.Fatal(err)
				}
			}
			if p.Verify(context.Background()) == nil {
				t.Fatal("changed canonical artifact accepted")
			}
		})
	}
}

func TestNumericPublisherStreamingHashAndCancellation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned held artifact fixture requires root")
	}
	path := filepath.Join(t.TempDir(), "artifact")
	data := make([]byte, 700000)
	copy(data, []byte{0x7f, 'E', 'L', 'F'})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := openNumericPublisherArtifact(path, int64(len(data)), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := a.file.Close(); err != nil {
			t.Error(err)
		}
	}()
	digest, header, err := hashNumericPublisherArtifact(context.Background(), a)
	expected := sha256.Sum256(data)
	if err != nil || digest != hex.EncodeToString(expected[:]) || string(header) != string(data[:4]) {
		t.Fatal("whole streamed digest differs")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := hashNumericPublisherArtifact(ctx, a); err == nil {
		t.Fatal("cancelled validation accepted")
	}
}

// Model cancellation racing a successful Err sample without wall-clock timing
// or a detached worker. A nil sample is not authority for later proof success.
type cancelAfterProofSample struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *cancelAfterProofSample) Err() error {
	err := c.Context.Err()
	if c.remaining > 0 {
		c.remaining--
		if c.remaining == 0 {
			c.cancel()
		}
	}
	return err
}

func TestNumericPublisherProofRejectsCancellationAfterSuccessfulSample(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned held artifact fixture requires root")
	}
	root := t.TempDir()
	p := &numericPublisherInstallationProof{source: (bootid.Reader{}).ForPID(os.Getpid())}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	for i := 0; i < 4; i++ {
		path := filepath.Join(root, string(rune('a'+i)))
		if err := os.WriteFile(path, []byte("authenticated artifact"), 0600); err != nil {
			t.Fatal(err)
		}
		artifact, err := openNumericPublisherArtifact(path, 1024, false)
		if err != nil {
			t.Fatal(err)
		}
		p.files = append(p.files, artifact)
	}
	if err := p.Verify(context.Background()); err != nil {
		t.Fatal("unchanged positive control refused", err)
	}
	for _, boundary := range []struct {
		name   string
		sample int
	}{{"artifact-traversal", 2}, {"final-identity-read", 6}} {
		t.Run(boundary.name, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Initial sample, four artifact samples, then the final identity
			// sample. Cancellation at the last sample must survive that read.
			ctx := &cancelAfterProofSample{Context: parent, cancel: cancel, remaining: boundary.sample}
			if err := p.Verify(ctx); err != ErrBoundary || parent.Err() != context.Canceled {
				t.Fatal("cancellation racing a successful sample was accepted", err)
			}
		})
	}
	// Failure must retain caller-owned held files for checked cleanup/reuse.
	if err := p.Verify(context.Background()); err != nil {
		t.Fatal("cancellation altered or closed the held proof", err)
	}
}

func TestNumericPublisherDiagnosticTransportPinned(t *testing.T) {
	if NumericPublisherValidationBudget != 20*time.Second || NumericPublisherIPCBudget != 5*time.Second || NumericPublisherCleanupBudget != 5*time.Second || NumericPublisherServiceRuntimeBudget != 40*time.Second || NumericPublisherWorkBudget != 15*time.Second || NumericPublisherServerWholeBudget != 35*time.Second || NumericOpenFilePublicationBudget != 40*time.Second {
		t.Fatal("publisher phase or whole-call budget changed")
	}
	data, err := os.ReadFile("../../../../deploy/systemd/ngfw-ra-openfile.service")
	if err != nil {
		t.Fatal("publisher shipped template absent")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != numericPublisherServiceDigest {
		t.Fatal("publisher template differs from installation proof")
	}
	text := string(data)
	if strings.Count(text, "StandardOutput=null\n") != 1 || strings.Count(text, "StandardError=journal\n") != 1 || strings.Contains(text, "StandardError=null") {
		t.Fatal("bounded publisher diagnostics cannot reach journal")
	}
	if !strings.Contains(text, "RuntimeMaxSec=40\n") || !strings.Contains(text, "CapabilityBoundingSet=\n") || !strings.Contains(text, "ExecStart=/usr/lib/ngfw/ngfw-ra-namespace-broker --publish-openfile\n") {
		t.Fatal("diagnostic transport changed fixed privilege or runtime contract")
	}
}
