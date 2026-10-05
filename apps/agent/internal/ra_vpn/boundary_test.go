package ravpn

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestHelperCapabilitiesRejectHostAdministration(t *testing.T) {
	status := func(mask uint64) string {
		return fmt.Sprintf("CapEff:\t%016x\nCapPrm:\t%016x\nCapBnd:\t%016x\nCapInh:\t0\nCapAmb:\t0\n", mask, mask, mask)
	}
	if err := ValidateHelperCapabilities(status(0x1400)); err != nil {
		t.Fatal(err)
	}
	for _, mask := range []uint64{0, 0x1000, 0x1400 | 1<<21, 0xffffffffffffffff} {
		if ValidateHelperCapabilities(status(mask)) == nil {
			t.Fatalf("unsafe capabilities accepted: %x", mask)
		}
	}
	if ValidateHelperCapabilities(status(0x1400)+"CapBnd:\t0\n") == nil {
		t.Fatal("duplicate capability records accepted")
	}
}

func TestPrivatePlanStrictIdentityAndBounds(t *testing.T) {
	plan := networkFixture()
	plan.NamespaceInode = 1234
	data, _ := json.Marshal(plan)
	if _, err := DecodePrivatePlan(data, plan.Instance); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		data     []byte
		instance string
	}{
		{"foreign", data, InstanceID("foreign", "road")},
		{"trailing", append(append([]byte{}, data...), []byte(` {}`)...), plan.Instance},
		{"unknown", append([]byte(`{"command":"sh",`), data[1:]...), plan.Instance},
		{"path", data, "../../etc"},
		{"oversized", make([]byte, 16385), plan.Instance},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodePrivatePlan(test.data, test.instance); err == nil {
				t.Fatal("unsafe helper manifest accepted")
			}
		})
	}
	plan.NamespaceInode = 0
	data, _ = json.Marshal(plan)
	if _, err := DecodePrivatePlan(data, plan.Instance); err == nil {
		t.Fatal("unbound namespace accepted")
	}
}

func TestPrivilegedReaderRefusesUnownedOrAbsentInstance(t *testing.T) {
	// This does not create or enter a namespace or modify the shared /run tree.
	if _, err := ReadPrivatePlan("../foreign"); err == nil {
		t.Fatal("unsafe path accepted")
	}
	if _, err := ReadPrivatePlan(InstanceID("missing-test-owner", "never-created")); err == nil {
		t.Fatal("missing runtime accepted")
	}
}
