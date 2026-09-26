package span

import (
	"testing"

	"ngfw/agent/binapi/span"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
)

// F-loopback-bvi-gso-lldp-span: a destination deleted behind the agent's back leaves the session in VPP's span
// bookkeeping of our source (span.c has no interface-delete hook). Retrieve reports it as "#<sw_if_index>" and the
// reconciler's Delete of that undesired object clears exactly that bit of our source.
func TestStaleDestinationCleared(t *testing.T) {
	f, st := fakeSpan()
	ctx := t.Context()
	d := New(f, df7test.Owner)
	st[mk{1, 77, false}] = span.SPAN_STATE_API_RX_TX // loop0 → a vanished index
	st[mk{1, 4, false}] = span.SPAN_STATE_API_RX     // loop0 → eth0, kept
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var stale bool
	for _, kv := range kvs {
		stale = stale || kv.Key == "span.mirror/loop0/#77/device"
	}
	if !stale {
		t.Fatalf("stale session not retrieved: %v", kvs)
	}
	if err := d.Delete(ctx, df7.Encode(Mirror{Source: "loop0", Destination: "#77", State: StateBoth}), nil); err != nil {
		t.Fatal(err)
	}
	if _, left := st[mk{1, 77, false}]; left {
		t.Fatal("stale session not cleared")
	}
	if st[mk{1, 4, false}] != span.SPAN_STATE_API_RX {
		t.Fatalf("the other session was touched: %v", st)
	}
	// a destination that is simply gone by name (not an index spelling) is still success without a call
	f.Reset()
	if err := d.Delete(ctx, df7.Encode(Mirror{Source: "loop0", Destination: "gone0", State: StateBoth}), nil); err != nil || len(f.CallsNamed("sw_interface_span_enable_disable")) != 0 {
		t.Fatalf("delete of a vanished named destination: %v", err)
	}
}
