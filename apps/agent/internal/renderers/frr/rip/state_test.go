package rip

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusPeers(t *testing.T) {
	for _, tc := range []struct {
		v6            bool
		protocol, row string
	}{{false, "rip", "192.0.2.2 1 2 120 00:00:12"}, {true, "RIPng", "fe80::2\n 1 2 120 00:00:12"}} {
		raw := []byte("Routing Protocol is \"" + tc.protocol + "\"\nRouting Information Sources:\nGateway BadPackets BadRoutes Distance Last Update\n" + tc.row + "\n")
		value, e := ParseStatus(raw, tc.v6)
		if e != nil {
			t.Fatal(e)
		}
		var s Status
		if json.Unmarshal(value, &s) != nil || len(s.Peers) != 1 || s.Peers[0].BadRoutes != 2 {
			t.Fatal(string(value))
		}
	}
}
func TestStatusMalformed(t *testing.T) {
	for _, raw := range []string{"", "% Unknown command", "Routing Protocol is \"rip\"", "Routing Protocol is \"rip\"\nRouting Information Sources:\nGateway BadPackets BadRoutes Distance Last Update\n192.0.2.2 bogus 2 120 00:00:01", strings.Repeat("x", MaxStatusBytes+1)} {
		if _, e := ParseStatus([]byte(raw), false); e == nil {
			t.Fatal("malformed success")
		}
	}
}
