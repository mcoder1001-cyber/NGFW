package ravpn

import "testing"

func TestNumericPublisherNeverStartedCgroup(t *testing.T) {
	fresh := map[string]string{"ControlGroup": "", "MainPID": "0", "ControlPID": "0", "ActiveState": "inactive", "SubState": "dead"}
	if !numericPublisherCgroup(fresh, false) {
		t.Fatal("refused actual never-started inactive manager state")
	}
	if numericPublisherCgroup(fresh, true) {
		t.Fatal("accepted empty cgroup for attested live server")
	}
	for key, value := range map[string]string{"MainPID": "123", "ControlPID": "456", "ActiveState": "active", "SubState": "running", "ControlGroup": "/system.slice/foreign.service"} {
		bad := make(map[string]string)
		for k, v := range fresh {
			bad[k] = v
		}
		bad[key] = value
		if numericPublisherCgroup(bad, false) {
			t.Fatalf("accepted missing cgroup with %s=%s", key, value)
		}
	}
	fresh["ControlGroup"] = "/system.slice/ngfw-ra-openfile.service"
	if !numericPublisherCgroup(fresh, true) {
		t.Fatal("refused fixed canonical cgroup")
	}
}
