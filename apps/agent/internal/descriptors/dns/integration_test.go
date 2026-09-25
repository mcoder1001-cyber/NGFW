package dns

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/dns"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration test against the host VPP. dns.enable is a VPP-global without a getter: nobody else
// on this host uses VPP's resolver (Unbound is RF-3's), so the test enables it with this slot's
// name servers and disables it again in Cleanup.
func TestDNSOnHost(t *testing.T) {
	// D-064: a host test that can crash the shared VPP is opt-in twice — VPP 26.06 segfaults on dns_resolve_name while
	// its dns plugin has no name server (2026-09-25 04:27, docs/vpp-code-track.md), so the dns.api path runs only when
	// the manager asks for it in a window.
	if os.Getenv("VRX_DNS_VPP_HOST") != "1" {
		t.Skip("dns.api on the shared VPP can crash it (V-item of F-unbound-chrony-syslog): set VRX_DNS_VPP_HOST=1 (and VRX_DF8_GLOBALS=1) in a manager window")
	}
	dfkittest.SkipUnlessGlobals(t, "dns_enable_disable / dns_name_server_add_del")
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	h.SkipUnlessCompatible(t, "dns", &dns.DNSEnableDisable{}, &dns.DNSNameServerAddDel{}, &dns.DNSResolveName{})
	c := h.Client()
	ctx := context.Background()
	slot := vpptest.Slot(t)
	// the test acts as the globals owner (D-071) for the resolver nobody else on this host uses
	ns := NewNameServer(c, dfkit.GlobalsOwner(true))
	en := NewEnable(c, dfkit.GlobalsOwner(true))
	v4 := NameServer{Address: fmt.Sprintf("10.%d.53.1", slot)}.Proto()
	v6 := NameServer{Address: fmt.Sprintf("fd00:%d::53", slot)}.Proto()
	on := Enable{Enabled: true}.Proto()
	t.Cleanup(func() {
		_ = en.Delete(context.Background(), on, nil)
		_ = ns.Delete(context.Background(), v4, nil)
		_ = ns.Delete(context.Background(), v6, nil)
	})
	steps := []struct {
		d scheduler.Descriptor
		v proto.Message
	}{{ns, v4}, {ns, v6}, {en, on}}
	for range 2 { // write-only: every resync re-applies
		for _, s := range steps {
			if _, err := s.d.Create(ctx, s.v); err != nil {
				t.Fatalf("%s create %v: %v", s.d.Name(), s.v, err)
			}
		}
	}
	for _, s := range steps {
		if _, err := s.d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
			t.Fatalf("%s retrieve: %v", s.d.Name(), err)
		}
	}
	// dns_resolve_name answers only when the upstream answers or VPP gives up (the upstreams here
	// are unreachable test addresses): bound the wait, the outcome is only logged.
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	ip4, ip6, err := ResolveName(rctx, c, h.Owner+"-nonexistent.invalid", true) // the steps above added the servers and enabled it
	cancel()
	t.Logf("dns_resolve_name via VPP: %v %v %v", ip4, ip6, err)
	dfkittest.HoldForEvidence(t, "CLI: show dns servers")
	for range 2 {
		if err := en.Delete(ctx, on, nil); err != nil {
			t.Fatal(err)
		}
		if err := ns.Delete(ctx, v4, nil); err != nil {
			t.Fatal(err)
		}
		if err := ns.Delete(ctx, v6, nil); err != nil {
			t.Fatal(err)
		}
	}
}
