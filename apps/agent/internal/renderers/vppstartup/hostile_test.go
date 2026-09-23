package vppstartup

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"ngfw/agent/internal/renderers"
)

// hostileStrings is the framework's hostile table (renderers/helpers_template_test.go) plus the
// startup.conf-specific brace/newline injections.
var hostileStrings = []string{
	`"; rm -rf /`,
	"a\nb",
	"a\r\nb",
	"a\x00b",
	"a\x1b[2Jb",
	"a\u2028b",
	"a\xffb",
	"lan\n}\nunix { exec /tmp/x }",
	"lan }",
	"lan{",
	"lan {",
	"lan # comment",
	"a b",
	"",
}

const mgmt = `"managementPci":["0000:0b:00.0"]`

// TestHostile: every case must fail with ErrInput, and the error text must stay on one line
// (hostile keys are quoted in error paths).
func TestHostile(t *testing.T) {
	type tc struct {
		name, doc, want string
		host            *Host
	}
	var cases []tc
	for _, s := range hostileStrings {
		if !utf8.ValidString(s) {
			continue // not representable in a JSON document; covered by TestTemplateBackstop
		}
		q := jsonString(s)
		cases = append(cases,
			tc{"name " + q, `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":` + q + `}}}}`, "name", nil})
		if s == "" {
			continue // an empty suffix leaves a valid PCI address / plugin name
		}
		cases = append(cases,
			tc{"pci key " + q, `{"dataplane":{` + mgmt + `,"devices":{` + jsonString("0000:04:00.0"+s) + `:{"name":"wan"}}}}`, "PCI address", nil},
			tc{"whitelist " + q, `{"dataplane":{` + mgmt + `,"pciWhitelist":[` + jsonString("0000:04:00.0"+s) + `]}}`, "PCI address", nil},
			tc{"mgmt " + q, `{"dataplane":{"managementPci":[` + jsonString("0000:0b:00.0"+s) + `]}}`, "PCI address", nil},
			tc{"plugin " + q, `{"dataplane":{"plugins":{` + jsonString("acl_plugin.so"+s) + `:true}}}`, "plugin", nil},
		)
	}
	cases = append(cases, []tc{
		// logical names
		{"upper case", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"LAN"}}}}`, "not allowed", nil},
		{"too long", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"abcdefghijklmnop"}}}}`, "length 16", nil},
		{"dot (sub-if separator)", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"lan.100"}}}}`, "not allowed", nil},
		{"slash", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"eth0/1"}}}}`, "not allowed", nil},
		{"leading digit", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"0lan"}}}}`, "not allowed", nil},
		{"trailing dash", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"lan-"}}}}`, "must not end", nil},
		{"local0", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"local0"}}}}`, "reserved", nil},
		{"default", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"default"}}}}`, "reserved", nil},
		{"loop0 stem", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"loop0"}}}}`, "stem", nil},
		{"host- stem", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"host-eth0"}}}}`, "stem", nil},
		{"name not a string", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":7}}}}`, "must be a string", nil},
		{"unknown device field", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"nam":"lan"}}}}`, "unknown field", nil},
		{"duplicate logical name", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"lan"},"0000:0c:00.0":{"name":"lan"}}}}`, `logical name "lan" used by both 0000:04:00.0 and 0000:0c:00.0`, nil},
		// PCI
		{"duplicate PCI in whitelist", `{"dataplane":{` + mgmt + `,"pciWhitelist":["0000:04:00.0","0000:04:00.0"]}}`, "duplicate PCI address 0000:04:00.0", nil},
		{"duplicate PCI by case in whitelist", `{"dataplane":{` + mgmt + `,"pciWhitelist":["0000:0c:00.0","0000:0C:00.0"]}}`, "duplicate PCI address 0000:0c:00.0", nil},
		{"duplicate PCI by case in devices", `{"dataplane":{` + mgmt + `,"devices":{"0000:0c:00.0":{"name":"lan"},"0000:0C:00.0":{"name":"wan"}}}}`, "duplicate PCI address 0000:0c:00.0", nil},
		{"duplicate mgmt PCI", `{"dataplane":{"managementPci":["0000:0b:00.0","0000:0B:00.0"]}}`, "listed twice", nil},
		{"short PCI", `{"dataplane":{` + mgmt + `,"pciWhitelist":["04:00.0"]}}`, "PCI address", nil},
		{"PCI function 8", `{"dataplane":{` + mgmt + `,"pciWhitelist":["0000:04:00.8"]}}`, "function", nil},
		{"PCI device 20", `{"dataplane":{` + mgmt + `,"pciWhitelist":["0000:04:20.0"]}}`, "device", nil},
		// management NIC
		{"mgmt NIC in whitelist", `{"dataplane":{` + mgmt + `,"pciWhitelist":["0000:0b:00.0"]}}`, "management NIC 0000:0b:00.0 must never be a DPDK device", nil},
		{"mgmt NIC in devices", `{"dataplane":{` + mgmt + `,"devices":{"0000:0b:00.0":{"name":"mgmt"}}}}`, "management NIC 0000:0b:00.0 must never be a DPDK device", nil},
		{"mgmt NIC in devices by case", `{"dataplane":{` + mgmt + `,"devices":{"0000:0B:00.0":{"name":"mgmt"}}}}`, "management NIC 0000:0b:00.0 must never be a DPDK device", nil},
		{"devices without managementPci", `{"dataplane":{"devices":{"0000:04:00.0":{"name":"wan"}}}}`, "must name the management NIC", nil},
		{"managementPci not a list", `{"dataplane":{"managementPci":"0000:0b:00.0"}}`, "must be an array", nil},
		// plugins
		{"plugin not on disk", `{"dataplane":{"plugins":{"evil_plugin.so":true}}}`, "not installed", nil},
		{"plugin path traversal", `{"dataplane":{"plugins":{"../../../tmp/x_plugin.so":true}}}`, "not allowed", nil},
		{"plugin not a .so", `{"dataplane":{"plugins":{"acl_plugin":true}}}`, "not a plugin file name", nil},
		{"plugin value not bool", `{"dataplane":{"plugins":{"acl_plugin.so":"enable"}}}`, "must be a boolean", nil},
		{"dpdk disabled with devices", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"wan"}},"plugins":{"dpdk_plugin.so":false}}}`, "cannot be disabled while DPDK devices are listed", nil},
		{"plugins with unknown on-disk list", `{"dataplane":{"plugins":{"acl_plugin.so":true}}}`, "plugin list is unknown", &Host{}},
		// CPUs
		{"main core beyond host", `{"dataplane":{"mainCore":32}}`, "core 32 does not exist (host has 32 CPUs: 0-31)", nil},
		{"worker core beyond host", `{"dataplane":{"mainCore":1,"workers":2,"corelist":[2,40]}}`, "worker core 40 does not exist", nil},
		{"auto workers beyond host", `{"dataplane":{"mainCore":30,"workers":2}}`, "worker core 32 does not exist", nil},
		{"too many workers", `{"dataplane":{"workers":40}}`, "need 41 CPUs, host has 32", nil},
		{"main core is a worker", `{"dataplane":{"mainCore":2,"workers":2,"corelist":[2,3]}}`, "main core 2 is also a worker core", nil},
		{"corelist without main core", `{"dataplane":{"workers":2,"corelist":[2,3]}}`, "must be set when corelist is given", nil},
		{"workers != corelist", `{"dataplane":{"mainCore":1,"workers":3,"corelist":[2,3]}}`, "workers=3 but corelist names 2 cores", nil},
		{"corelist duplicate", `{"dataplane":{"mainCore":1,"corelist":[2,2]}}`, "core 2 listed twice", nil},
		{"worker not isolated", `{"dataplane":{"mainCore":1,"corelist":[2,3]}}`, "worker core 3 is not in the isolated CPU set (isolcpus=2,4-7)", &Host{CPUs: 8, IsolCPUs: []uint32{2, 4, 5, 6, 7}}},
		{"main core isolated", `{"dataplane":{"mainCore":2,"corelist":[4,5]}}`, "main core 2 is an isolated CPU", &Host{CPUs: 8, IsolCPUs: []uint32{2, 4, 5, 6, 7}}},
		{"auto worker not isolated", `{"dataplane":{"mainCore":1,"workers":2}}`, "worker core 3 is not in the isolated CPU set", &Host{CPUs: 8, IsolCPUs: []uint32{2, 4, 5, 6, 7}}},
		// RSS
		{"rx queues > workers", `{"dataplane":{"mainCore":1,"corelist":[2,3],"rxQueues":3}}`, "3 RX queues exceed the 2 worker thread(s)", nil},
		{"rx queues without workers", `{"dataplane":{"rxQueues":2}}`, "2 RX queues exceed the 1 worker thread(s)", nil},
		{"device rx queues > workers", `{"dataplane":{` + mgmt + `,"mainCore":1,"workers":2,"devices":{"0000:04:00.0":{"name":"wan","rxQueues":4}}}}`, "4 RX queues exceed", nil},
		{"rx desc not power of two", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"wan","rxDesc":1000}}}}`, "power of two", nil},
		{"negative queue", `{"dataplane":{` + mgmt + `,"devices":{"0000:04:00.0":{"name":"wan","txQueues":-1}}}}`, "must be an integer", nil},
		// hugepages
		{"buffers exceed hugepages", `{"dataplane":{"buffersPerNuma":1000000}}`, "exceeds the 2 GiB from host reservation", nil},
		{"buffers exceed configured hugepages", `{"dataplane":{"hugepagesGb":1,"buffersPerNuma":300000}}`, "exceeds the 1 GiB from dataplane.hugepagesGb", nil},
		{"fractional buffers", `{"dataplane":{"buffersPerNuma":1.5}}`, "must be an integer", nil},
	}...)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host := vrxA(t)
			if c.host != nil {
				host = *c.host
			}
			out, _, err := Generate(parseDoc(t, c.doc), host, DefaultSettings())
			if err == nil {
				t.Fatalf("accepted:\n%s", out)
			}
			if !errors.Is(err, ErrInput) {
				t.Errorf("error does not wrap ErrInput: %v", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
			if strings.ContainsAny(err.Error(), "\n\r\x00\x1b") {
				t.Errorf("error text is not one clean line: %q", err)
			}
		})
	}
}

// TestTemplateBackstop bypasses BuildModel: even a model with hostile strings cannot inject a
// line, because every string goes through ident/pathtok in the template.
func TestTemplateBackstop(t *testing.T) {
	for _, s := range hostileStrings {
		for name, m := range map[string]*Model{
			"device name": {DPDK: true, Devices: []Device{{PCI: "0000:04:00.0", Name: s}}},
			"device pci":  {DPDK: true, Devices: []Device{{PCI: s}}},
			"blacklist":   {DPDK: true, Blacklist: []string{s}},
			"plugin":      {Plugins: []Plugin{{File: s, Enable: true}}},
		} {
			if name == "device name" && s == "" {
				continue // an empty name means "no name" at the model level
			}
			if out, err := RenderModel(m, DefaultSettings()); err == nil {
				t.Errorf("%s %q rendered:\n%s", name, s, out)
			}
		}
		st := DefaultSettings()
		st.LogFile = "/var/log/" + s
		if _, err := RenderModel(&Model{}, st); err == nil {
			t.Errorf("log path %q rendered", s)
		}
	}
	if _, err := RenderModel(nil, DefaultSettings()); err == nil {
		t.Error("nil model rendered")
	}
	// the helpers reject with the framework's error
	if _, err := PathToken("relative/x"); !errors.Is(err, renderers.ErrUnsafe) {
		t.Error(err)
	}
}

func TestLogicalNameAccepts(t *testing.T) {
	for _, ok := range []string{"wan", "lan", "lan2", "dmz", "p2p", "sync", "green", "loopback", "wan_1", "a-b", "hostname"} {
		if err := LogicalName(ok); err != nil {
			t.Errorf("LogicalName(%q) = %v", ok, err)
		}
	}
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r >= 0x7f:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
