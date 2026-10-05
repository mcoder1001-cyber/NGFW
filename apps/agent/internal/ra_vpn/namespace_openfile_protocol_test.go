package ravpn

import (
	"ngfw/agent/internal/vpp/bootid"
	"strings"
	"testing"
)

func TestNumericPublisherRequestClosedRoleContract(t *testing.T) {
	source := bootid.Identity{BootID: "12345678-1234-1234-1234-123456789abc", PID: 100, StartTime: 7}
	target := source
	target.PID = 200
	server := source
	server.PID = 300
	valid := []numericPublisherRequest{
		{Phase: "probe", Source: source},
		{Phase: "publish", Source: source, Kind: NumericOpenFileTargets, Target: target, PreviousServer: server},
		{Phase: "publish", Source: source, Kind: NumericOpenFileObserver, Instance: strings.Repeat("a", 64), Target: target, PreviousServer: server},
	}
	for _, request := range valid {
		if err := validateNumericPublisherRequest(request); err != nil {
			t.Fatal(err)
		}
	}
	bad := []numericPublisherRequest{}
	for _, request := range valid {
		r := request
		r.Source.PID = 1
		bad = append(bad, r)
	}
	r := valid[0]
	r.Instance = "/tmp/foreign"
	bad = append(bad, r)
	r = valid[1]
	r.Kind = 99
	bad = append(bad, r)
	r = valid[1]
	r.Instance = strings.Repeat("a", 64)
	bad = append(bad, r)
	r = valid[2]
	r.Instance = "ngfw-ra@foreign.service"
	bad = append(bad, r)
	r = valid[2]
	r.PreviousServer = bootid.Identity{}
	bad = append(bad, r)
	r = valid[2]
	r.Target.BootID = "invalid"
	bad = append(bad, r)
	for _, request := range bad {
		if validateNumericPublisherRequest(request) == nil {
			t.Fatalf("accepted invalid role contract: phase=%s kind=%d", request.Phase, request.Kind)
		}
	}
}
