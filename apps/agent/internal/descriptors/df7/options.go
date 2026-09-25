package df7

import (
	"strconv"

	"ngfw/agent/internal/scheduler"
)

// Cross-task key prefixes this factory depends on (D-065 interface alias, P05 core vrf and
// interface-ip, DF-2 classify tables).
const (
	InterfaceKeyPrefix   = "interface"
	VRFKeyPrefix         = "vrf"
	InterfaceIPKeyPrefix = "interface-ip"
	// ClassifyTablePrefix is DF-2's classify table descriptor name as built on task/DF-2
	// ("classify.table/<name>"); the DF-7 prompt wrote "classify-table/…" (DF-7-questions.md Q2).
	ClassifyTablePrefix = "classify.table"
)

// InterfaceKey is the generic interface reference "interface/<name>" (D-065).
func InterfaceKey(name string) scheduler.Key { return scheduler.Join(InterfaceKeyPrefix, name) }

// VRFKey is P05 core's FIB table key "vrf/<id>".
func VRFKey(id uint32) scheduler.Key {
	return scheduler.Join(VRFKeyPrefix, strconv.FormatUint(uint64(id), 10))
}

// InterfaceIPKey is P05 core's interface address key "interface-ip/<if>/<addr>/<len>".
func InterfaceIPKey(ifName, prefix string) scheduler.Key {
	return scheduler.Join(InterfaceIPKeyPrefix, ifName, prefix)
}

// ClassifyTableKey is DF-2's classify table key "classify.table/<name>".
func ClassifyTableKey(name string) scheduler.Key { return scheduler.Join(ClassifyTablePrefix, name) }

// IDRange is the closed range of numeric ids (QoS egress map ids, BFD conf-key ids) this agent
// owns on a shared VPP. A nil range owns every id (production: one agent per VPP).
type IDRange struct{ Lo, Hi uint32 }

// Owns reports whether id is inside the range; a nil range owns everything.
func (r *IDRange) Owns(id uint32) bool { return r == nil || (id >= r.Lo && id <= r.Hi) }

// Options are the settings every DF-7 plugin's Register and constructors take.
type Options struct {
	// InterfaceKey maps an interface name to its dependency key (default InterfaceKey).
	InterfaceKey func(name string) scheduler.Key
	// IDs is the numeric id range of untagged objects this owner may create and retrieve.
	IDs *IDRange
	// ClassifyIndex resolves a DF-2 classify table name to its VPP table index (P05 wires
	// DF-2's classify Store.Get); nil = no classify tables known.
	ClassifyIndex func(name string) (uint32, bool)
}

// Option configures Options.
type Option func(*Options)

// BuildOptions applies opts over the defaults.
func BuildOptions(opts []Option) Options {
	o := Options{InterfaceKey: InterfaceKey}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// WithInterfaceKey overrides the interface dependency key scheme.
func WithInterfaceKey(f func(name string) scheduler.Key) Option {
	return func(o *Options) {
		if f != nil {
			o.InterfaceKey = f
		}
	}
}

// WithIDRange restricts the numeric ids of untagged objects to lo..hi.
func WithIDRange(lo, hi uint32) Option {
	return func(o *Options) { o.IDs = &IDRange{Lo: lo, Hi: hi} }
}

// WithIDs sets the numeric id range of untagged objects to a copy of r, as the agent's wiring hands
// it over (subsystems.Wiring.IDRange converted with DF7, TD-8b / TD-8 verify V2): nil = every id
// (VRX_VPP_ID_RANGE=all), an empty range (Lo > Hi, subsystems.NoIDs) = no id. A family registers
// with WithIDs(ids.DF7()), never with a missing option.
func WithIDs(r *IDRange) Option {
	return func(o *Options) {
		if r == nil {
			o.IDs = nil
			return
		}
		c := *r
		o.IDs = &c
	}
}

// WithClassifyTables sets the classify table name → index resolver.
func WithClassifyTables(f func(name string) (uint32, bool)) Option {
	return func(o *Options) { o.ClassifyIndex = f }
}

// IfaceDep is the mandatory dependency on an interface.
func (o Options) IfaceDep(name string) scheduler.Dependency {
	return scheduler.Dependency{Key: o.InterfaceKey(name)}
}

// CheckID fails with ErrSpec when id is outside the owned range.
func (o Options) CheckID(what string, id uint32) error {
	if !o.IDs.Owns(id) {
		return Specf("%s %d is outside this owner's id range %d..%d", what, id, o.IDs.Lo, o.IDs.Hi)
	}
	return nil
}
