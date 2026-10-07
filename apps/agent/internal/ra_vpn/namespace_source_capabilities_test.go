package ravpn

import (
	"fmt"
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
	for _, bad := range [][]byte{status(1<<12, 0, canonicalSourceCapabilities), status(0, 1, canonicalSourceCapabilities), status(0, 0, canonicalSourceCapabilities|1), append(after, []byte("CapInh:\t0\n")...)} {
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
