package ravpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestBrokerExecutableProofRejectsBoundaryChanges(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires protected root-owned actual files")
	}
	for _, scenario := range []string{"unchanged", "body", "receipt", "replacement", "writable-parent", "cancelled", "closed", "wrong-path"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "broker")
			body := []byte{0x7f, 'E', 'L', 'F', 1, 2, 3, 4}
			digest := sha256.Sum256(body)
			if err := os.WriteFile(path, body, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+".sha256", []byte(hex.EncodeToString(digest[:])+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			proof, err := newBrokerExecutableProof(context.Background(), path)
			if err != nil {
				t.Fatal("fresh complete protected ELF proof", err)
			}
			defer func() { _ = proof.Close() }()
			ctx := context.Background()
			switch scenario {
			case "body":
				if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F', 4, 3, 2, 1}, 0700); err != nil {
					t.Fatal(err)
				}
			case "receipt":
				if err := os.WriteFile(path+".sha256", []byte("foreign\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, body, 0700); err != nil {
					t.Fatal(err)
				}
			case "writable-parent":
				if err := os.Chmod(dir, 0770); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "closed":
				_ = proof.Close()
			case "wrong-path":
				path += ".foreign"
			}
			if valid := proof.Verify(ctx, path) == nil; valid != (scenario == "unchanged") {
				t.Fatalf("proof validity=%v after %s", valid, scenario)
			}
		})
	}
}

func TestBrokerExecutableProofRequiresFullMatchingELFDigest(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires protected root-owned actual files")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "broker")
	body := []byte{0x7f, 'E', 'L', 'F', 1, 2, 3, 4}
	digest := sha256.Sum256(body)
	if err := os.WriteFile(path, body, 0700); err != nil {
		t.Fatal(err)
	}
	body[7]++
	if err := os.WriteFile(path, body, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".sha256", []byte(hex.EncodeToString(digest[:])+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := newBrokerExecutableProof(context.Background(), path); err == nil {
		_ = p.Close()
		t.Fatal("same ELF prefix with altered final byte accepted")
	}
}
