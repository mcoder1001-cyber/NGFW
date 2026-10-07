package ravpn

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestUnitObserverPinsActualShippedDaemonTemplate(t *testing.T) {
	data, err := os.ReadFile("../../../../deploy/systemd/ngfw-ra@.service")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != unitObserverRAServiceDigest {
		t.Fatal("observer rejects actual shipped private daemon template")
	}
	if unitObserverRAServiceDigest == "bf5e89b553235d455db222039d5e8b2cdbe639a1d36054b5dab6cca3ea266d9a" {
		t.Fatal("historical broad-visibility daemon template still accepted")
	}
	data = append(data, []byte("\n[Service]\nCapabilityBoundingSet=CAP_SYS_ADMIN\n")...)
	changed := sha256.Sum256(data)
	if hex.EncodeToString(changed[:]) == unitObserverRAServiceDigest {
		t.Fatal("altered daemon privilege template accepted")
	}
}
