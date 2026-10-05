package desired

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

func TestRemoteAccessFailsClosedWithoutObjectsOrCredentials(t *testing.T) {
	for _, enabled := range []*bool{nil, proto.Bool(true), proto.Bool(false)} {
		ds := &ngfwv1.DesiredState{Vpn: &ngfwv1.VpnConfig{RemoteAccess: map[string]*ngfwv1.RemoteAccessProfile{
			"road/warrior~one": {Enabled: enabled},
		}}}
		s := &sink{}
		RemoteAccess(s, ds, map[string]bool{"vpn": true})
		if len(s.kvs) != 0 {
			t.Fatal("unavailable RA projected objects")
		}
		wanted := enabled == nil || *enabled
		if (len(s.errs) != 0) != wanted {
			t.Fatalf("enabled=%v errors=%v", enabled, s.errs)
		}
		if wanted && !strings.Contains(s.errs[0], "/vpn/remoteAccess/road~1warrior~0one/enabled") {
			t.Fatal(s.errs)
		}
	}
}

func TestRemoteAccessUnmanagedDomainUnchanged(t *testing.T) {
	s := &sink{}
	RemoteAccess(s, &ngfwv1.DesiredState{}, map[string]bool{"routing": true})
	if len(s.errs) != 0 || len(s.kvs) != 0 {
		t.Fatal("unmanaged VPN changed")
	}
}
