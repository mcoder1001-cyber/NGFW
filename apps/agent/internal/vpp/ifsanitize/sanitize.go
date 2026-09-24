// Package ifsanitize clears the per-interface state VPP 26.06 keeps on a sw_if_index after the
// interface is deleted (docs/vpp-code-track.md V19, V21; LOG D-095). VPP reuses a freed
// sw_if_index for the next interface of any type, and several per-index vectors are not reset
// by the sw-interface delete callbacks:
//
//   - ip4/ip6 "ip classify table" (classify_set_interface_ip_table): read directly when an
//     address is added — the new interface gets a classify DPO to the old table on its /32.
//     If that table was deleted, the first packet to the address crashes VPP in
//     vnet_classify_find_entry (2026-09-24 04:50:27). No readback exists: reset blindly.
//   - l2 input/output classify tables (classify_set_interface_l2_tables): no readback: reset
//     blindly (all three tables ~0 also disables the l2 feature bit).
//   - input ACL tables: readback with classify_table_by_interface; unbound when the table
//     still exists (VPP refuses an unbind naming a freed table).
//   - output ACL, policer classify and flow classify tables: no usable readback
//     (policer/flow_classify_dump read out of bounds per interface, see policer/attach.go), so
//     each existing classify table is probed with an unbind: VPP answers NO_SUCH_TABLE unless
//     that exact table is bound on the interface, and unbinds it otherwise.
//   - ADL: the adl-input feature on device-input (read with feature_is_enabled).
//   - vxlan bypass: a per-index bitmap that makes a later enable a silent no-op (V21); reset
//     blindly (disable is a no-op when the bit is clear).
//   - IPsec SPD binding (ipsec_interface_add_del_spd; DF-5 review M3): spd_index_by_sw_if_index
//     survives the interface, and VPP then refuses to bind an SPD to the new interface ("spd
//     already assigned"). Read with ipsec_spd_interface_dump; any binding on a new interface
//     is stale by definition (nothing of ours is bound yet) and is removed. The unbind needs an
//     existing spd_id but does not check it against the bound one (ipsec_set_interface_spd),
//     and the dump reports the SPD pool index, not its id, so the first spd_id of
//     ipsec_spds_dump is used (deleting an SPD clears every binding to it, so a stale binding
//     always refers to an existing SPD).
//
// A binding that points at a freed table cannot be removed through the API. It is harmless
// while the classify feature is off (VPP disables every feature arc on interface delete and a
// later add of the same kind is a no-op), so it is reported as "unclearable" and logged; when
// the corresponding feature is enabled on the new interface Sanitize fails, and the creator
// removes the interface instead of reporting it created.
//
// Sanitize uses only binapi messages and touches only the given sw_if_index. Messages of a
// plugin that is not loaded are skipped. Every run is logged at info and counted (Metrics).
package ifsanitize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"

	adlapi "ngfw/agent/binapi/adl"
	classifyapi "ngfw/agent/binapi/classify"
	featureapi "ngfw/agent/binapi/feature"
	"ngfw/agent/binapi/interface_types"
	ipsecapi "ngfw/agent/binapi/ipsec"
	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/vpp"
)

// NoIndex is VPP's "none" table index (~0).
const NoIndex = ^uint32(0)

// ErrNoSuchTable is VNET_API_ERROR_NO_SUCH_TABLE (vnet/error.h): the answer to an unbind that
// names a table which is not the one bound.
const ErrNoSuchTable api.VPPApiError = -65

// ErrActiveStaleBinding means the new interface has a classify feature enabled whose table
// binding cannot be removed (its table is gone): a packet would crash VPP (V19).
var ErrActiveStaleBinding = errors.New("ifsanitize: active classify feature bound to a deleted table (VPP V19)")

// State names used in reports, logs and the metric label.
const (
	StateIPClassify      = "ip-classify"      // classify_set_interface_ip_table
	StateL2Classify      = "l2-classify"      // classify_set_interface_l2_tables
	StateInputACL        = "input-acl"        // input_acl_set_interface
	StateOutputACL       = "output-acl"       // output_acl_set_interface
	StatePolicerClassify = "policer-classify" // policer_classify_set_interface
	StateFlowClassify    = "flow-classify"    // flow_classify_set_interface
	StateADL             = "adl"              // adl_interface_enable_disable
	StateVxlanBypass     = "vxlan-bypass"     // sw_interface_set_vxlan_bypass
	StateIPsecSPD        = "ipsec-spd"        // ipsec_interface_add_del_spd
)

// Report is what one Sanitize run found and did.
type Report struct {
	SwIfIndex uint32
	Name      string
	// Reset lists the states reset blindly (no readback in VPP; the call is a no-op when unset).
	Reset []string
	// Cleared lists inherited state that was found and removed ("input-acl ip4 table 3").
	Cleared []string
	// Unclearable lists inherited bindings to deleted tables (dormant: feature off).
	Unclearable []string
	// Skipped lists steps skipped because VPP does not know the message (plugin not loaded).
	Skipped []string
}

// Inherited reports whether anything inherited was found.
func (r Report) Inherited() bool { return len(r.Cleared) > 0 || len(r.Unclearable) > 0 }

type sanitizer struct {
	ctx context.Context
	c   vpp.Client
	idx interface_types.InterfaceIndex
	ids []uint32 // live classify table indices
	rep *Report
}

// Sanitize clears inherited per-interface state on idx, a sw_if_index VPP has just returned for
// a new interface called name (used in logs and errors only). Call it after the create and
// before the interface is reported created; on error the caller removes the interface.
func Sanitize(ctx context.Context, c vpp.Client, idx uint32, name string) (Report, error) {
	rep := Report{SwIfIndex: idx, Name: name}
	s := &sanitizer{ctx: ctx, c: c, idx: interface_types.InterfaceIndex(idx), rep: &rep}
	err := s.run()
	record(rep, err)
	log := slog.Default().With("interface", name, "sw_if_index", idx)
	switch {
	case err != nil:
		log.Error("interface sanitize failed (VPP V19)", "err", err, "cleared", rep.Cleared, "unclearable", rep.Unclearable)
	case len(rep.Unclearable) > 0:
		log.Warn("interface sanitized: inherited bindings to deleted classify tables remain (dormant, VPP V19)",
			"cleared", rep.Cleared, "unclearable", rep.Unclearable, "reset", rep.Reset, "skipped", rep.Skipped)
	default:
		log.Info("interface sanitized (VPP V19/V21 inherited state)", "cleared", rep.Cleared, "reset", rep.Reset, "skipped", rep.Skipped)
	}
	if err != nil {
		return rep, fmt.Errorf("sanitize %s (sw_if_index %d): %w", name, idx, err)
	}
	return rep, nil
}

func (s *sanitizer) run() error {
	ids, err := classifyapi.NewServiceClient(s.c).ClassifyTableIds(s.ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return fmt.Errorf("classify_table_ids: %w", err)
	}
	s.ids = ids.Ids
	for _, step := range []func() error{s.ipClassify, s.l2Classify, s.inputACL, s.outputACL, s.policerClassify, s.flowClassify, s.adl, s.vxlanBypass, s.ipsecSPD} {
		if err := step(); err != nil {
			return err
		}
	}
	return s.activeCheck()
}

// unknownMsg reports whether err says the VPP does not know the message (plugin not loaded).
func unknownMsg(err error) bool {
	var unknown *adapter.UnknownMsgError
	return errors.As(err, &unknown)
}

func isRetval(err error, want api.VPPApiError) bool {
	var apiErr api.VPPApiError
	return errors.As(err, &apiErr) && apiErr == want
}

func (s *sanitizer) exists(table uint32) bool {
	for _, id := range s.ids {
		if id == table {
			return true
		}
	}
	return false
}

func (s *sanitizer) ipClassify() error {
	svc := classifyapi.NewServiceClient(s.c)
	for _, v6 := range []bool{false, true} {
		if _, err := svc.ClassifySetInterfaceIPTable(s.ctx, &classifyapi.ClassifySetInterfaceIPTable{IsIPv6: v6, SwIfIndex: s.idx, TableIndex: NoIndex}); err != nil {
			return fmt.Errorf("classify_set_interface_ip_table (reset ip%s): %w", af(v6), err)
		}
		s.rep.Reset = append(s.rep.Reset, StateIPClassify+" ip"+af(v6))
	}
	return nil
}

func af(v6 bool) string {
	if v6 {
		return "6"
	}
	return "4"
}

func (s *sanitizer) l2Classify() error {
	svc := classifyapi.NewServiceClient(s.c)
	for _, in := range []bool{true, false} {
		req := &classifyapi.ClassifySetInterfaceL2Tables{SwIfIndex: s.idx, IP4TableIndex: NoIndex, IP6TableIndex: NoIndex, OtherTableIndex: NoIndex, IsInput: in}
		if _, err := svc.ClassifySetInterfaceL2Tables(s.ctx, req); err != nil {
			return fmt.Errorf("classify_set_interface_l2_tables (reset %s): %w", dir(in), err)
		}
		s.rep.Reset = append(s.rep.Reset, StateL2Classify+" "+dir(in))
	}
	return nil
}

func dir(in bool) string {
	if in {
		return "input"
	}
	return "output"
}

// slot is one table kind of a three-table binding message.
type slot struct {
	name string
	set  func(table uint32) (ip4, ip6, l2 uint32)
}

var aclSlots = []slot{
	{"ip4", func(t uint32) (uint32, uint32, uint32) { return t, NoIndex, NoIndex }},
	{"ip6", func(t uint32) (uint32, uint32, uint32) { return NoIndex, t, NoIndex }},
	{"l2", func(t uint32) (uint32, uint32, uint32) { return NoIndex, NoIndex, t }},
}

func (s *sanitizer) inputACL() error {
	svc := classifyapi.NewServiceClient(s.c)
	cur, err := svc.ClassifyTableByInterface(s.ctx, &classifyapi.ClassifyTableByInterface{SwIfIndex: s.idx})
	if err != nil {
		return fmt.Errorf("classify_table_by_interface: %w", err)
	}
	for i, t := range []uint32{cur.IP4TableID, cur.IP6TableID, cur.L2TableID} {
		if t == NoIndex {
			continue
		}
		sl := aclSlots[i]
		what := fmt.Sprintf("%s %s table %d", StateInputACL, sl.name, t)
		if !s.exists(t) {
			s.rep.Unclearable = append(s.rep.Unclearable, what+" (table deleted)")
			continue
		}
		ip4, ip6, l2 := sl.set(t)
		if _, err := svc.InputACLSetInterface(s.ctx, &classifyapi.InputACLSetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: l2, IsAdd: false}); err != nil {
			return fmt.Errorf("input_acl_set_interface (unbind %s): %w", what, err)
		}
		s.rep.Cleared = append(s.rep.Cleared, what)
	}
	return nil
}

// probe unbinds every live table from every slot of one binding kind: NO_SUCH_TABLE means
// "not this table", success means it was bound and is now removed.
func (s *sanitizer) probe(state string, slots []slot, unbind func(ip4, ip6, l2 uint32) error) error {
	for _, t := range s.ids {
		for _, sl := range slots {
			ip4, ip6, l2 := sl.set(t)
			err := unbind(ip4, ip6, l2)
			switch {
			case err == nil:
				s.rep.Cleared = append(s.rep.Cleared, fmt.Sprintf("%s %s table %d", state, sl.name, t))
			case isRetval(err, ErrNoSuchTable):
			case unknownMsg(err):
				s.rep.Skipped = append(s.rep.Skipped, state)
				return nil
			default:
				return fmt.Errorf("%s: probe unbind %s table %d: %w", state, sl.name, t, err)
			}
		}
	}
	return nil
}

func (s *sanitizer) outputACL() error {
	svc := classifyapi.NewServiceClient(s.c)
	return s.probe(StateOutputACL, aclSlots, func(ip4, ip6, l2 uint32) error {
		_, err := svc.OutputACLSetInterface(s.ctx, &classifyapi.OutputACLSetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: l2, IsAdd: false})
		return err
	})
}

func (s *sanitizer) policerClassify() error {
	svc := classifyapi.NewServiceClient(s.c)
	return s.probe(StatePolicerClassify, aclSlots, func(ip4, ip6, l2 uint32) error {
		_, err := svc.PolicerClassifySetInterface(s.ctx, &classifyapi.PolicerClassifySetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, L2TableIndex: l2, IsAdd: false})
		return err
	})
}

func (s *sanitizer) flowClassify() error {
	svc := classifyapi.NewServiceClient(s.c)
	return s.probe(StateFlowClassify, aclSlots[:2], func(ip4, ip6, _ uint32) error {
		_, err := svc.FlowClassifySetInterface(s.ctx, &classifyapi.FlowClassifySetInterface{SwIfIndex: s.idx, IP4TableIndex: ip4, IP6TableIndex: ip6, IsAdd: false})
		return err
	})
}

// featureOn reads feature_is_enabled; ok=false when VPP does not know the message.
func (s *sanitizer) featureOn(arc, feature string) (on, ok bool, err error) {
	rep, err := featureapi.NewServiceClient(s.c).FeatureIsEnabled(s.ctx, &featureapi.FeatureIsEnabled{ArcName: arc, FeatureName: feature, SwIfIndex: s.idx})
	if unknownMsg(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("feature_is_enabled %s/%s: %w", arc, feature, err)
	}
	return rep.IsEnabled, true, nil
}

func (s *sanitizer) adl() error {
	on, ok, err := s.featureOn("device-input", "adl-input")
	if err != nil || !ok || !on {
		return err
	}
	_, err = adlapi.NewServiceClient(s.c).AdlInterfaceEnableDisable(s.ctx, &adlapi.AdlInterfaceEnableDisable{SwIfIndex: s.idx, EnableDisable: false})
	if unknownMsg(err) {
		s.rep.Skipped = append(s.rep.Skipped, StateADL)
		return nil
	}
	if err != nil {
		return fmt.Errorf("adl_interface_enable_disable (disable): %w", err)
	}
	s.rep.Cleared = append(s.rep.Cleared, StateADL+" adl-input")
	return nil
}

func (s *sanitizer) vxlanBypass() error {
	svc := vxlanapi.NewServiceClient(s.c)
	for _, v6 := range []bool{false, true} {
		_, err := svc.SwInterfaceSetVxlanBypass(s.ctx, &vxlanapi.SwInterfaceSetVxlanBypass{SwIfIndex: s.idx, IsIPv6: v6, Enable: false})
		if unknownMsg(err) {
			s.rep.Skipped = append(s.rep.Skipped, StateVxlanBypass)
			return nil
		}
		if err != nil {
			return fmt.Errorf("sw_interface_set_vxlan_bypass (reset ip%s): %w", af(v6), err)
		}
		s.rep.Reset = append(s.rep.Reset, StateVxlanBypass+" ip"+af(v6))
	}
	return nil
}

func (s *sanitizer) ipsecSPD() error {
	svc := ipsecapi.NewServiceClient(s.c)
	stream, err := svc.IpsecSpdInterfaceDump(s.ctx, &ipsecapi.IpsecSpdInterfaceDump{})
	if unknownMsg(err) {
		s.rep.Skipped = append(s.rep.Skipped, StateIPsecSPD)
		return nil
	}
	if err != nil {
		return fmt.Errorf("ipsec_spd_interface_dump: %w", err)
	}
	var bound []uint32
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if unknownMsg(err) {
			s.rep.Skipped = append(s.rep.Skipped, StateIPsecSPD)
			return nil
		}
		if err != nil {
			return fmt.Errorf("ipsec_spd_interface_dump: %w", err)
		}
		if d.SwIfIndex == s.idx {
			bound = append(bound, d.SpdIndex)
		}
	}
	if len(bound) == 0 {
		return nil
	}
	spds, err := svc.IpsecSpdsDump(s.ctx, &ipsecapi.IpsecSpdsDump{})
	if err != nil {
		return fmt.Errorf("ipsec_spds_dump: %w", err)
	}
	spdID, found := uint32(0), false
	for {
		d, err := spds.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("ipsec_spds_dump: %w", err)
		}
		if !found {
			spdID, found = d.SpdID, true
		}
	}
	what := fmt.Sprintf("%s spd-index %d", StateIPsecSPD, bound[0])
	if !found {
		s.rep.Unclearable = append(s.rep.Unclearable, what+" (no SPD exists to name in the unbind)")
		return nil
	}
	if _, err := svc.IpsecInterfaceAddDelSpd(s.ctx, &ipsecapi.IpsecInterfaceAddDelSpd{IsAdd: false, SwIfIndex: s.idx, SpdID: spdID}); err != nil {
		return fmt.Errorf("ipsec_interface_add_del_spd (unbind stale %s): %w", what, err)
	}
	s.rep.Cleared = append(s.rep.Cleared, what)
	return nil
}

// activeFeatures are the classify features that read a per-interface table index on the
// packet path. After the unbinds none of them may be on: if one is, its table is gone and a
// packet would crash VPP.
var activeFeatures = []struct{ arc, feature string }{
	{"ip4-unicast", "ip4-inacl"}, {"ip6-unicast", "ip6-inacl"},
	{"ip4-output", "ip4-outacl"}, {"ip6-output", "ip6-outacl"},
	{"ip4-unicast", "ip4-policer-classify"}, {"ip6-unicast", "ip6-policer-classify"},
	{"ip4-unicast", "ip4-flow-classify"}, {"ip6-unicast", "ip6-flow-classify"},
}

func (s *sanitizer) activeCheck() error {
	var active []string
	for _, f := range activeFeatures {
		on, _, err := s.featureOn(f.arc, f.feature)
		if err != nil {
			return err
		}
		if on {
			active = append(active, f.arc+"/"+f.feature)
		}
	}
	if len(active) > 0 {
		return fmt.Errorf("%w: %v still enabled after unbinding every live table", ErrActiveStaleBinding, active)
	}
	return nil
}
