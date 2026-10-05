package ravpn

import (
	"context"
	"errors"
	"testing"
)

func TestNamespaceReadbackRejectsForeignOwnership(t *testing.T) {
	instance := InstanceID("w19", "road")
	owned := `{"nftables":[{"table":{"family":"inet","name":"ngfw_ra","comment":"ngfw-ra:` + instance + `"}}]}`
	if replace, err := ownedFirewall([]byte(owned), instance); err != nil || !replace {
		t.Fatal("owned table refused")
	}
	for _, data := range []string{`not json`, `{"nftables":[{"table":{"family":"inet","name":"ngfw_ra"}}]}`, `{"nftables":[{"table":{"family":"inet","name":"foreign"}}]}`} {
		if _, err := ownedFirewall([]byte(data), instance); err == nil {
			t.Fatal("foreign table accepted")
		}
	}
	if add, err := ownedOuterRule([]byte(`[{"priority":100,"fwmark":"0x1","table":100}]`)); err != nil || add {
		t.Fatal("owned rule refused")
	}
	for _, data := range []string{`[{"priority":100,"fwmark":"0x2","table":100}]`, `[{"priority":100,"fwmark":"0x1","table":101}]`, `[{"priority":100,"fwmark":"0x1","table":100},{"priority":100,"fwmark":"0x1","table":100}]`} {
		if _, err := ownedOuterRule([]byte(data)); err == nil {
			t.Fatal("foreign or duplicate rule accepted")
		}
	}
}

func TestNamespaceCommandRefusesUnlistedExecutable(t *testing.T) {
	if _, err := command(context.Background(), "/bin/true", nil); !errors.Is(err, ErrBoundary) {
		t.Fatal("namespace command accepted a tool outside its fixed allowlist")
	}
}
