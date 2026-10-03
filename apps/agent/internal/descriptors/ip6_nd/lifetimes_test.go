package ip6nd

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/internal/scheduler"
)

func TestConfiguredLifetimesOnlyWithMatchingExpiries(t *testing.T) {
	start := time.Unix(1700000000, 0)
	key := scheduler.Key("ip6-nd.ra-prefix/loop1/2001:db8::/64")
	path := filepath.Join(t.TempDir(), "lifetimes.json")
	store, err := OpenLifetimeStore(path)
	if err != nil {
		t.Fatal(err)
	}
	record := lifetimeRecord{Index: 7, Valid: 86400, Preferred: 14400,
		ValidExpiry: expiry(86400, start, start), PreferredExpiry: expiry(14400, start, start)}
	if err = store.save(key, &record); err != nil {
		t.Fatal(err)
	}
	// Restart the descriptor's store: configured values remain backed by observed VPP deadlines.
	store, err = OpenLifetimeStore(path)
	if err != nil {
		t.Fatal(err)
	}
	now := start.Add(120 * time.Second)
	live := ip6_nd.IP6ndRaPrefix{ValLifetime: 86280, PrefLifetime: 14280,
		ValidLifetimeExpires: 86280, PrefLifetimeExpires: 14280, DecrementLifetimeFlag: true}
	cases := []struct {
		name     string
		change   func(*ip6_nd.IP6ndRaPrefix)
		idx      uint32
		restored bool
	}{
		{"countdown", func(*ip6_nd.IP6ndRaPrefix) {}, 7, true},
		{"valid timer changed by one second", func(p *ip6_nd.IP6ndRaPrefix) { p.ValLifetime--; p.ValidLifetimeExpires-- }, 7, false},
		{"preferred timer changed", func(p *ip6_nd.IP6ndRaPrefix) { p.PrefLifetime -= 60; p.PrefLifetimeExpires -= 60 }, 7, false},
		{"both timers reset", func(p *ip6_nd.IP6ndRaPrefix) {
			p.ValLifetime = 86400
			p.PrefLifetime = 14400
			p.ValidLifetimeExpires = 86400
			p.PrefLifetimeExpires = 14400
		}, 7, false},
		{"index reused", func(*ip6_nd.IP6ndRaPrefix) {}, 8, false},
		{"non countdown", func(p *ip6_nd.IP6ndRaPrefix) { p.DecrementLifetimeFlag = false }, 7, false},
		{"invalid expiry", func(p *ip6_nd.IP6ndRaPrefix) { p.ValidLifetimeExpires = math.NaN() }, 7, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := live
			tc.change(&p)
			value := &RaPrefix{ValidLifetime: p.ValLifetime, PreferredLifetime: p.PrefLifetime}
			store.restore(key, tc.idx, p, value, now, now)
			if tc.restored {
				if value.ValidLifetime != 86400 || value.PreferredLifetime != 14400 {
					t.Fatalf("not reconstructed: %v", value)
				}
			} else {
				if value.ValidLifetime != p.ValLifetime || value.PreferredLifetime != p.PrefLifetime {
					t.Fatalf("changed timer hidden: %v", value)
				}
			}
		})
	}
}

func TestExpiredAndInfiniteLifetimes(t *testing.T) {
	store, _ := OpenLifetimeStore("")
	start := time.Unix(1700000000, 0)
	key := scheduler.Key("prefix")
	record := lifetimeRecord{Index: 1, Valid: math.MaxUint32, Preferred: 10, PreferredExpiry: expiry(10, start, start)}
	if err := store.save(key, &record); err != nil {
		t.Fatal(err)
	}
	now := start.Add(20 * time.Second)
	live := ip6_nd.IP6ndRaPrefix{ValLifetime: math.MaxUint32, PrefLifetime: 0,
		PrefLifetimeExpires: -10, DecrementLifetimeFlag: true}
	value := &RaPrefix{ValidLifetime: live.ValLifetime, PreferredLifetime: live.PrefLifetime}
	store.restore(key, 1, live, value, now, now)
	if value.ValidLifetime != math.MaxUint32 || value.PreferredLifetime != 10 {
		t.Fatalf("expired/infinite: %v", value)
	}
	if err := store.save(key, nil); err != nil {
		t.Fatal(err)
	}
	value.PreferredLifetime = 0
	store.restore(key, 1, live, value, now, now)
	if value.PreferredLifetime != 0 {
		t.Fatal("deleted snapshot restored")
	}
}

func TestInitialExpiryCaptureRejectsConcurrentTimerChange(t *testing.T) {
	start := time.Unix(1700000000, 0)
	setEnd := start.Add(2 * time.Millisecond)
	dumpStart := start.Add(10 * time.Millisecond)
	dumpEnd := start.Add(12 * time.Millisecond)
	// Sample taken between request and response: fractional time remaining retains the exact deadline.
	if !initialExpiryMatches(86400, 86399, 86399.990, start, setEnd, dumpStart, dumpEnd) {
		t.Fatal("normal fractional expiry rejected")
	}
	if initialExpiryMatches(86400, 86398, 86398.990, start, setEnd, dumpStart, dumpEnd) {
		t.Fatal("concurrent one-second timer change accepted as configured")
	}
}
