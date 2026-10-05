package ravpn

import (
	"math"
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
