package ravpn

import (
	"encoding/json"
	"testing"
)

func TestOwnedXfrmActualHexadecimalReadback(t *testing.T) {
	// Captured from a real disposable Linux namespace using ip -j -d link show.
	raw := []byte(`[{"ifindex":6,"ifname":"xfrm0","flags":["NOARP"],"linkinfo":{"info_kind":"xfrm","info_data":{"if_id":"0x1"}}}]`)
	var links []observedLink
	if err := json.Unmarshal(raw, &links); err != nil {
		t.Fatal("actual existing XFRM link cannot be read during restart", err)
	}
	if len(links) != 1 || links[0].Info.Kind != "xfrm" {
		t.Fatal("actual XFRM shape changed")
	}
	encoded, err := json.Marshal(links[0].Info.Data.ID)
	if err != nil || string(encoded) != `"0x1"` {
		t.Fatal("actual XFRM identifier was not preserved")
	}
}
