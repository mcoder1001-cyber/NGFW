package ravpn

import (
	"encoding/json"
	"testing"
)

func TestOwnedXfrmInterfaceIDRemainsExactlyOne(t *testing.T) {
	for _, raw := range []string{"1", `"0x1"`, "0", "2", "1.0", "-1", `"0x2"`, `"0x01"`, `"1"`, `null`, `"0x1; unexpected"`} {
		if got := ownedXfrmInterfaceID(json.RawMessage(raw)); got != (raw == "1" || raw == `"0x1"`) {
			t.Errorf("ID=%s accepted=%v", raw, got)
		}
	}
}
