package dhcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/dhcp6_ia_na_client_cp"
	"ngfw/agent/binapi/dhcp6_pd_client_cp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration test against the VPP on this host (docs/lab/shared-host-rules.md): VRX_INTEGRATION=1,
// shared lab lock, relays only in this slot's table range, clients only on this slot's tagged
// loopback, cleanup in t.Cleanup. The DUID is a VPP-global without getter or reset: its host
// check runs only with VRX_DF8_DUID=1.
func TestDHCPOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.LockGlobals(t)
	h.SkipUnlessCompatible(t, "dhcp", &dhcp.DHCPProxyConfig{}, &dhcp.DHCPProxyDump{}, &dhcp.DHCPClientConfig{},
		&dhcp.DHCPClientDump{}, &dhcp.DHCPProxySetVss{}, &dhcp6_ia_na_client_cp.DHCP6ClientEnableDisable{},
		&dhcp6_pd_client_cp.DHCP6PdClientEnableDisable{}, &dhcp6_pd_client_cp.IP6AddDelAddressUsingPrefix{})
	c := h.Client()
	ctx := context.Background()
	base := vpptest.TableBase(t)
	slot := vpptest.Slot(t)
	scope := WithVRFScope(func(v uint32) bool { return v >= base+800 && v < base+900 }) // restarttest uses +900..999
	rx := base + 801

	t.Run("proxy+vss", func(t *testing.T) {
		pd := NewProxy(c, scope)
		vd := NewProxyVSS(c, scope)
		p4 := Proxy{RxVRF: rx, ServerVRF: 0, Server: fmt.Sprintf("10.%d.80.1", slot), Src: fmt.Sprintf("10.%d.80.2", slot)}.Proto()
		p6 := Proxy{RxVRF: rx, ServerVRF: 0, Server: fmt.Sprintf("fd00:%d::80:1", slot), Src: fmt.Sprintf("fd00:%d::80:2", slot)}.Proto()
		vss := ProxyVSS{Family: "ip4", VRF: rx, Type: VSSASCII, VPNASCIIID: h.Owner + "-vpn"}.Proto()
		kvs := []*scheduler.KV{{Key: pd.KeyOf(p4), Value: p4}, {Key: pd.KeyOf(p6), Value: p6}}
		t.Cleanup(func() {
			_ = vd.Delete(context.Background(), vss, nil)
			for _, kv := range kvs {
				_ = pd.Delete(context.Background(), kv.Value, nil)
			}
		})
		for _, kv := range kvs {
			if _, err := pd.Create(ctx, kv.Value); err != nil {
				t.Fatalf("create %s: %v", kv.Key, err)
			}
			dfkittest.AssertRetrieved(t, pd, *kv)
		}
		if _, err := vd.Create(ctx, vss); err != nil {
			t.Fatalf("create vss: %v", err)
		}
		dfkittest.AssertRetrieved(t, vd, dfkittest.KV(vd, vss))
		dfkittest.AssertEmptyPlan(t, pd, *kvs[0], *kvs[1])
		dfkittest.AssertEmptyPlan(t, vd, dfkittest.KV(vd, vss))
		vss2 := ProxyVSS{Family: "ip4", VRF: rx, Type: VSSVPNID, OUI: 0x0a0b0c, VPNIndex: 77}.Proto()
		if _, err := vd.Update(ctx, vss, vss2, nil); err != nil {
			t.Fatalf("update vss: %v", err)
		}
		dfkittest.AssertRetrieved(t, vd, dfkittest.KV(vd, vss2))
		dfkittest.HoldForEvidence(t, "CLI: show dhcp proxy / show dhcpv6 proxy")
		if err := vd.Delete(ctx, vss2, nil); err != nil {
			t.Fatalf("delete vss: %v", err)
		}
		dfkittest.AssertAbsent(t, vd, vd.KeyOf(vss2))
		for _, kv := range kvs {
			if err := pd.Delete(ctx, kv.Value, nil); err != nil {
				t.Fatalf("delete %s: %v", kv.Key, err)
			}
			dfkittest.AssertAbsent(t, pd, kv.Key)
		}
	})

	t.Run("client", func(t *testing.T) {
		ifName, _ := h.Loopback(t, 81)
		d := NewClient(c, h.Owner)
		want := Client{Interface: ifName, Hostname: h.Owner + "-host", ClientID: h.Owner + "-cid", DSCP: 10}.Proto()
		kv := dfkittest.KV(d, want)
		t.Cleanup(func() { _ = d.Delete(context.Background(), want, nil) })
		meta, err := d.Create(ctx, want)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		got := dfkittest.AssertRetrieved(t, d, kv)
		if got.Meta != meta {
			t.Fatalf("Retrieve meta %v, Create meta %v", got.Meta, meta)
		}
		dfkittest.AssertEmptyPlan(t, d, kv)
		if _, err := d.Create(ctx, want); err != nil { // re-apply of the identical client
			t.Fatalf("re-create: %v", err)
		}
		leases, err := d.Leases(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("lease state %s: %+v", ifName, leases[ifName])
		dfkittest.HoldForEvidence(t, "CLI: show dhcp client")
		if err := d.Delete(ctx, want, meta); err != nil {
			t.Fatalf("delete: %v", err)
		}
		dfkittest.AssertAbsent(t, d, kv.Key)
	})

	t.Run("dhcp6 write-only", func(t *testing.T) {
		ifName, _ := h.Loopback(t, 82)
		c6 := NewDHCP6Client(c, h.Owner)
		pdc := NewDHCP6PDClient(c, h.Owner)
		pda := NewDHCP6PDAddress(c, h.Owner)
		v6 := DHCP6Client{Interface: ifName}.Proto()
		vpd := DHCP6PDClient{Interface: ifName, PrefixGroup: h.Owner + "-pd"}.Proto()
		vad := DHCP6PDAddress{Interface: ifName, PrefixGroup: h.Owner + "-pd", Address: "::1:0:0:0:1/64"}.Proto()
		t.Cleanup(func() {
			_ = pda.Delete(context.Background(), vad, nil)
			_ = pdc.Delete(context.Background(), vpd, nil)
			_ = c6.Delete(context.Background(), v6, nil)
		})
		for _, step := range []struct {
			d   scheduler.Descriptor
			obj *scheduler.KV
		}{{c6, &scheduler.KV{Value: v6}}, {pdc, &scheduler.KV{Value: vpd}}, {pda, &scheduler.KV{Value: vad}}} {
			for i := range 2 { // Create twice: write-only descriptors are re-applied on every resync
				if _, err := step.d.Create(ctx, step.obj.Value); err != nil {
					t.Fatalf("%s create #%d: %v", step.d.Name(), i+1, err)
				}
			}
			if _, err := step.d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
				t.Fatalf("%s Retrieve: %v, want ErrRetrieveUnsupported", step.d.Name(), err)
			}
		}
		dfkittest.HoldForEvidence(t, "CLI: show dhcp6 clients / show dhcp6 pd clients / show ip6 address using prefix")
		for _, step := range []struct {
			d   scheduler.Descriptor
			obj *scheduler.KV
		}{{pda, &scheduler.KV{Value: vad}}, {pdc, &scheduler.KV{Value: vpd}}, {c6, &scheduler.KV{Value: v6}}} {
			for i := range 2 { // Delete twice: absent counts as deleted
				if err := step.d.Delete(ctx, step.obj.Value, nil); err != nil {
					t.Fatalf("%s delete #%d: %v", step.d.Name(), i+1, err)
				}
			}
		}
	})

	t.Run("dhcp6 duid", func(t *testing.T) {
		if os.Getenv("VRX_DF8_DUID") != "1" {
			t.Skip("dhcp6_duid_ll_set changes a VPP-global without getter or reset; set VRX_DF8_DUID=1 to run")
		}
		d := NewDHCP6DUID(c, dfkit.GlobalsOwner(true))
		v := DHCP6DUID{DUIDLL: fmt.Sprintf("00:03:00:01:02:00:00:%02x:00:01", slot)}.Proto()
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatalf("create: %v", err)
		}
	})
}
