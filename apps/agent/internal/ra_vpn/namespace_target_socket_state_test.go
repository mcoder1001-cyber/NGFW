package ravpn

import "testing"

func TestTargetSupplierSocketLifecycleProperties(t *testing.T) {
	path := "/run/ngfw-ra-ipc/targets/17.sock"
	fields := map[string]string{"FragmentPath": targetSupplierSocket, "DropInPaths": "", "ActiveState": "active", "SubState": "listening", "Listen": path + " (SequentialPacket)"}
	if !targetSupplierSocketProperties(fields, path, false) || targetSupplierSocketProperties(fields, path, true) {
		t.Fatal("idle socket must require a listening supplier")
	}
	fields["SubState"] = "running"
	if !targetSupplierSocketProperties(fields, path, true) || !targetSupplierSocketProperties(fields, path, false) {
		t.Fatal("pre-connect observation must permit the preceding supplier to finish")
	}
	for _, field := range []string{"FragmentPath", "DropInPaths", "ActiveState", "SubState", "Listen"} {
		original := fields[field]
		fields[field] = "foreign"
		if targetSupplierSocketProperties(fields, path, true) || targetSupplierSocketProperties(fields, path, false) {
			t.Fatal("accepted altered socket property", field)
		}
		fields[field] = original
	}
}
