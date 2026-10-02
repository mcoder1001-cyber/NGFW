package basepolicy

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func kernel(names []string) []byte {
	b, _ := json.Marshal(map[string]any{"nftables": []any{map[string]any{"set": map[string]any{"family": "inet", "table": "vrx_base", "name": "punt_interfaces", "type": "ifname", "elem": names}}}})
	return b
}
func TestMembers(t *testing.T) {
	got, err := Members("mgmt0", []string{"tap2"}, []string{"tap1", "tap2"})
	if err != nil || !slices.Equal(got, []string{"tap1", "tap2"}) {
		t.Fatalf("union %v %v", got, err)
	}
	for _, names := range [][]string{{"lo"}, {"mgmt0"}, {"tap;accept"}, {"tap0", "tap0"}, {strings.Repeat("x", 16)}} {
		if _, err := Members("mgmt0", nil, names); err == nil {
			t.Fatalf("accepted %v", names)
		}
	}
	names := make([]string, 64)
	for i := range names {
		names[i] = fmt.Sprintf("tap%d", i)
	}
	if _, err := Members("mgmt0", nil, names); err != nil {
		t.Fatal(err)
	}
	if _, err := Members("mgmt0", []string{"extra"}, names); err == nil {
		t.Fatal("65-member union accepted")
	}
}
func TestParse(t *testing.T) {
	if _, err := Parse(kernel(nil), "mgmt0"); err != nil {
		t.Fatal(err)
	}
	bad := [][]byte{[]byte(`{}`), []byte(`{"nftables":[{"table":{}}]}`), []byte(strings.Repeat("x", MaxJSON+1)), append(kernel(nil), []byte(` {}`)...), kernel([]string{"mgmt0"}), kernel([]string{"tap0", "tap0"})}
	for _, replacement := range []string{"wrong", "inet vrx"} {
		bad = append(bad, []byte(strings.Replace(string(kernel(nil)), "vrx_base", replacement, 1)))
	}
	bad = append(bad, []byte(strings.Replace(string(kernel(nil)), `"ifname"`, `"ipv4_addr"`, 1)), []byte(strings.Replace(string(kernel(nil)), `"type":"ifname"`, `"type":"ifname","flags":["interval"]`, 1)))
	for _, data := range bad {
		if _, err := Parse(data, "mgmt0"); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
