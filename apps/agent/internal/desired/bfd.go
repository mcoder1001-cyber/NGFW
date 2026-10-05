package desired

import (
	"fmt"
	"hash/fnv"
	"math"
	"strconv"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/bfd"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/renderers/frr/redistribute"
	"ngfw/agent/internal/scheduler"
)

// BfdKeyID is stable over restarts and scoped to the assigned ID range.
func BfdKeyID(ref, typ string, first, last uint32) (uint32, error) {
	if last < first {
		return 0, fmt.Errorf("invalid BFD key ID range")
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(typ + "\x00" + ref))
	id := uint64(first) + uint64(h.Sum32())%(uint64(last)-uint64(first)+1)
	if id > math.MaxUint32 {
		return 0, fmt.Errorf("BFD key ID exceeds uint32")
	}
	return uint32(id), nil
}

// BfdEnvironment supplies scoped identifiers, ownership capability and secret reference lookup.
type BfdEnvironment struct {
	First, Last uint32
	Multihop    bool
	SetRef      func(uint32, string)
	Ref         func(uint32) string
}

// Bfd projects standalone sessions. Authentication is resolved outside Values.
func Bfd(s Sink, ds *ngfwv1.DesiredState, in map[string]bool, env BfdEnvironment) {
	if !in["routing"] {
		return
	}
	edges := redistribute.Edges(ds.GetRouting())
	for _, a := range edges {
		for _, b := range edges {
			if a.Source < b.Source && a.Source == b.Target && a.Target == b.Source && a.Vrf == b.Vrf && a.RouteMap == "" && b.RouteMap == "" {
				s.Warnf(Ptr("routing", a.Target, "redistribute", a.Source), "routing.bfd-redistribution-loop", "redistribution loop %s ↔ %s has no route-map filter", a.Source, a.Target)
			}
		}
	}
	first, last := env.First, env.Last
	keys := map[uint32]string{}
	endpointTuples := map[string]bool{}
	for j, v := range ds.GetRouting().GetBfd().GetSessions() {
		at := Ptr("routing", "bfd", "sessions", strconv.Itoa(j))
		multiplier := v.GetDetectMultiplier()
		wireKeyID := v.GetAuth().GetKeyId()
		if multiplier > 255 {
			s.Errorf(at+"/detectMultiplier", "routing.bfd-multiplier", "detect multiplier must fit in 1–255")
			continue
		}
		if wireKeyID > 255 {
			s.Errorf(at+"/auth/keyId", "routing.bfd-key-id", "wire key ID must fit in 0–255")
			continue
		}
		if v.GetMultihop() && !env.Multihop {
			s.Errorf(at+"/multihop", "routing.bfd-multihop", "multihop requires this agent to be the globals owner")
			continue
		}
		if v.GetMultihop() {
			tuple := v.GetLocalAddress() + "\x00" + v.GetPeerAddress()
			if endpointTuples[tuple] {
				s.Errorf(at, "routing.bfd-endpoint-duplicate", "multihop endpoint tuple must be unique across interfaces")
				continue
			}
			endpointTuples[tuple] = true
		}
		x := bfd.Session{Multihop: v.GetMultihop(), Interface: v.GetInterface(), Local: v.GetLocalAddress(), Peer: v.GetPeerAddress(), DesiredMinTx: v.GetDesiredMinTxUs(), RequiredMinRx: v.GetRequiredMinRxUs(), DetectMult: uint8(multiplier)}
		if v.Enabled != nil {
			x.AdminDown = !v.GetEnabled()
		}
		if x.DesiredMinTx == 0 {
			x.DesiredMinTx = 300000
		}
		if x.RequiredMinRx == 0 {
			x.RequiredMinRx = 300000
		}
		if x.DetectMult == 0 {
			x.DetectMult = 3
		}
		if a := v.GetAuth(); a != nil {
			if last < first {
				s.Errorf(at+"/auth", "routing.bfd-id-range", "authentication requires an assigned key ID range")
				continue
			}
			id, err := BfdKeyID(a.GetKeyRef(), a.GetType(), first, last)
			if err != nil {
				s.Errorf(at+"/auth", "routing.bfd-id-range", "%v", err)
				continue
			}
			identity := a.GetType() + "\x00" + a.GetKeyRef()
			if old, ok := keys[id]; ok && old != identity {
				s.Errorf(at+"/auth", "routing.bfd-key-collision", "BFD key identifiers collide; choose a different reference")
				continue
			}
			keys[id] = identity
			if env.SetRef != nil {
				env.SetRef(id, a.GetKeyRef())
			}
			key := bfd.AuthKey{ID: id, Type: a.GetType()}
			if err := key.Validate(); err != nil {
				s.Errorf(at+"/auth", "routing.bfd-auth", "%v", err)
				continue
			}
			s.Add(bfd.KeyAuthKey(id), df7.Encode(key), at+"/auth")
			x.Auth = &bfd.SessionAuth{ConfKeyID: id, BFDKeyID: uint8(wireKeyID)}
		}
		if err := x.Validate(); err != nil {
			s.Errorf(at, "routing.bfd-session", "%v", err)
			continue
		}
		s.Add(bfd.KeySession(x.Interface, x.Local, x.Peer), df7.Encode(x), at)
	}
}

// AssembleBfd reads timers/admin state from actual VPP objects; secret references are restored only for matching keys.
func AssembleBfd(ds *ngfwv1.DesiredState, kvs []scheduler.KV, stored *ngfwv1.DesiredState, env BfdEnvironment) {
	c := &ngfwv1.BfdConfig{}
	for _, kv := range kvs {
		if kv.Key == FRRConfigKey {
			if doc, status, e := ParseFRRValue(kv.Value); e == nil && status == FRRApplied {
				c.Profiles = doc.GetRouting().GetBfd().GetProfiles()
			}
		}
	}
	for _, kv := range kvs {
		if kv.Key.Descriptor() != bfd.NameSession {
			continue
		}
		x, e := df7.Decode[bfd.Session](kv.Value)
		if e != nil {
			continue
		}
		v := &ngfwv1.BfdSession{Interface: proto.String(x.Interface), LocalAddress: proto.String(x.Local), PeerAddress: proto.String(x.Peer), DesiredMinTxUs: proto.Uint32(x.DesiredMinTx), RequiredMinRxUs: proto.Uint32(x.RequiredMinRx), DetectMultiplier: proto.Uint32(uint32(x.DetectMult)), Enabled: proto.Bool(!x.AdminDown), Multihop: proto.Bool(x.Multihop)}
		if x.Auth != nil {
			if env.Ref != nil {
				if ref := env.Ref(x.Auth.ConfKeyID); ref != "" {
					for _, key := range kvs {
						if key.Key.Descriptor() == bfd.NameAuthKey {
							if a, e := df7.Decode[bfd.AuthKey](key.Value); e == nil && a.ID == x.Auth.ConfKeyID {
								v.Auth = &ngfwv1.BfdAuth{Type: proto.String(a.Type), KeyId: proto.Uint32(uint32(x.Auth.BFDKeyID)), KeyRef: proto.String(ref)}
							}
						}
					}
				}
			}
			for _, s := range stored.GetRouting().GetBfd().GetSessions() {
				if s.GetInterface() == x.Interface && s.GetLocalAddress() == x.Local && s.GetPeerAddress() == x.Peer && s.GetAuth() != nil {
					v.Auth = proto.Clone(s.Auth).(*ngfwv1.BfdAuth)
					break
				}
			}
		}
		c.Sessions = append(c.Sessions, v)
	}
	if len(c.Sessions) > 0 || len(c.Profiles) > 0 || stored.GetRouting().GetBfd() != nil {
		if ds.Routing == nil {
			ds.Routing = &ngfwv1.RoutingConfig{}
		}
		if b := stored.GetRouting().GetBfd(); b != nil {
			c.Profiles = b.Profiles
		}
		ds.Routing.Bfd = c
	}
}
