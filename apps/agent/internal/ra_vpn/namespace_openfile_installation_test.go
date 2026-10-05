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

func TestNumericPublisherDiagnosticTransportPinned(t *testing.T) {
	if NumericPublisherValidationBudget != 20*time.Second || NumericPublisherIPCBudget != 5*time.Second || NumericPublisherCleanupBudget != 5*time.Second || NumericPublisherServiceRuntimeBudget != 30*time.Second || NumericOpenFilePublicationBudget != 40*time.Second {
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
	if !strings.Contains(text, "RuntimeMaxSec=30\n") || !strings.Contains(text, "CapabilityBoundingSet=\n") || !strings.Contains(text, "ExecStart=/usr/lib/ngfw/ngfw-ra-namespace-broker --publish-openfile\n") {
		t.Fatal("diagnostic transport changed fixed privilege or runtime contract")
	}
}
