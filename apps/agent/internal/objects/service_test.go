package objects

import (
	"errors"
	"strings"
	"testing"
)

func specs(ps []PortSpec) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.String()
	}
	return strings.Join(s, "; ")
}

const servicesDoc = `{
  "services": {
    "https":   {"protocol": "tcp", "destinationPorts": ["443"]},
    "web":     {"protocol": "tcp", "destinationPorts": ["80", "8080-8090"], "tcpFlags": {"mask": 18, "value": 2}},
    "dns":     {"protocol": "tcp-udp", "destinationPorts": ["53"], "tcpFlags": {"mask": 2, "value": 2}},
    "sip":     {"protocol": "udp", "sourcePorts": ["5060"], "destinationPorts": ["5060-5061"]},
    "sctp":    {"protocol": "sctp", "destinationPorts": ["2905"]},
    "ping":    {"protocol": "icmp", "type": 8},
    "unreach": {"protocol": "icmp6", "type": 1, "code": 4},
    "icmpany": {"protocol": "icmp"},
    "gre":     {"protocol": "other", "number": 47},
    "any":     {"protocol": "any"}
  },
  "serviceGroups": {
    "web-all": {"members": ["https", "web", "https"]},
    "infra":   {"members": ["web-all", "dns", "ping", "gre"]},
    "loop-a":  {"members": ["loop-b"]},
    "loop-b":  {"members": ["loop-a"]}
  }
}`

func TestExpandService(t *testing.T) {
	doc := objectsDoc(t, servicesDoc)
	cases := map[string]string{
		"https": "proto 6 src 0-65535 dst 443-443 flags 0x00/0x00",
		"web":   "proto 6 src 0-65535 dst 80-80 flags 0x02/0x12; proto 6 src 0-65535 dst 8080-8090 flags 0x02/0x12",
		// tcp-udp: TCP and UDP entries, flags only on TCP
		"dns":     "proto 6 src 0-65535 dst 53-53 flags 0x02/0x02; proto 17 src 0-65535 dst 53-53 flags 0x00/0x00",
		"sip":     "proto 17 src 5060-5060 dst 5060-5061 flags 0x00/0x00",
		"sctp":    "proto 132 src 0-65535 dst 2905-2905 flags 0x00/0x00",
		"ping":    "proto 1 src 8-8 dst 0-255 flags 0x00/0x00",
		"unreach": "proto 58 src 1-1 dst 4-4 flags 0x00/0x00",
		"icmpany": "proto 1 src 0-255 dst 0-255 flags 0x00/0x00",
		"gre":     "proto 47 src 0-65535 dst 0-65535 flags 0x00/0x00",
		"any":     "proto 0 src 0-65535 dst 0-65535 flags 0x00/0x00",
		// a group: union, deduplicated (https twice), sorted
		"web-all": "proto 6 src 0-65535 dst 80-80 flags 0x02/0x12; proto 6 src 0-65535 dst 443-443 flags 0x00/0x00; proto 6 src 0-65535 dst 8080-8090 flags 0x02/0x12",
		"infra": "proto 1 src 8-8 dst 0-255 flags 0x00/0x00; proto 6 src 0-65535 dst 53-53 flags 0x02/0x02; proto 6 src 0-65535 dst 80-80 flags 0x02/0x12; " +
			"proto 6 src 0-65535 dst 443-443 flags 0x00/0x00; proto 6 src 0-65535 dst 8080-8090 flags 0x02/0x12; proto 17 src 0-65535 dst 53-53 flags 0x00/0x00; " +
			"proto 47 src 0-65535 dst 0-65535 flags 0x00/0x00",
	}
	for ref, want := range cases {
		got, err := ExpandService(doc, ref)
		if err != nil || specs(got) != want {
			t.Errorf("%s: %v\n got  %s\n want %s", ref, err, specs(got), want)
		}
	}
	if _, err := ExpandService(doc, "loop-a"); !errors.Is(err, ErrCycle) {
		t.Fatalf("cycle: %v", err)
	}
	if _, err := ExpandService(doc, "nosuch"); !errors.Is(err, ErrUnknownObject) {
		t.Fatalf("unknown: %v", err)
	}
	var le *LimitError
	if _, err := ExpandService(doc, "infra", WithLimit(3)); !errors.As(err, &le) || le.Ref != "infra" {
		t.Fatalf("limit: %v", err)
	}
}

// Inline specs of ACL rules take the same path; what the schema rejects is refused here too.
func TestExpandServiceSpecInvalid(t *testing.T) {
	doc := objectsDoc(t, `{"services": {
	  "code-no-type": {"protocol": "icmp", "code": 3},
	  "flags":        {"protocol": "tcp", "tcpFlags": {"mask": 1, "value": 2}},
	  "port0":        {"protocol": "udp", "destinationPorts": ["0"]},
	  "reversed":     {"protocol": "udp", "destinationPorts": ["90-80"]},
	  "noproto":      {"protocol": "gre"},
	  "nonumber":     {"protocol": "other"}
	}}`)
	for name, s := range doc.GetServices() {
		if _, err := ExpandServiceSpec(ServiceSpecOf(s)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
