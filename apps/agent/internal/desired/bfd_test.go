package desired

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/bfd"
	"ngfw/agent/internal/descriptors/df7"
	"testing"
)

func TestBfdAuthenticationProjectionAndRetrieve(t *testing.T) {
	ds := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Bfd: &ngfwv1.BfdConfig{Sessions: []*ngfwv1.BfdSession{{Interface: proto.String("loop0"), LocalAddress: proto.String("10.14.1.1"), PeerAddress: proto.String("10.14.1.2"), Auth: &ngfwv1.BfdAuth{Type: proto.String("keyed-sha1"), KeyId: proto.Uint32(17), KeyRef: proto.String("key/test")}}}}}}
	refs := map[uint32]string{}
	env := BfdEnvironment{First: 14000, Last: 14999, SetRef: func(id uint32, ref string) { refs[id] = ref }, Ref: func(id uint32) string { return refs[id] }}
	sink := newRecSink()
	Bfd(sink, ds, map[string]bool{"routing": true}, env)
	if len(sink.kvs) != 2 || len(sink.issues) != 0 {
		t.Fatalf("%v %v", sink.kvs, sink.issues)
	}
	key, e := df7.Decode[bfd.AuthKey](sink.kvs[0].Value)
	if e != nil || key.ID < 14000 || key.ID > 14999 {
		t.Fatalf("key %v %v", key, e)
	}
	out := &ngfwv1.DesiredState{}
	AssembleBfd(out, sink.kvs, nil, env)
	auth := out.GetRouting().GetBfd().GetSessions()[0].GetAuth()
	if auth.GetKeyRef() != "key/test" || auth.GetKeyId() != 17 {
		t.Fatalf("auth %v", auth)
	}
	ds.Routing.Bfd.Sessions[0].Auth.KeyId = proto.Uint32(300)
	bad := newRecSink()
	Bfd(bad, ds, map[string]bool{"routing": true}, env)
	if len(bad.kvs) != 0 || len(bad.issues) == 0 {
		t.Fatal("oversize wire ID accepted")
	}
}
func TestBfdRejectsMultihopCapability(t *testing.T) {
	ds := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Bfd: &ngfwv1.BfdConfig{Sessions: []*ngfwv1.BfdSession{{Multihop: proto.Bool(true)}}}}}
	s := newRecSink()
	Bfd(s, ds, map[string]bool{"routing": true}, BfdEnvironment{})
	if len(s.kvs) != 0 || len(s.issues) != 1 {
		t.Fatalf("%v %v", s.kvs, s.issues)
	}
}

func TestBfdKeyIDInclusiveRangeBoundaries(t *testing.T) {
	for _, c := range []struct{ first, last, want uint32 }{
		{14000, 14999, 14215}, {0, ^uint32(0), 2849723215},
		{^uint32(0) - 1, ^uint32(0), ^uint32(0)}, {17, 17, 17}, {0, 0, 0},
	} {
		got, err := BfdKeyID("key/test", "keyed-sha1", c.first, c.last)
		if err != nil || got != c.want {
			t.Fatalf("range %d..%d: got %d, %v; want %d", c.first, c.last, got, err, c.want)
		}
	}
	if _, err := BfdKeyID("key/test", "keyed-sha1", 2, 1); err == nil {
		t.Fatal("reversed range accepted")
	}
}

func TestBfdProjectionByteBoundsAndDefaults(t *testing.T) {
	for _, c := range []struct {
		name              string
		multiplier, keyID uint32
		auth              bool
		rejected          bool
		wantMultiplier    uint8
	}{
		{"nil-default", 0, 0, false, false, 3}, {"auth-default-zero-id", 0, 0, true, false, 3},
		{"byte-max", 255, 255, true, false, 255}, {"multiplier-overflow", 256, 0, false, true, 0},
		{"multiplier-max32", ^uint32(0), 0, false, true, 0}, {"key-overflow", 3, 256, true, true, 0},
		{"key-max32", 3, ^uint32(0), true, true, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := &ngfwv1.BfdSession{Interface: proto.String("loop0"), LocalAddress: proto.String("10.14.1.1"), PeerAddress: proto.String("10.14.1.2"), DetectMultiplier: proto.Uint32(c.multiplier)}
			if c.auth {
				v.Auth = &ngfwv1.BfdAuth{Type: proto.String("keyed-sha1"), KeyRef: proto.String("key/test"), KeyId: proto.Uint32(c.keyID)}
			}
			ds := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Bfd: &ngfwv1.BfdConfig{Sessions: []*ngfwv1.BfdSession{v}}}}
			sink := newRecSink()
			Bfd(sink, ds, map[string]bool{"routing": true}, BfdEnvironment{First: 14000, Last: 14999})
			if c.rejected {
				if len(sink.kvs) != 0 || len(sink.issues) == 0 {
					t.Fatalf("unsafe projection %v %v", sink.kvs, sink.issues)
				}
				return
			}
			if len(sink.issues) != 0 {
				t.Fatal(sink.issues)
			}
			got, err := df7.Decode[bfd.Session](sink.kvs[len(sink.kvs)-1].Value)
			if err != nil || got.DetectMult != c.wantMultiplier {
				t.Fatalf("session %v %v", got, err)
			}
			if c.auth && (got.Auth == nil || uint32(got.Auth.BFDKeyID) != c.keyID) {
				t.Fatalf("wire ID changed: %v", got.Auth)
			}
			invalid := newRecSink()
			if c.auth {
				Bfd(invalid, ds, map[string]bool{"routing": true}, BfdEnvironment{First: 2, Last: 1})
				if len(invalid.kvs) != 0 || len(invalid.issues) == 0 {
					t.Fatal("invalid ID range emitted objects")
				}
			}
		})
	}
}
