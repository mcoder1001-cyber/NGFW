package ravpn

import (
	"bytes"
	"ngfw/agent/internal/vpp/bootid"
	"testing"
)

func TestTargetsOpenFileExactNumericRecord(t *testing.T) {
	target := bootid.Identity{BootID: "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", PID: 4242, StartTime: 987654}
	data, err := RenderTargetsOpenFile(target)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseTargetsOpenFile(data)
	if err != nil || !parsed.Equal(target) {
		t.Fatal("failed exact roundtrip")
	}
	if bytes.Contains(data, []byte("%i")) || !bytes.Contains(data, []byte("/proc/4242/ns/mnt")) {
		t.Fatal("missing literal numeric path")
	}
	for _, bad := range [][]byte{append(append([]byte(nil), data...), []byte("ExecStart=/bin/false\n")...), bytes.Replace(data, []byte("pid=4242"), []byte("pid=04242"), 1), bytes.Replace(data, []byte("vpp-mount"), []byte("foreign"), 1)} {
		if _, err := ParseTargetsOpenFile(bad); err == nil {
			t.Fatal("accepted modified configuration")
		}
	}
	target.BootID = "bad\n[Service]"
	if _, err := RenderTargetsOpenFile(target); err == nil {
		t.Fatal("accepted injected boot header")
	}
	if _, err := TargetsOpenFilePath(-1); err == nil {
		t.Fatal("accepted negative PID")
	}
}
