package ravpn

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ngfw/agent/internal/vpp/bootid"
)

func TestObserverOpenFileRefusesUnboundedOrInjectedIdentity(t *testing.T) {
	instance := strings.Repeat("a", 64)
	identity := bootid.Identity{BootID: "12345678-1234-1234-1234-123456789abc", PID: 123, StartTime: math.MaxUint64}
	content, err := RenderObserverOpenFile(instance, identity)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "%i") || !strings.Contains(string(content), "OpenFile=/proc/123/ns/net:unit-net:read-only") {
		t.Fatal("numeric role not literal")
	}
	parsedInstance, parsed, err := ParseObserverOpenFile(content)
	if err != nil || parsedInstance != instance || !parsed.Equal(identity) {
		t.Fatal("owned full identity not preserved")
	}
	if ValidateObserverOpenFile(strings.Repeat("b", 64), identity, content) == nil {
		t.Fatal("foreign instance accepted")
	}
	other := identity
	other.StartTime--
	if ValidateObserverOpenFile(instance, other, content) == nil {
		t.Fatal("foreign generation accepted")
	}
	for _, alter := range []func(*bootid.Identity){
		func(v *bootid.Identity) { v.BootID += "\nOpenFile=/etc/shadow" },
		func(v *bootid.Identity) { v.PID = 1 },
		func(v *bootid.Identity) { v.PID = -1 },
		func(v *bootid.Identity) { v.StartTime = 0 },
		func(v *bootid.Identity) { v.BootID = "" },
	} {
		bad := identity
		alter(&bad)
		if _, err := RenderObserverOpenFile(instance, bad); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
	if _, err := RenderObserverOpenFile("../../foreign", identity); err == nil {
		t.Fatal("caller path accepted")
	}
	for _, bad := range []string{
		"", string(content) + "CapabilityBoundingSet=CAP_SYS_ADMIN\n",
		strings.Replace(string(content), "pid=123", "pid=0123", 1),
		strings.Replace(string(content), "[Service]", "[Service]\nExecStart=/bin/sh", 1),
		strings.Replace(string(content), "/proc/123/exe", "/proc/124/exe", 1),
		strings.Replace(string(content), "unit-exe:read-only", "unit-net:read-only", 1),
		strings.Replace(string(content), "OpenFile=\n", "", 1),
		strings.Repeat("x", 1025),
	} {
		if _, _, err := ParseObserverOpenFile([]byte(bad)); err == nil {
			t.Fatal("foreign or ambiguous drop-in accepted")
		}
	}
}

func TestObserverOpenFileReadbackProtectsOwnedFile(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("positive protected root-owned file requires UID 0")
	}
	instance := strings.Repeat("a", 64)
	identity := bootid.Identity{BootID: "12345678-1234-1234-1234-123456789abc", PID: 123, StartTime: 456}
	content, err := RenderObserverOpenFile(instance, identity)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// #nosec G302 -- private directory needs owner traversal; no group/other access.
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "10-openfile.conf")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := readObserverOpenFileAt(path, instance, identity); err != nil {
		t.Fatal(err)
	}
	other := identity
	other.StartTime++
	if readObserverOpenFileAt(path, instance, other) == nil {
		t.Fatal("foreign generation accepted")
	}
	// #nosec G302 -- deliberate group-readable negative fixture; production rejects it.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if readObserverOpenFileAt(path, instance, identity) == nil {
		t.Fatal("group-readable configuration accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "linked")
	if err := os.Link(path, linked); err != nil {
		t.Fatal(err)
	}
	if readObserverOpenFileAt(path, instance, identity) == nil {
		t.Fatal("multiply-linked configuration accepted")
	}
	if err := os.Remove(linked); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if readObserverOpenFileAt(alias, instance, identity) == nil {
		t.Fatal("symlink configuration accepted")
	}
	if err := os.WriteFile(path, append(content, []byte("ExecStart=/bin/sh\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if readObserverOpenFileAt(path, instance, identity) == nil {
		t.Fatal("extra command accepted")
	}
}
