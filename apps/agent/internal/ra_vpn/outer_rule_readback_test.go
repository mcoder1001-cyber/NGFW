package ravpn

import "testing"

func TestOwnedOuterRuleRealKernelJSON(t *testing.T) {
	// Actual iproute2 JSON after: ip -4 rule add priority100 fwmark1 lookup100.
	data := []byte(`[{"priority":0,"src":"all","table":"local"},{"priority":100,"src":"all","fwmark":"0x1","table":"100"},{"priority":32766,"src":"all","table":"main"},{"priority":32767,"src":"all","table":"default"}]`)
	if add, err := ownedOuterRule(data); err != nil || add {
		t.Fatalf("actual owned kernel rule refused: add=%v err=%v", add, err)
	}
}

func TestOwnedOuterRuleRefusesOtherStringTables(t *testing.T) {
	for _, table := range []string{`"101"`, `"0100"`, `"0x64"`, `"main"`, `null`, `100.0`, `"100; foreign"`} {
		data := []byte(`[{"priority":100,"src":"all","fwmark":"0x1","table":` + table + `}]`)
		if _, err := ownedOuterRule(data); err == nil {
			t.Fatalf("foreign table %s accepted", table)
		}
	}
}
