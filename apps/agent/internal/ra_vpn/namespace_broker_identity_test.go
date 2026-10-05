package ravpn

import "testing"

func TestCanonicalBrokerUnitRefusesForeignAndInjectedCgroups(t *testing.T) {
	valid := "ngfw-ra-namespace-broker@0-123-uid0.service"
	got, err := canonicalBrokerUnit([]byte("0::/system.slice/" + valid + "\n"))
	if err != nil || got != valid {
		t.Fatal("refused canonical accepted socket service")
	}
	nested := "/system.slice/system-ngfw\\x2dra\\x2dnamespace\\x2dbroker.slice/" + valid
	if got, err := canonicalBrokerUnit([]byte("0::" + nested)); err != nil || got != valid || !canonicalBrokerControlGroup(valid, nested) {
		t.Fatal("refused exact templated broker slice")
	}
	if canonicalBrokerControlGroup(valid, "/system.slice/foreign.slice/"+valid) {
		t.Fatal("accepted foreign slice")
	}
	for _, bad := range []string{"0::/system.slice/foreign.service", "0::/user.slice/" + valid, "0::/system.slice/ngfw-ra-namespace-broker@.service", "0::/system.slice/ngfw-ra-namespace-broker@..service", "0::/system.slice/ngfw-ra-namespace-broker@../foreign.service", "0::/system.slice/" + valid + "\n0::/foreign", "0::/system.slice/ngfw-ra-namespace-broker@x;start.service"} {
		if _, err := canonicalBrokerUnit([]byte(bad)); err == nil {
			t.Fatal("accepted foreign or injected unit")
		}
	}
}
