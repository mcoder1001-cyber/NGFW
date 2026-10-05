package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

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
