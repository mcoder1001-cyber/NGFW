package subsystems

// Shared seams of the wave-A/B features (W-seed, launch plan §5.2): the agent's event sink and resync
// hook (A5) and the slot's id range. Each is inert until something uses it: no behaviour change.

import (
	"fmt"
	"os"
	"strconv"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// EnvTableBase is the environment variable of a test slot's first VRF/table id
// (docs/lab/shared-host-rules.md §1: slot N owns N000–N999).
const EnvTableBase = "VRX_VPP_TABLE_BASE"

// SlotIDRangeSize is the number of table/numeric ids a slot owns.
const SlotIDRangeSize = 1000

// IDRange is a closed range of numeric ids (FIB tables, SPD/SA ids, policy ids, map ids). Convert it
// to the family's own range type (df2.IDRange, df7.IDRange, vpn.IDRange) when registering.
type IDRange struct{ Lo, Hi uint32 }

// SlotIDRange returns the table/id range of this agent's slot from VRX_VPP_TABLE_BASE:
// base..base+999. nil (no error) when the variable is unset: the product agent owns every id, which is
// what a nil df2/df7 range (and the zero vpn.IDRange) means. A malformed value is an error, never a
// silent "own everything".
func SlotIDRange() (*IDRange, error) {
	s, ok := os.LookupEnv(EnvTableBase)
	if !ok || s == "" {
		return nil, nil
	}
	base, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("%s=%q is not a table id: %w", EnvTableBase, s, err)
	}
	if base == 0 || base > uint64(^uint32(0))-(SlotIDRangeSize-1) {
		return nil, fmt.Errorf("%s=%q is outside 1–%d (table 0 is VPP's default table; the range must fit in 32 bits)", EnvTableBase, s, uint64(^uint32(0))-(SlotIDRangeSize-1))
	}
	return &IDRange{Lo: uint32(base), Hi: uint32(base) + SlotIDRangeSize - 1}, nil
}

// Publish hands ev to the agent's event bus (Env.Publish; the bus assigns seq and time). Without a
// sink, and for a nil event, it does nothing.
func (w *Wiring) Publish(ev *vrxv1.Event) {
	if ev != nil && w.env.Publish != nil {
		w.env.Publish(ev)
	}
}

// RequestResync asks the agent for a full resync of its stored desired state (Env.Resync). Without a
// hook it does nothing.
func (w *Wiring) RequestResync() {
	if w.env.Resync != nil {
		w.env.Resync()
	}
}
