package ip6nd

import (
	"fmt"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/vpp/vpptest"
)

func TestRaOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	loop, idx := df2test.Loopback(t, c, 3)
	other, otherIdx := df2test.Loopback(t, c, 4) // IPv6 enabled, RA left at defaults: never retrieved
	df2test.AddAddress(t, c, idx, fmt.Sprintf("2001:db8:%d:3::1/64", slot))
	df2test.AddAddress(t, c, otherIdx, fmt.Sprintf("2001:db8:%d:4::1/64", slot))

	cfg := NewRaConfig(c, owner)
	before, err := cfg.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if find(before, cfg.KeyOf(&RaConfig{Interface: loop})) != nil || find(before, cfg.KeyOf(&RaConfig{Interface: other})) != nil {
		t.Fatalf("default RA config must not be retrieved: %+v", before)
	}
	desired := &RaConfig{Interface: loop, Managed: true, Other: true, SuppressLinkLayerOption: true, RouterLifetime: 1800, MaxInterval: 300, InitialCount: 2}
	meta, err := cfg.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cfg.Delete(df2test.Ctx(t), desired, meta) })
	actual, err := cfg.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kv := find(actual, cfg.KeyOf(desired))
	if kv == nil {
		t.Fatalf("Retrieve does not show %s: %+v", cfg.KeyOf(desired), actual)
	}
	t.Logf("ra-config Retrieve = %+v", kv.Value)
	if want := NormalizeRaConfig(desired); !proto.Equal(kv.Value, want) || kv.Meta != meta {
		t.Fatalf("ra-config Retrieve = %+v, want %+v", kv.Value, want)
	}
	if find(actual, cfg.KeyOf(&RaConfig{Interface: other})) != nil {
		t.Fatalf("untouched interface retrieved: %+v", actual)
	}

	pfx := NewRaPrefix(c, owner)
	desiredPfx := &RaPrefix{Interface: loop, Prefix: fmt.Sprintf("2001:db8:%d:3::/64", slot), ValidLifetime: 7200, PreferredLifetime: 3600, OffLink: true}
	pmeta, err := pfx.Create(ctx, desiredPfx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pfx.Delete(df2test.Ctx(t), desiredPfx, pmeta) })
	pactual, err := pfx.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pkv := find(pactual, pfx.KeyOf(desiredPfx))
	if pkv == nil {
		t.Fatalf("Retrieve does not show %s: %+v", pfx.KeyOf(desiredPfx), pactual)
	}
	t.Logf("ra-prefix Retrieve = %+v", pkv.Value)
	if want := NormalizeRaPrefix(desiredPfx); !proto.Equal(pkv.Value, want) || pkv.Meta != pmeta {
		t.Fatalf("ra-prefix Retrieve = %+v, want %+v", pkv.Value, want)
	}
	// Update the prefix in place, then the config back to fewer flags.
	updatedPfx := &RaPrefix{Interface: loop, Prefix: desiredPfx.Prefix, NoAutoconfig: true}
	if _, err := pfx.Update(ctx, desiredPfx, updatedPfx, pmeta); err != nil {
		t.Fatal(err)
	}
	if pactual, _ = pfx.Retrieve(ctx); !proto.Equal(find(pactual, pfx.KeyOf(updatedPfx)).Value, NormalizeRaPrefix(updatedPfx)) {
		t.Fatalf("after prefix Update: %+v", pactual)
	}
	updated := &RaConfig{Interface: loop, Suppress: true}
	if _, err := cfg.Update(ctx, desired, updated, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = cfg.Retrieve(ctx); !proto.Equal(find(actual, cfg.KeyOf(updated)).Value, NormalizeRaConfig(updated)) {
		t.Fatalf("after config Update: %+v", find(actual, cfg.KeyOf(updated)))
	}
	df2test.Hold(t)
	if err := pfx.Delete(ctx, updatedPfx, pmeta); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Delete(ctx, updated, meta); err != nil {
		t.Fatal(err)
	}
	if pactual, _ = pfx.Retrieve(ctx); find(pactual, pfx.KeyOf(desiredPfx)) != nil {
		t.Fatalf("prefix still retrieved after Delete: %+v", pactual)
	}
	if actual, _ = cfg.Retrieve(ctx); find(actual, cfg.KeyOf(desired)) != nil {
		t.Fatalf("config still retrieved after Delete (defaults not restored): %+v", find(actual, cfg.KeyOf(desired)).Value)
	}
}

// TestProxyNdOnHost is disabled on the shared host: on 2026-09-23 15:52:38 the first
// ip6nd_proxy_add_del (loop305, 2001:db8:3:5::99) crashed VPP 26.06 (systemd restart #1,
// see docs/status/tasks/DF-2-questions.md). The descriptor is covered by the fake-client
// tests; re-enable with VRX_DF2_PROXY_ND=1 once the manager clears it.
func TestProxyNdOnHost(t *testing.T) {
	if os.Getenv("VRX_DF2_PROXY_ND") != "1" {
		t.Skip("skip: ip6nd_proxy_add_del crashed VPP 26.06 on vrx-a (2026-09-23 15:52:38); set VRX_DF2_PROXY_ND=1 to run")
	}
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	loop, idx := df2test.Loopback(t, c, 5)
	df2test.AddAddress(t, c, idx, fmt.Sprintf("2001:db8:%d:5::1/64", slot))
	d := NewProxyNd(c, owner)
	desired := &ProxyNd{Interface: loop, Address: fmt.Sprintf("2001:db8:%d:5::99", slot)}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Delete(df2test.Ctx(t), desired, meta) })
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kv := find(actual, d.KeyOf(desired))
	if kv == nil || !proto.Equal(kv.Value, desired) || kv.Meta != meta {
		t.Fatalf("Retrieve = %+v, want %+v", actual, desired)
	}
	t.Logf("proxy-nd Retrieve = %+v", kv.Value)
	df2test.Hold(t)
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); find(actual, d.KeyOf(desired)) != nil {
		t.Fatalf("still retrieved after Delete: %+v", actual)
	}
}

// TestDadOnHost is skip-unless-plugin-loaded: ip6_dad_autoremove is not loaded on vrx-a.
func TestDadOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	d := NewDad(c)
	before, err := d.Retrieve(ctx)
	df2test.SkipIfNotLoaded(t, DadPlugin, err)
	t.Logf("dad before (global, restored in Cleanup): %+v", before)
	desired := &Dad{Transmits: 2}
	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := df2test.Ctx(t)
		if len(before) == 0 {
			_ = d.Delete(ctx, desired, nil)
			return
		}
		_, _ = d.Create(ctx, before[0].Value)
	})
	actual, err := d.Retrieve(ctx)
	if err != nil || find(actual, d.KeyOf(desired)) == nil || !proto.Equal(find(actual, d.KeyOf(desired)).Value, NormalizeDad(desired)) {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if err := d.Delete(ctx, desired, nil); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("still enabled after Delete: %+v", actual)
	}
	t.Logf("dad: enabled (transmits 2) → retrieved → disabled → absent; ip6_dad.api is a core API on this host")
}
