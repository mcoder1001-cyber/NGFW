package desired

// F-lb projection: services.lb → lb.conf / lb.vip / lb.as / lb.intf-nat (DF-7 specs), globals-owner gating (D-071),
// the write-only note (D-063), the sentinel range (D-090) and the unsupported services members (merge seam).

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/lb"
	"ngfw/agent/internal/scheduler"
)

type lbSink struct {
	kvs   map[scheduler.Key]proto.Message
	ptrs  map[scheduler.Key]string
	notes []string
}

func newLbSink() *lbSink {
	return &lbSink{kvs: map[scheduler.Key]proto.Message{}, ptrs: map[scheduler.Key]string{}}
}

func (s *lbSink) Add(k scheduler.Key, v proto.Message, p string) { s.kvs[k] = v; s.ptrs[k] = p }
func (s *lbSink) Errorf(p, rule, f string, a ...any) {
	s.notes = append(s.notes, "E "+p+" "+rule+" "+fmt.Sprintf(f, a...))
}
func (s *lbSink) Warnf(p, rule, _ string, _ ...any) { s.notes = append(s.notes, "W "+p+" "+rule) }

func (s *lbSink) keys() []string {
	out := make([]string, 0, len(s.kvs))
	for k := range s.kvs {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

func lbServices(t *testing.T, js string) *vrxv1.ServicesConfig {
	t.Helper()
	svc := &vrxv1.ServicesConfig{}
	if err := protojson.Unmarshal([]byte(js), svc); err != nil {
		t.Fatal(err)
	}
	return svc
}

const lbServicesJSON = `{"lb": {
  "settings": {"ip4Source": "10.2.1.1", "flowBuckets": 2048},
  "vips": {
    "web": {"prefix": "10.2.250.1/32", "protocol": "tcp", "port": 80, "encap": "gre4", "newFlowsTableLength": 1024,
            "servers": [{"address": "10.2.2.10"}, {"address": "10.2.2.11", "flushOnDelete": true}]},
    "v6": {"prefix": "2001:DB8:2:250::1/128", "protocol": "udp", "port": 53, "encap": "nat6", "targetPort": 5353,
           "servers": [{"address": "2001:db8:2:2:0::10"}]}
  },
  "natInterfaces": [{"interface": "loop0", "family": "ip6"}]
}}`

func TestLbProjection(t *testing.T) {
	for _, owner := range []bool{false, true} {
		s := newLbSink()
		Lb(s, lbServices(t, lbServicesJSON), LbEnv{GlobalsOwner: owner})
		want := []string{
			"lb.as/10.2.250.1/32/tcp/80/10.2.2.10",
			"lb.as/10.2.250.1/32/tcp/80/10.2.2.11",
			"lb.as/2001:db8:2:250::1/128/udp/53/2001:db8:2:2::10",
			"lb.intf-nat/loop0/ip6",
			"lb.vip/10.2.250.1/32/tcp/80",
			"lb.vip/2001:db8:2:250::1/128/udp/53",
		}
		wantNotes := []string{"W /services/lb/settings agent.unsupported-field", "W /services/lb agent.write-only"}
		if owner {
			want = append(want, "lb.conf/global")
			sort.Strings(want)
			wantNotes = wantNotes[1:]
		}
		if got := s.keys(); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("owner=%v keys\n got %v\nwant %v", owner, got, want)
		}
		if strings.Join(s.notes, ",") != strings.Join(wantNotes, ",") {
			t.Fatalf("owner=%v notes %v", owner, s.notes)
		}
		if owner {
			c, err := df7.Decode[lb.Conf](s.kvs[lb.KeyConf()])
			if err != nil || c.IP4Src != "10.2.1.1" || c.StickyBucketsPerCore != 2048 || c.FlowTimeout != 0 || s.ptrs[lb.KeyConf()] != "/services/lb/settings" {
				t.Fatalf("conf %+v %v", c, err)
			}
		}
		v6, err := df7.Decode[lb.VIPSpec](s.kvs["lb.vip/2001:db8:2:250::1/128/udp/53"])
		if err != nil || v6.Encap != lb.EncapNAT6 || v6.SrvType != lb.SrvClusterIP || v6.TargetPort != 5353 || v6.NewFlowsTableLength != 1024 {
			t.Fatalf("nat6 VIP defaults %+v %v", v6, err)
		}
		as, _ := df7.Decode[lb.AS](s.kvs["lb.as/10.2.250.1/32/tcp/80/10.2.2.11"])
		if !as.FlushOnDelete || s.ptrs["lb.as/10.2.250.1/32/tcp/80/10.2.2.11"] != "/services/lb/vips/web/servers/1" {
			t.Fatalf("as %+v", as)
		}
	}
}

func TestLbProjectionErrors(t *testing.T) {
	s := newLbSink()
	Lb(s, lbServices(t, `{"lb": {"vips": {
	  "gc": {"prefix": "0.0.0.0/32", "encap": "gre4"},
	  "all": {"prefix": "0.0.0.0/0", "encap": "gre4"},
	  "hostbits": {"prefix": "10.2.250.1/24", "encap": "gre4"},
	  "badport": {"prefix": "10.2.250.9/32", "protocol": "any", "port": 80, "encap": "gre4"},
	  "fam": {"prefix": "10.2.250.8/32", "encap": "gre4", "servers": [{"address": "not-an-ip"}]}
	}}}`), LbEnv{})
	got := strings.Join(s.notes, "\n")
	for _, want := range []string{
		"E /services/lb/vips/gc/prefix services.lb-vip-reserved",
		"E /services/lb/vips/all/prefix services.lb-vip-reserved",
		"E /services/lb/vips/hostbits services.lb-spec",
		"E /services/lb/vips/badport services.lb-spec",
		"E /services/lb/vips/fam/servers/0/address services.lb-spec",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if _, ok := s.kvs["lb.vip/10.2.250.8/32/any/0"]; !ok || len(s.kvs) != 1 {
		t.Fatalf("keys %v", s.keys())
	}
}

// Merge seam: every other non-empty services member is reported as not implemented; empty members and lb are not.
func TestLbUnsupportedServicesMembers(t *testing.T) {
	s := newLbSink()
	Lb(s, lbServices(t, `{"snmp": {"enabled": true}, "ntp": {}, "lb": {"vips": {}}}`), LbEnv{})
	if strings.Join(s.notes, ",") != "W /services/snmp agent.unsupported-field" {
		t.Fatalf("notes %v", s.notes)
	}
	s = newLbSink()
	Lb(s, nil, LbEnv{})
	if len(s.notes) != 0 || len(s.kvs) != 0 {
		t.Fatalf("nil services: %v %v", s.notes, s.keys())
	}
}
