package desired

// F-loopback-bvi-gso-lldp-span: interfaces.<src>.mirror[] ⇄ span.mirror (DF-7, descriptors/span).
//
//	interfaces.<src>.mirror[i] {destination, direction, level}
//	    → span.mirror/<src>/<destination>/<device|l2>   (state rx|tx|both; depends on interface/<src>
//	                                                     and interface/<destination>)
//
// The destination is any interface the agent can name: a monitor port, a loopback, or a GRE tunnel
// of type erspan (ERSPAN; created by F-tunnels or, on the lab host, a DF-6 fixture — only its
// interface/<name> alias is used). The scheduler deletes every session before either interface
// (D-095c; V19/V21: VPP keeps span state of a deleted index). Retrieve reads sw_interface_span_dump
// at both levels; the assembler reports the sessions in the stored document's order (then any
// other session sorted), so an unchanged configuration never shows array drift.

import (
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/span"
	"ngfw/agent/internal/scheduler"
)

// Mirror levels and directions (configuration spelling; the directions equal span's states).
const (
	mirrorLevelDevice = "device"
	mirrorLevelL2     = "l2"
)

var mirrorDirections = map[string]bool{span.StateRx: true, span.StateTx: true, span.StateBoth: true}

// MirrorValue is the span.mirror value of one session (the encoding span.Retrieve reports).
func MirrorValue(src, dst, direction string, l2 bool) proto.Message {
	return df7.Encode(span.Mirror{Source: src, Destination: dst, State: direction, L2: l2})
}

// Mirror emits one span.mirror object per session of every interface in ifs.
func Mirror(s Sink, ifs map[string]*vrxv1.Interface) {
	for _, src := range sortedKeys(ifs) {
		for i, m := range ifs[src].GetMirror() {
			pt := Ptr("interfaces", src, "mirror", strconv.Itoa(i))
			dst, dir, level := m.GetDestination(), m.GetDirection(), m.GetLevel()
			if dir == "" {
				dir = span.StateBoth
			}
			if level == "" {
				level = mirrorLevelDevice
			}
			switch {
			case dst == "" || dst == src:
				s.Errorf(pt+"/destination", "interfaces.loopback-bvi-gso-lldp-span-mirror-destination", "mirror session %d of %s needs a destination other than the source", i, src)
				continue
			case !mirrorDirections[dir]:
				s.Errorf(pt+"/direction", "interfaces.loopback-bvi-gso-lldp-span-mirror-direction", "mirror direction %q is not rx, tx or both", dir)
				continue
			case level != mirrorLevelDevice && level != mirrorLevelL2:
				s.Errorf(pt+"/level", "interfaces.loopback-bvi-gso-lldp-span-mirror-level", "mirror level %q is not device or l2", level)
				continue
			}
			l2 := level == mirrorLevelL2
			s.Add(span.Key(src, dst, l2), MirrorValue(src, dst, dir, l2), pt)
		}
	}
}

// AssembleMirror sets interfaces.<src>.mirror from the retrieved span.mirror objects; stored is the
// agent's stored `interfaces` document (its session order is kept).
func AssembleMirror(ds *vrxv1.DesiredState, kvs []scheduler.KV, stored map[string]*vrxv1.Interface) {
	bySource := map[string][]*vrxv1.MirrorSession{}
	for _, kv := range kvs {
		if kv.Key.Descriptor() != span.NameMirror {
			continue
		}
		m, err := df7.Decode[span.Mirror](kv.Value)
		if err != nil {
			continue
		}
		level := mirrorLevelDevice
		if m.L2 {
			level = mirrorLevelL2
		}
		bySource[m.Source] = append(bySource[m.Source], &vrxv1.MirrorSession{
			Destination: proto.String(m.Destination), Direction: proto.String(m.State), Level: proto.String(level),
		})
	}
	for _, src := range sortedKeys(bySource) {
		if !lookupNode(ds, src, true) {
			continue
		}
		ds.Interfaces[src].Mirror = orderSessions(bySource[src], stored[src].GetMirror())
	}
}

// orderSessions orders got like want (by destination and level), then the rest by destination, level.
func orderSessions(got, want []*vrxv1.MirrorSession) []*vrxv1.MirrorSession {
	rank := map[string]int{}
	for i, w := range want {
		level := w.GetLevel()
		if level == "" {
			level = mirrorLevelDevice
		}
		if _, dup := rank[w.GetDestination()+"\x00"+level]; !dup {
			rank[w.GetDestination()+"\x00"+level] = i
		}
	}
	pos := func(m *vrxv1.MirrorSession) int {
		if r, ok := rank[m.GetDestination()+"\x00"+m.GetLevel()]; ok {
			return r
		}
		return len(want)
	}
	sort.SliceStable(got, func(i, j int) bool {
		pi, pj := pos(got[i]), pos(got[j])
		if pi != pj {
			return pi < pj
		}
		if got[i].GetDestination() != got[j].GetDestination() {
			return got[i].GetDestination() < got[j].GetDestination()
		}
		return got[i].GetLevel() < got[j].GetLevel()
	})
	return got
}
