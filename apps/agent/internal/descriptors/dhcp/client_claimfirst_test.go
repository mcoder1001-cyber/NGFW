package dhcp

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	iface "ngfw/agent/internal/descriptors/interface"
)

// TD-22 (TD-11b Q3, D-133): dhcp.client claims an untagged interface BEFORE dhcp_client_config.

// orderedClaims is an in-memory claim store that can refuse claims and records how many
// dhcp_client_config calls VPP had seen when each claim was made.
type orderedClaims struct {
	mu      sync.Mutex
	f       *dfkittest.FakeVPP
	refuse  error
	held    map[[2]string]bool
	atClaim []int
}

func (s *orderedClaims) Claim(ifName, holder string) error {
	if s.refuse != nil {
		return s.refuse
	}
	n := len(s.f.CallsNamed("dhcp_client_config"))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held[[2]string{ifName, holder}] = true
	s.atClaim = append(s.atClaim, n)
	return nil
}

func (s *orderedClaims) Release(ifName, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.held, [2]string{ifName, holder})
	return nil
}

func (s *orderedClaims) Claimed(ifName, holder string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held[[2]string{ifName, holder}]
}

func (s *orderedClaims) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.held)
}

func claimWorld(t *testing.T, owner string, refuse error) (*orderedClaims, *dfkittest.FakeVPP) {
	t.Helper()
	f, _ := newClientFake()
	s := &orderedClaims{f: f, refuse: refuse, held: map[[2]string]bool{}}
	iface.SetClaimStore(owner, s)
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	return s, f
}

// A claim that cannot be recorded fails the Create before anything is written to VPP.
func TestClientClaimsBeforeWrite(t *testing.T) {
	const own = "w5cf1"
	refused := errors.New("claim store: VPP boot identity not known yet")
	_, f := claimWorld(t, own, refused)
	_, err := NewClient(f, own).Create(context.Background(), Client{Interface: "ens192", Hostname: "w5-wan"}.Proto())
	if !errors.Is(err, refused) {
		t.Fatalf("Create = %v; want the claim error", err)
	}
	if n := len(f.CallsNamed("dhcp_client_config")); n != 0 {
		t.Fatalf("dhcp_client_config sent %d time(s) although the claim failed", n)
	}
}

// The claim is recorded before dhcp_client_config reaches VPP; a failed add releases it.
func TestClientClaimOrderAndUndo(t *testing.T) {
	const own = "w5cf2"
	s, f := claimWorld(t, own, nil)
	d := NewClient(f, own)
	v := Client{Interface: "ens192", Hostname: "w5-wan"}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	if len(s.atClaim) != 1 || s.atClaim[0] != 0 {
		t.Fatalf("claim made after %v dhcp_client_config call(s); want before the first", s.atClaim)
	}
	if err := d.Delete(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	if s.count() != 0 {
		t.Fatal("delete kept the claim")
	}
	f.On("dhcp_client_config", func(api.Message) ([]api.Message, error) { return nil, errors.New("vpp: write refused") })
	if _, err := d.Create(context.Background(), v); err == nil {
		t.Fatal("Create succeeded although the write failed")
	}
	if s.count() != 0 {
		t.Fatal("claim left behind by a failed add")
	}
}
