package ravpn

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceCapabilityNormalizationBounds(t *testing.T) {
	status := func(inherited, ambient, effective uint64) []byte {
		return []byte(fmt.Sprintf("Uid:\t0\t0\t0\t0\nNoNewPrivs:\t1\nCapEff:\t%x\nCapPrm:\t%x\nCapBnd:\t%x\nCapInh:\t%x\nCapAmb:\t%x\n", effective, canonicalSourceCapabilities, canonicalSourceCapabilities, inherited, ambient))
	}
	before := status(1<<21, 0, canonicalSourceCapabilities)
	if sourceCapabilityStatus(before, false) != nil {
		t.Fatal("refused observed systemd source state")
	}
	if sourceCapabilityStatus(before, true) == nil {
		t.Fatal("accepted nonzero inheritable normalized state")
	}
	after := status(0, 0, canonicalSourceCapabilities)
	if sourceCapabilityStatus(after, true) != nil {
		t.Fatal("refused exact normalized state")
	}
	for _, bad := range [][]byte{status(1<<12, 0, canonicalSourceCapabilities), status(0, 1, canonicalSourceCapabilities), status(0, 0, canonicalSourceCapabilities|1<<unix.CAP_NET_RAW), append(after, []byte("CapInh:\t0\n")...)} {
		if sourceCapabilityStatus(bad, false) == nil {
			t.Fatal("accepted widened or ambiguous source capability state")
		}
	}
}

func TestBrokerCapabilityNormalizationRetainsSeparateBoundary(t *testing.T) {
	status := []byte(fmt.Sprintf("Uid:\t0\t0\t0\t0\nNoNewPrivs:\t1\nCapEff:\t%x\nCapPrm:\t%x\nCapBnd:\t%x\nCapInh:\t%x\nCapAmb:\t0\n", canonicalBrokerCapabilities, canonicalBrokerCapabilities, canonicalBrokerCapabilities, uint64(1<<21)))
	if processCapabilityStatus(status, canonicalBrokerCapabilities, false) != nil {
		t.Fatal("refused broker bounded inherited mount setup capability")
	}
	if sourceCapabilityStatus(status, false) == nil || processCapabilityStatus(status, canonicalBrokerCapabilities, true) == nil {
		t.Fatal("broker state adopted as source or normalized state")
	}
	zero := []byte("Uid: 0 0 0 0\nNoNewPrivs: 1\nCapEff: 0\nCapPrm: 0\nCapBnd: 0\nCapInh: 200000\nCapAmb: 0\n")
	if processCapabilityStatus(zero, 0, false) == nil {
		t.Fatal("inheritable mount privilege accepted outside permitted broker/source boundary")
	}
}

func TestCanonicalSourceCapabilityMasksMatchPackagedUnits(t *testing.T) {
	bits := map[string]uint{"CAP_NET_ADMIN": unix.CAP_NET_ADMIN, "CAP_SYS_ADMIN": unix.CAP_SYS_ADMIN,
		"CAP_IPC_LOCK": unix.CAP_IPC_LOCK, "CAP_CHOWN": unix.CAP_CHOWN, "CAP_DAC_OVERRIDE": unix.CAP_DAC_OVERRIDE}
	for _, source := range []struct{ path, digest string }{
		{"systemd/ngfw-agent.service", expectedAgentUnitSHA256},
		{"hardening/systemd/ngfw-agent.service.d/10-ngfw-hardening.conf", expectedAgentHardeningSHA256},
	} {
		t.Run(source.path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "deploy", source.path))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if hex.EncodeToString(hash[:]) != source.digest {
				t.Fatal("agent fragment changed without exact RA attestation update")
			}
			var mask uint64
			found := false
			for _, line := range strings.Split(string(data), "\n") {
				if !strings.HasPrefix(line, "CapabilityBoundingSet=") {
					continue
				}
				found = true
				names := strings.Fields(strings.TrimPrefix(line, "CapabilityBoundingSet="))
				if len(names) == 0 {
					mask = 0
					continue
				}
				for _, name := range names {
					bit, ok := bits[name]
					if !ok {
						t.Fatalf("unreviewed agent capability %s", name)
					}
					mask |= uint64(1) << bit
				}
			}
			if !found || mask != canonicalSourceCapabilities {
				t.Fatalf("packaged=%x source=%x", mask, canonicalSourceCapabilities)
			}
			for _, field := range []string{"CapEff", "CapPrm", "CapBnd"} {
				for bit := uint(0); bit < 64; bit++ {
					status := fmt.Sprintf("Uid: 0 0 0 0\nNoNewPrivs: 1\nCapEff: %x\nCapPrm: %x\nCapBnd: %x\nCapInh: 0\nCapAmb: 0\n", mask, mask, mask)
					if sourceCapabilityStatus([]byte(status), true) != nil {
						t.Fatal("exact packaged mask rejected")
					}
					changed := strings.Replace(status, fmt.Sprintf("%s: %x", field, mask), fmt.Sprintf("%s: %x", field, mask^(uint64(1)<<bit)), 1)
					if sourceCapabilityStatus([]byte(changed), true) == nil {
						t.Fatalf("accepted missing/extra bit%d in %s", bit, field)
					}
				}
			}
		})
	}
	if canonicalBrokerCapabilities != uint64(1<<unix.CAP_SYS_ADMIN|1<<unix.CAP_SYS_CHROOT) {
		t.Fatal("separate broker boundary changed")
	}
	status := []byte(fmt.Sprintf("Uid: 0 0 0 0\nNoNewPrivs: 1\nCapEff: %x\nCapPrm: %x\nCapBnd: %x\nCapInh: 0\nCapAmb: 0\n", canonicalSourceCapabilities, canonicalSourceCapabilities, canonicalSourceCapabilities))
	if processCapabilityStatus(status, canonicalBrokerCapabilities, true) == nil {
		t.Fatal("source privileges admitted to broker")
	}
}
