package subsystems

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	iface "ngfw/agent/internal/descriptors/interface"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"testing"
)

type adminMutationRecorder struct {
	scheduler.Descriptor
	writes int
}

func (r *adminMutationRecorder) Retrieve(context.Context) ([]scheduler.KV, error) {
	return []scheduler.KV{{Value: &iface.AdminState{Interface: "interface.ipip/ipip8001"}}, {Value: &iface.AdminState{Interface: "interface.ipip/ipip8002"}}}, nil
}
func (r *adminMutationRecorder) Create(context.Context, proto.Message) (any, error) {
	r.writes++
	return nil, nil
}
func (r *adminMutationRecorder) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	r.writes++
	return nil, nil
}
func (r *adminMutationRecorder) Delete(context.Context, proto.Message, any) error {
	r.writes++
	return nil
}

// No incoming VPN document or SA is required: the live profile retains control
// before negotiation and after deletion, including a tunnels-only partial Apply.
func TestNativeAdminGuardRetainsProtectionWithoutSA(t *testing.T) {
	ctx := context.Background()
	target := &iface.AdminState{Interface: "interface.ipip/ipip8001"}
	record := &adminMutationRecorder{}
	guard := &nativeAdminGuard{Descriptor: record, owner: "guard-test", profiles: func(context.Context, string) ([]scheduler.KV, error) {
		return []scheduler.KV{{Value: &vpnpb.Ikev2Profile{Name: "site", TunnelInterface: "ipip8001"}}}, nil
	}}
	actual, e := guard.Retrieve(ctx)
	if e != nil || len(actual) != 1 || iface.RefID(actual[0].Value.(*iface.AdminState).Interface) != "ipip8002" {
		t.Fatalf("protected runtime admin state leaked into reconciliation: %v %v", actual, e)
	}
	for _, phase := range []string{"before-sa", "after-sa-deletion"} {
		t.Run(phase, func(t *testing.T) {
			if _, e := guard.Create(ctx, target); e == nil {
				t.Fatal("allowed protected IPIP admin-up")
			}
			if _, e := guard.Update(ctx, target, target, nil); e == nil {
				t.Fatal("allowed protected IPIP admin update")
			}
			if record.writes != 0 {
				t.Fatal("reached VPP mutation")
			}
		})
	}
	if e := guard.Delete(ctx, target, nil); e != nil {
		t.Fatal(e)
	}
	if record.writes != 1 {
		t.Fatal("admin-down must remain possible")
	}
	guard.profiles = func(context.Context, string) ([]scheduler.KV, error) { return nil, errors.New("state unavailable") }
	if _, e := guard.Create(ctx, target); e == nil {
		t.Fatal("unavailable profile state must fail closed")
	}
	if record.writes != 1 {
		t.Fatal("unavailable state caused write")
	}
	guard.profiles = func(context.Context, string) ([]scheduler.KV, error) { return nil, nil }
	if _, e := guard.Create(ctx, target); e != nil {
		t.Fatal(e)
	}
	if record.writes != 2 {
		t.Fatal("unprotected IPIP admin-up should remain available")
	}
}
