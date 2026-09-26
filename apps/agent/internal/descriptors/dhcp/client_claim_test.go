package dhcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	dhcpapi "ngfw/agent/binapi/dhcp"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
)

// failingClaims is a claim store whose Claim fails (a full disk, a store that lost its VPP identity, …).
type failingClaims struct{ inner iface.ClaimStore }

func (f failingClaims) Claim(string, string) error { return errors.New("claim store: disk full") }
func (f failingClaims) Release(n, h string) error  { return f.inner.Release(n, h) }
func (f failingClaims) Claimed(n, h string) bool   { return f.inner.Claimed(n, h) }

// TD-11b Q3: dhcp.client claims BEFORE the VPP add. With the old order (add, then claim) a failing claim left an
// unclaimed client in VPP that Retrieve never reports and Delete never removes; this test fails on that order.
func TestClientClaimsBeforeVPPWrite(t *testing.T) {
	f, clients := newClientFake()
	owner := "w5td11b"
	iface.SetClaimStore(owner, failingClaims{inner: iface.NewMemoryClaimStore()})
	t.Cleanup(func() { iface.SetClaimStore(owner, iface.NewMemoryClaimStore()) })
	d := NewClient(f, owner)
	v := Client{Interface: "ens192", Hostname: "w5-wan"}.Proto()
	if _, err := d.Create(context.Background(), v); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("create with a failing claim store: %v", err)
	}
	if n := len(f.CallsNamed("dhcp_client_config")); n != 0 {
		t.Fatalf("%d dhcp_client_config calls: VPP was written before the claim", n)
	}
	if _, ok := clients[9]; ok {
		t.Fatal("client left in VPP without a claim")
	}

	// a VPP failure after a fresh claim releases it; nothing of ours remains
	store := iface.NewMemoryClaimStore()
	iface.SetClaimStore(owner, store)
	f.On("dhcp_client_config", func(api.Message) ([]api.Message, error) {
		return []api.Message{&dhcpapi.DHCPClientConfigReply{Retval: int32(api.INVALID_SW_IF_INDEX)}}, nil
	})
	if _, err := d.Create(context.Background(), v); !dfkit.IsVPPError(err, api.INVALID_SW_IF_INDEX) {
		t.Fatalf("create with a VPP failure: %v", err)
	}
	tg, err := dfkit.ResolveTarget(context.Background(), f, "ens192", owner, NameClient)
	if err != nil || tg.Claimed() {
		t.Fatalf("fresh claim not released after the failed add: %v", err)
	}
}
