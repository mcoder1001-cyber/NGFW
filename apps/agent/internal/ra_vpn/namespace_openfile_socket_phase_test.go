package ravpn

import "testing"

func TestNumericPublisherSocketStateBindsActivationToCapturedIdentity(t *testing.T) {
	base := map[string]string{"FragmentPath": numericPublisherSocket, "DropInPaths": "", "ActiveState": "active", "SubState": "listening", "Listen": numericPublisherSocketPath + " (SequentialPacket)"}
	if !numericPublisherSocketState(base, false) || !numericPublisherSocketState(base, true) {
		t.Fatal("fixed listening socket refused")
	}
	base["SubState"] = "running"
	if numericPublisherSocketState(base, false) || !numericPublisherSocketState(base, true) {
		t.Fatal("running socket must require captured complete identity")
	}
	for field, value := range map[string]string{"FragmentPath": "/tmp/foreign.socket", "DropInPaths": "/tmp/foreign.conf", "ActiveState": "inactive", "Listen": "/tmp/foreign.sock (SequentialPacket)"} {
		changed := make(map[string]string, len(base))
		for name, original := range base {
			changed[name] = original
		}
		changed[field] = value
		if numericPublisherSocketState(changed, true) {
			t.Fatal("foreign or inactive metadata accepted", field)
		}
	}
	for _, state := range []string{"", "dead", "failed", "deferred", "start-post", "stop-pre", "running-other"} {
		base["SubState"] = state
		if numericPublisherSocketState(base, true) || numericPublisherSocketState(base, false) {
			t.Fatal("unsupported state accepted", state)
		}
	}
}
