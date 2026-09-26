package desired

// F-qos-flat: the `services.qos` part of the `services` domain — flat QoS on VPP's policer and qos plugins, through
// DF-7's descriptors (D-104: used, not rebuilt). Hierarchical QoS and per-interface queues are V3 (excluded).
//
//	services.qos.policers.<n>              → policer.policer/<n>          VPP name "<owner>:<n>"
//	services.qos.shapers.<n>               → policer.policer/shaper:<n>   the "shaper" is an egress POLICER (V3): 1r2c,
//	                                                                      cir = rateKbps, cb = burstBytes or ≈ 10 ms of
//	                                                                      traffic (at least 3000 B), exceed → drop —
//	                                                                      drop-based rate limiting, not queueing
//	services.qos.maps.<n>                  → qos.egress-map/<id>          id = maps.<n>.id, else the lowest free id of
//	                                                                      the agent's id range, in name order (D-071)
//	services.qos.interfaces.<if>
//	    .policer.input / .policer.output   → policer.interface/<if>/input|output   write-only (D-063): applied once per
//	    .shaper                            → policer.interface/<if>/output         VPP boot identity (D-076/D-080), never
//	                                         {policer: shaper:<n>}                 un-applied unless applied in this VPP
//	                                                                               lifetime; not reported by Retrieve
//	    .record                            → qos.record/<if>/<source>
//	    .store                             → qos.store/<if>/ip            (VPP 26.06: ip only)
//	    .mark                              → qos.mark/<if>/<output> {map: id}      depends on the map, so the
//	                                                                               scheduler removes marks before maps
//	descriptions, map names and ids, …     → qos.meta/services.qos        agent-local record (D-073b): VPP numbers
//	                                                                      the maps and keeps no descriptions
//
// Interfaces are named by their logical name (D-069) and every per-interface object depends on the interface's
// alias key `interface/<name>`. Retrieve (AssembleQoS) reports what VPP has — policers, shapers, maps, records,
// stores and marks — named and decorated through the qos.meta record; the write-only attachments are left unset
// (contract §5), and DryRun marks them `agent.write-only` so the drift view can skip them.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/policer"
	"ngfw/agent/internal/descriptors/qos"
	"ngfw/agent/internal/scheduler"
)

func init() { ServicesImplemented["qos"] = true }

// ShaperPrefix prefixes the VPP policer that realises a shaper ("shaper:<name>"): policer names are objectNames
// (no ':'), so a shaper can never collide with a policer.
const ShaperPrefix = "shaper:"

// Minimum derived shaper burst: two full-size Ethernet frames. A bucket smaller than one frame would drop every
// full-size packet (a 1r2c policer conforms a packet only when the bucket holds its whole length).
const shaperMinBurst = 3000

// ShaperBurstBytes is the burst of a shaper without burstBytes: ≈ 10 ms of traffic at rateKbps
// (rateKbps × 1000 / 8 × 0.010 = rateKbps × 1.25 bytes), at least 3000 bytes.
func ShaperBurstBytes(rateKbps uint32) uint64 {
	b := (uint64(rateKbps)*5 + 3) / 4
	if b < shaperMinBurst {
		return shaperMinBurst
	}
	return b
}

// Rule ids of the projection's findings (the schema's where the schema has the rule).
const (
	ruleQosReferences  = "services.qos-references"
	ruleQosConsistency = "services.qos-consistency"
	ruleQosStoreSource = "services.qos-flat-store-source"
	ruleQosPolicer     = "services.qos-flat-policer"
	ruleQosEgress      = "services.qos-flat-egress"
	ruleQosMapID       = "services.qos-flat-map-id"
	ruleWriteOnly      = "agent.write-only"
)

var (
	qosIDsMu sync.Mutex
	qosIDs   *df7.IDRange // nil = every id (VRX_VPP_ID_RANGE=all)
)

// SetQoSMapIDRange sets the egress map id range the projection allocates from and accepts (the agent's id range,
// subsystems' F-qos-flat wiring: Wiring.IDRange → df7): nil = every id, an empty range = none.
func SetQoSMapIDRange(r *df7.IDRange) {
	qosIDsMu.Lock()
	defer qosIDsMu.Unlock()
	if r != nil {
		c := *r
		r = &c
	}
	qosIDs = r
}

func qosMapIDRange() *df7.IDRange {
	qosIDsMu.Lock()
	defer qosIDsMu.Unlock()
	return qosIDs
}

// Configuration spellings ↔ DF-7 policer types.
var qosPolicerTypes = map[string]string{
	"1r2c":         policer.Type1R2C,
	"1r3c-rfc2697": policer.Type1R3C,
	"2r3c-rfc2698": policer.Type2R3C2698,
	"2r3c-rfc4115": policer.Type2R3C4115,
	"2r3c-mef5cf1": policer.Type2R3CMef5CF1,
}

// QosPolicerTypeName is the configuration spelling of a DF-7 policer type ("" when unknown).
func QosPolicerTypeName(t string) string {
	for k, v := range qosPolicerTypes {
		if v == t {
			return k
		}
	}
	return ""
}

func twoRate(t string) bool     { return strings.HasPrefix(t, "2r3c") }
func threeColour(t string) bool { return t != "1r2c" }

var qosSources = map[string]bool{qos.SourceExt: true, qos.SourceVLAN: true, qos.SourceMPLS: true, qos.SourceIP: true}

// qosSourceOrder is the order of the rows of an egress map (qos.EgressMap and the document's `rows`).
var qosSourceOrder = []string{qos.SourceExt, qos.SourceVLAN, qos.SourceMPLS, qos.SourceIP}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func qosAction(a *vrxv1.QosPolicerAction, def string) policer.Action {
	out := policer.Action{Type: orDefault(a.GetAction(), def)}
	if out.Type == policer.ActMark {
		out.DSCP = uint8(a.GetDscp()) //nolint:gosec // G115: the schema bounds dscp to 0–63; Validate re-checks
	}
	return out
}

// qosPolicerSpec is the DF-7 spec of services.qos.policers.<name> (the schema's defaults for absent leaves).
func qosPolicerSpec(name string, p *vrxv1.QosPolicer) (policer.Policer, error) {
	typ := orDefault(p.GetType(), "1r2c")
	t, ok := qosPolicerTypes[typ]
	if !ok {
		return policer.Policer{}, fmt.Errorf("unknown algorithm %q", typ)
	}
	return policer.Policer{
		Name: name, CIR: p.GetCir(), EIR: p.GetEir(), CB: p.GetCb(), EB: p.GetEb(),
		RateType: orDefault(p.GetRateUnit(), policer.RateKbps), RoundType: orDefault(p.GetRound(), policer.RoundClosest),
		Type: t, ColorAware: p.GetColorAware(),
		Conform: qosAction(p.GetConformAction(), policer.ActTransmit),
		Exceed:  qosAction(p.GetExceedAction(), policer.ActDrop),
		Violate: qosAction(p.GetViolateAction(), policer.ActDrop),
	}, nil
}

// QosShaperSpec is the egress policer that realises services.qos.shapers.<name> (V3).
func QosShaperSpec(name string, sh *vrxv1.QosShaper) policer.Policer {
	burst := sh.GetBurstBytes()
	if sh.BurstBytes == nil {
		burst = ShaperBurstBytes(sh.GetRateKbps())
	}
	return policer.Policer{
		Name: ShaperPrefix + name, CIR: sh.GetRateKbps(), CB: burst,
		RateType: policer.RateKbps, RoundType: policer.RoundClosest, Type: policer.Type1R2C,
		Conform: policer.Action{Type: policer.ActTransmit},
		Exceed:  policer.Action{Type: policer.ActDrop},
		Violate: policer.Action{Type: policer.ActDrop},
	}
}

// qosMapIDs assigns every map its egress map id: the document's id, else the lowest free id of the range in name
// order. Findings go to s; a map without a valid id is left out of the result.
func qosMapIDs(s Sink, maps map[string]*vrxv1.QosMap) map[string]uint32 {
	r := qosMapIDRange()
	out := map[string]uint32{}
	used := map[uint32]string{}
	for _, name := range sortedKeys(maps) {
		m := maps[name]
		if m.Id == nil {
			continue
		}
		pt := Ptr("services", "qos", "maps", name, "id")
		switch id := m.GetId(); {
		case !r.Owns(id):
			s.Errorf(pt, ruleQosMapID, "egress map id %d is outside this agent's id range %d–%d (D-071)", id, r.Lo, r.Hi)
		case used[id] != "":
			s.Errorf(pt, ruleQosConsistency, "map id %d is already used by map %q", id, used[id])
		default:
			used[id] = name
			out[name] = id
		}
	}
	next := uint32(1)
	if r != nil {
		next = r.Lo
	}
	for _, name := range sortedKeys(maps) {
		if maps[name].Id != nil {
			continue
		}
		for used[next] != "" && r.Owns(next) && next != ^uint32(0) {
			next++
		}
		if !r.Owns(next) || used[next] != "" {
			s.Errorf(Ptr("services", "qos", "maps", name), ruleQosMapID, "no free egress map id left in this agent's id range; set maps.%s.id", name)
			continue
		}
		used[next] = name
		out[name] = next
	}
	return out
}

// qosEgressMap is the DF-7 spec of one map: per source a 256-entry row (unlisted values 0; an all-zero row omitted).
func qosEgressMap(id uint32, m *vrxv1.QosMap) qos.EgressMap {
	out := qos.EgressMap{ID: id}
	rows := m.GetRows()
	for _, src := range qosSourceOrder {
		var entries []*vrxv1.QosMapEntry
		switch src {
		case qos.SourceExt:
			entries = rows.GetExt()
		case qos.SourceVLAN:
			entries = rows.GetVlan()
		case qos.SourceMPLS:
			entries = rows.GetMpls()
		case qos.SourceIP:
			entries = rows.GetIp()
		}
		row := make([]int, 256)
		nonZero := false
		for _, e := range entries {
			if e.GetFrom() > 255 {
				continue // the schema bounds it; never index out of the row
			}
			row[e.GetFrom()] = int(e.GetTo())
			if e.GetTo() != 0 {
				nonZero = true
			}
		}
		if !nonZero {
			continue
		}
		switch src {
		case qos.SourceExt:
			out.Ext = row
		case qos.SourceVLAN:
			out.VLAN = row
		case qos.SourceMPLS:
			out.MPLS = row
		case qos.SourceIP:
			out.IP = row
		}
	}
	return out
}

func qosRowFroms(rows *vrxv1.QosMap_Rows) map[string][]int {
	out := map[string][]int{}
	add := func(src string, entries []*vrxv1.QosMapEntry) {
		for _, e := range entries {
			out[src] = append(out[src], int(e.GetFrom()))
		}
	}
	add(qos.SourceExt, rows.GetExt())
	add(qos.SourceVLAN, rows.GetVlan())
	add(qos.SourceMPLS, rows.GetMpls())
	add(qos.SourceIP, rows.GetIp())
	if len(out) == 0 {
		return nil
	}
	return out
}

// QoS projects services.qos (nil: nothing) onto the policer / qos objects and the qos.meta record.
func QoS(s Sink, q *vrxv1.QosService) {
	if q == nil {
		return
	}
	meta := qos.DocMeta{}
	for _, name := range sortedKeys(q.GetPolicers()) {
		p := q.GetPolicers()[name]
		pt := Ptr("services", "qos", "policers", name)
		spec, err := qosPolicerSpec(name, p)
		if err == nil {
			err = spec.Validate()
		}
		if err != nil {
			s.Errorf(pt, ruleQosPolicer, "policer %q: %v", name, err)
			continue
		}
		s.Add(policer.KeyPolicer(name), df7.Encode(spec), pt)
		if d := p.GetDescription(); d != "" {
			if meta.Policers == nil {
				meta.Policers = map[string]string{}
			}
			meta.Policers[name] = d
		}
	}
	for _, name := range sortedKeys(q.GetShapers()) {
		sh := q.GetShapers()[name]
		pt := Ptr("services", "qos", "shapers", name)
		spec := QosShaperSpec(name, sh)
		if err := spec.Validate(); err != nil {
			s.Errorf(pt, ruleQosPolicer, "shaper %q (egress policer %q): %v", name, spec.Name, err)
			continue
		}
		s.Add(policer.KeyPolicer(spec.Name), df7.Encode(spec), pt)
		if sh.GetDescription() != "" || sh.BurstBytes != nil {
			if meta.Shapers == nil {
				meta.Shapers = map[string]qos.ShaperMeta{}
			}
			meta.Shapers[name] = qos.ShaperMeta{Description: sh.GetDescription(), Burst: sh.BurstBytes != nil}
		}
	}
	ids := qosMapIDs(s, q.GetMaps())
	for _, name := range sortedKeys(q.GetMaps()) {
		id, ok := ids[name]
		if !ok {
			continue
		}
		m := q.GetMaps()[name]
		spec := qosEgressMap(id, m)
		if err := spec.Validate(); err != nil {
			s.Errorf(Ptr("services", "qos", "maps", name), ruleQosConsistency, "map %q: %v", name, err)
			continue
		}
		s.Add(qos.KeyEgressMap(id), df7.Encode(spec), Ptr("services", "qos", "maps", name))
		if meta.Maps == nil {
			meta.Maps = map[string]qos.MapMeta{}
		}
		meta.Maps[name] = qos.MapMeta{ID: id, ExplicitID: m.Id != nil, Description: m.GetDescription(), Rows: qosRowFroms(m.GetRows())}
	}
	for _, ifName := range sortedKeys(q.GetInterfaces()) {
		qosInterface(s, q, ifName, ids)
		if d := q.GetInterfaces()[ifName].GetDescription(); d != "" {
			if meta.Interfaces == nil {
				meta.Interfaces = map[string]string{}
			}
			meta.Interfaces[ifName] = d
		}
	}
	if !meta.Empty() {
		s.Add(qos.KeyMeta(), df7.Encode(meta), Ptr("services", "qos"))
	}
}

func qosInterface(s Sink, q *vrxv1.QosService, ifName string, ids map[string]uint32) {
	a := q.GetInterfaces()[ifName]
	pt := func(leaf ...string) string {
		return Ptr(append([]string{"services", "qos", "interfaces", ifName}, leaf...)...)
	}
	attach := func(pointer, dir, pol string) {
		s.Add(policer.KeyInterface(ifName, dir), df7.Encode(policer.Attachment{Interface: ifName, Direction: dir, Policer: pol}), pointer)
	}
	retrievable := false
	if in := a.GetPolicer().GetInput(); in != "" {
		if _, ok := q.GetPolicers()[in]; !ok {
			s.Errorf(pt("policer", "input"), ruleQosReferences, "policer %q does not exist in qos.policers", in)
		} else {
			attach(pt("policer", "input"), policer.DirInput, in)
		}
	}
	out, shaper := a.GetPolicer().GetOutput(), a.GetShaper()
	switch {
	case out != "" && shaper != "":
		s.Errorf(pt("shaper"), ruleQosEgress, "shaper and policer.output are exclusive (both are the one egress policer of the interface)")
	case out != "":
		if _, ok := q.GetPolicers()[out]; !ok {
			s.Errorf(pt("policer", "output"), ruleQosReferences, "policer %q does not exist in qos.policers", out)
		} else {
			attach(pt("policer", "output"), policer.DirOutput, out)
		}
	case shaper != "":
		if _, ok := q.GetShapers()[shaper]; !ok {
			s.Errorf(pt("shaper"), ruleQosReferences, "shaper %q does not exist in qos.shapers", shaper)
		} else {
			attach(pt("shaper"), policer.DirOutput, ShaperPrefix+shaper)
		}
	}
	if src := a.GetRecord(); src != "" {
		if !qosSources[src] {
			s.Errorf(pt("record"), ruleQosConsistency, "unknown QoS source %q", src)
		} else {
			s.Add(qos.KeyRecord(ifName, src), df7.Encode(qos.Record{Interface: ifName, Source: src}), pt("record"))
			retrievable = true
		}
	}
	if st := a.GetStore(); st != nil {
		src := orDefault(st.GetSource(), qos.SourceIP)
		switch {
		case src != qos.SourceIP:
			s.Errorf(pt("store", "source"), ruleQosStoreSource, "VPP 26.06 stores a QoS value for the ip source only (qos store %s is not implemented)", src)
		case st.GetValue() > 63:
			s.Errorf(pt("store", "value"), ruleQosConsistency, "ip values are 0–63")
		default:
			s.Add(qos.KeyStore(ifName, src), df7.Encode(qos.Store{Interface: ifName, Source: src, Value: uint8(st.GetValue())}), pt("store")) //nolint:gosec // G115: bounded above
			retrievable = true
		}
	}
	if mk := a.GetMark(); mk != nil {
		src := mk.GetOutput()
		id, ok := ids[mk.GetMap()]
		switch {
		case !qosSources[src]:
			s.Errorf(pt("mark", "output"), ruleQosConsistency, "unknown QoS source %q", src)
		case !ok:
			if _, exists := q.GetMaps()[mk.GetMap()]; !exists {
				s.Errorf(pt("mark", "map"), ruleQosReferences, "map %q does not exist in qos.maps", mk.GetMap())
			} // else the map's own finding explains why it has no id
		default:
			s.Add(qos.KeyMark(ifName, src), df7.Encode(qos.Mark{Interface: ifName, Source: src, Map: id}), pt("mark"))
			retrievable = true
		}
	}
	// write-only leaves (D-063): applied, but VPP cannot report them — marked so the drift view skips them
	writeOnly := a.GetPolicer().GetInput() != "" || out != "" || shaper != ""
	switch {
	case writeOnly && !retrievable:
		s.Warnf(pt(), ruleWriteOnly, "the QoS attachment of %s is applied but VPP cannot report policer attachments (policer_input/output have no dump, D-063); Retrieve does not show it", ifName)
	case writeOnly:
		if a.GetPolicer() != nil {
			s.Warnf(pt("policer"), ruleWriteOnly, "policer attachments are applied but VPP cannot report them (no dump, D-063)")
		}
		if shaper != "" {
			s.Warnf(pt("shaper"), ruleWriteOnly, "the shaper attachment (an egress policer) is applied but VPP cannot report it (no dump, D-063)")
		}
	}
}

// qosOf returns ds.services.qos, allocated.
func qosOf(ds *vrxv1.DesiredState) *vrxv1.QosService {
	if ds.Services == nil {
		ds.Services = &vrxv1.ServicesConfig{}
	}
	if ds.Services.Qos == nil {
		ds.Services.Qos = &vrxv1.QosService{}
	}
	return ds.Services.Qos
}

func qosIface(q *vrxv1.QosService, name string) *vrxv1.QosInterface {
	if q.Interfaces == nil {
		q.Interfaces = map[string]*vrxv1.QosInterface{}
	}
	a, ok := q.Interfaces[name]
	if !ok {
		a = &vrxv1.QosInterface{}
		q.Interfaces[name] = a
	}
	return a
}

func qosActionOut(a policer.Action) *vrxv1.QosPolicerAction {
	out := &vrxv1.QosPolicerAction{Action: proto.String(a.Type)}
	if a.Type == policer.ActMark {
		out.Dscp = proto.Uint32(uint32(a.DSCP))
	}
	return out
}

// AssembleQoS adds services.qos of the retrieved objects to ds: `services.qos` is always present once the domain
// is assembled (an empty one when VPP has nothing of ours). Maps are named, and descriptions and explicit ids
// restored, from the qos.meta record; a map id the record does not know is reported as "map-<id>" with its id.
func AssembleQoS(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	q := qosOf(ds)
	meta, _ := qos.DocMetaOf(kvs)
	mapName := func(id uint32) (string, bool) {
		if n, _, ok := meta.MapByID(id); ok {
			return n, true
		}
		return "map-" + strconv.FormatUint(uint64(id), 10), false
	}
	sorted := append([]scheduler.KV(nil), kvs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	for _, kv := range sorted {
		switch kv.Key.Descriptor() {
		case policer.NamePolicer:
			p, err := df7.Decode[policer.Policer](kv.Value)
			if err != nil {
				continue
			}
			if name, ok := strings.CutPrefix(p.Name, ShaperPrefix); ok {
				sh := &vrxv1.QosShaper{RateKbps: proto.Uint32(p.CIR)}
				sm := meta.Shapers[name]
				if sm.Burst || p.CB != ShaperBurstBytes(p.CIR) {
					sh.BurstBytes = proto.Uint64(p.CB)
				}
				if sm.Description != "" {
					sh.Description = proto.String(sm.Description)
				}
				if q.Shapers == nil {
					q.Shapers = map[string]*vrxv1.QosShaper{}
				}
				q.Shapers[name] = sh
				continue
			}
			typ := QosPolicerTypeName(p.Type)
			out := &vrxv1.QosPolicer{
				Type: proto.String(typ), RateUnit: proto.String(p.RateType), Cir: proto.Uint32(p.CIR), Cb: proto.Uint64(p.CB),
				Round: proto.String(p.RoundType), ColorAware: proto.Bool(p.ColorAware),
				ConformAction: qosActionOut(p.Conform), ExceedAction: qosActionOut(p.Exceed), ViolateAction: qosActionOut(p.Violate),
			}
			if twoRate(typ) || p.EIR != 0 {
				out.Eir = proto.Uint32(p.EIR)
			}
			if threeColour(typ) || p.EB != 0 {
				out.Eb = proto.Uint64(p.EB)
			}
			if d := meta.Policers[p.Name]; d != "" {
				out.Description = proto.String(d)
			}
			if q.Policers == nil {
				q.Policers = map[string]*vrxv1.QosPolicer{}
			}
			q.Policers[p.Name] = out
		case qos.NameEgressMap:
			m, err := df7.Decode[qos.EgressMap](kv.Value)
			if err != nil {
				continue
			}
			name, known := mapName(m.ID)
			mm := meta.Maps[name]
			out := &vrxv1.QosMap{Rows: &vrxv1.QosMap_Rows{
				Ext: qosRowOut(m.Ext, mm.Rows[qos.SourceExt]), Vlan: qosRowOut(m.VLAN, mm.Rows[qos.SourceVLAN]),
				Mpls: qosRowOut(m.MPLS, mm.Rows[qos.SourceMPLS]), Ip: qosRowOut(m.IP, mm.Rows[qos.SourceIP]),
			}}
			if !known || mm.ExplicitID {
				out.Id = proto.Uint32(m.ID)
			}
			if mm.Description != "" {
				out.Description = proto.String(mm.Description)
			}
			if q.Maps == nil {
				q.Maps = map[string]*vrxv1.QosMap{}
			}
			q.Maps[name] = out
		case policer.NameInterface: // write-only: present only when kvs are a desired state (never in a Retrieve)
			a, err := df7.Decode[policer.Attachment](kv.Value)
			if err != nil {
				continue
			}
			itf := qosIface(q, a.Interface)
			switch shaper, isShaper := strings.CutPrefix(a.Policer, ShaperPrefix); {
			case a.Direction == policer.DirOutput && isShaper:
				itf.Shaper = proto.String(shaper)
			case a.Direction == policer.DirOutput:
				qosPol(itf).Output = proto.String(a.Policer)
			default:
				qosPol(itf).Input = proto.String(a.Policer)
			}
		case qos.NameRecord:
			r, err := df7.Decode[qos.Record](kv.Value)
			if err != nil {
				continue
			}
			if itf := qosIface(q, r.Interface); itf.Record == nil {
				itf.Record = proto.String(r.Source)
			}
		case qos.NameStore:
			st, err := df7.Decode[qos.Store](kv.Value)
			if err != nil {
				continue
			}
			qosIface(q, st.Interface).Store = &vrxv1.QosInterface_Store{Source: proto.String(st.Source), Value: proto.Uint32(uint32(st.Value))}
		case qos.NameMark:
			mk, err := df7.Decode[qos.Mark](kv.Value)
			if err != nil {
				continue
			}
			name, _ := mapName(mk.Map)
			qosIface(q, mk.Interface).Mark = &vrxv1.QosInterface_Mark{Map: proto.String(name), Output: proto.String(mk.Source)}
		}
	}
	for name, itf := range q.GetInterfaces() {
		if d := meta.Interfaces[name]; d != "" {
			itf.Description = proto.String(d)
		}
	}
}

func qosPol(itf *vrxv1.QosInterface) *vrxv1.QosInterface_Policer {
	if itf.Policer == nil {
		itf.Policer = &vrxv1.QosInterface_Policer{}
	}
	return itf.Policer
}

// qosRowOut rebuilds a row's entries from VPP's 256 outputs: the recorded values the document listed first, in its
// order (their outputs as VPP has them, zeros included), then every other non-zero output in value order.
func qosRowOut(row []int, froms []int) []*vrxv1.QosMapEntry {
	var out []*vrxv1.QosMapEntry
	at := func(i int) int {
		if i >= 0 && i < len(row) {
			return row[i]
		}
		return 0
	}
	seen := map[int]bool{}
	for _, f := range froms {
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, &vrxv1.QosMapEntry{From: proto.Uint32(uint32(f)), To: proto.Uint32(uint32(at(f)))}) //nolint:gosec // G115: bytes
	}
	for i, v := range row {
		if v != 0 && !seen[i] {
			out = append(out, &vrxv1.QosMapEntry{From: proto.Uint32(uint32(i)), To: proto.Uint32(uint32(v))}) //nolint:gosec // G115: bytes
		}
	}
	return out
}
