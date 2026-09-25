package subsystems

// F-qos-flat: the flat QoS families of the `services` domain — DF-7's policer and qos descriptors (policer.Register,
// qos.Register: never df7/registry.Register, which also registers lb/mpls/span/lldp/bfd/vrrp/igmp — D-030) plus the
// qos.meta document record, all with the product's persisted stores:
//
//   - claims on untagged interfaces: the owner's DF-1 claim store (iface.SetClaimStore, installed by register);
//   - applied-once records of policer.interface (D-076/D-080): the owner's DF-7 BootStore (df7.SetBootStore);
//   - egress map ids: the agent's id range (Wiring.IDRange → df7.WithIDRange; the projection allocates from the same
//     range, desired.SetQoSMapIDRange); without a range the family owns no map id (fail closed) and says so;
//   - classify tables of policer.classify: DF-2's persisted classify store (df7.WithClassifyTables);
//   - the qos.meta record: <state dir>/qos-<owner>.json.
//
// No QoS global is set (D-071): flat QoS has none. policer.bind and policer.classify are registered (policer.Register)
// but belong to no domain: the configuration has no leaf for them (worker handoff needs worker threads; classifier
// policing tables belong to F-rpf-adl-pbr), so they are never planned.

import (
	"errors"
	"fmt"
	"path/filepath"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/policer"
	"ngfw/agent/internal/descriptors/qos"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

// registerQoS registers the F-qos-flat families with r.
func (w *Wiring) registerQoS(r scheduler.Registry) error {
	ids, err := w.IDRange()
	if errors.Is(err, ErrNoIDRange) {
		// fail closed (seams.go): the empty range owns no egress map id; policers, records, stores and marks
		// need no id and keep working. The product agent always has a range (TD-8b).
		w.env.Log.Warn("qos: no VPP id range — egress maps (services.qos.maps) cannot be created", "err", err)
	} else if err != nil {
		return fmt.Errorf("qos: %w", err)
	}
	opts := []df7.Option{df7.WithInterfaceKey(dfkit.DefaultInterfaceKey)}
	r7 := ids.DF7()
	if r7 != nil {
		opts = append(opts, df7.WithIDRange(r7.Lo, r7.Hi))
	}
	desired.SetQoSMapIDRange(r7)
	st, err := w.ClassifyStore()
	if err != nil {
		return fmt.Errorf("qos: classify store: %w", err)
	}
	opts = append(opts, df7.WithClassifyTables(func(name string) (uint32, bool) {
		rec, ok := st.Get(name)
		return rec.Index, ok
	}))
	c, owner := w.env.Client, w.env.Owner
	policer.Register(r, c, owner, opts...)
	qos.Register(r, c, owner, opts...)
	r.Register(qos.NewMeta(&qos.FileMetaStore{Path: filepath.Join(w.env.StateDir, "qos-"+owner+".json")}))
	return nil
}
