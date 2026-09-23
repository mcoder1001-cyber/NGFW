package span

import (
	"testing"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Host test: mirrors between this slot's loopbacks (loop<slot>40…), device and L2 level.
func TestSpanOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	d := New(h.C, h.Owner)
	src, _ := h.Loopback(40, true, true)
	dst, _ := h.Loopback(41, true, true)
	dst2, _ := h.Loopback(42, true, true)
	h.CleanupOwned(d)

	desired := []scheduler.KV{
		df7test.Desired(d, df7.Encode(Mirror{Source: src, Destination: dst, State: StateBoth})),
		df7test.Desired(d, df7.Encode(Mirror{Source: src, Destination: dst2, State: StateRx})),
		df7test.Desired(d, df7.Encode(Mirror{Source: dst2, Destination: dst, State: StateTx, L2: true})),
	}
	created := h.Apply(d, desired...)
	h.ExpectRetrieved(d, desired...)

	t.Run("update state in place", func(t *testing.T) {
		n := df7.Encode(Mirror{Source: src, Destination: dst2, State: StateTx})
		_, err := d.Update(h.Ctx, desired[1].Value, n, created[1].Meta)
		h.Must("update", err)
		desired[1].Value, created[1].Value = n, n
		h.ExpectRetrieved(d, desired...)
	})
	h.Hold("span mirrors")
	t.Run("restart simulation", func(t *testing.T) {
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return New(c, h.Owner) }, desired...)
	})
	h.DeleteAll(d, created)
	h.ExpectNone(d)
}
