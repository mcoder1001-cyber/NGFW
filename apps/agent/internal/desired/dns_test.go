package desired

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	dnsd "ngfw/agent/internal/descriptors/dns"
	"ngfw/agent/internal/renderers/chrony"
	"ngfw/agent/internal/renderers/rsyslog"
	"ngfw/agent/internal/renderers/unbound"
	"ngfw/agent/internal/scheduler"
)

// F-unbound-chrony-syslog: the services/management projection and its assembler.

type recSink struct {
	kvs    []scheduler.KV
	ptrs   map[scheduler.Key]string
	issues []string // "<E|W> <pointer> <rule>"
}

func (r *recSink) Add(k scheduler.Key, v proto.Message, pointer string) {
	r.kvs = append(r.kvs, scheduler.KV{Key: k, Value: v})
	if r.ptrs == nil {
		r.ptrs = map[scheduler.Key]string{}
	}
	r.ptrs[k] = pointer
}

func (r *recSink) Errorf(pointer, rule, _ string, _ ...any) {
	r.issues = append(r.issues, fmt.Sprintf("E %s %s", pointer, rule))
}

func (r *recSink) Warnf(pointer, rule, _ string, _ ...any) {
	r.issues = append(r.issues, fmt.Sprintf("W %s %s", pointer, rule))
}

func (r *recSink) keys() string {
	var out []string
	for _, kv := range r.kvs {
		out = append(out, string(kv.Key))
	}
	return strings.Join(out, " ")
}

func doc() *vrxv1.DesiredState {
	return &vrxv1.DesiredState{
		Services: &vrxv1.ServicesConfig{
			Dns: &vrxv1.DnsService{
				Resolvers: map[string]*vrxv1.DnsResolver{"lan": {Listen: []*vrxv1.SocketAddress{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(31053)}}}},
				VppCache:  &vrxv1.DnsService_VppCache{Enabled: proto.Bool(true), Upstreams: []string{"192.0.2.53", "2001:db8::53"}},
			},
			Ntp:  &vrxv1.NtpService{Enabled: proto.Bool(true), Servers: []*vrxv1.NtpService_Server{{Address: proto.String("127.0.0.1")}}},
			Snmp: &vrxv1.SnmpService{Enabled: proto.Bool(true)},
		},
		Management: &vrxv1.ManagementConfig{
			Syslog: []*vrxv1.SyslogTarget{{Address: proto.String("127.0.0.1"), Protocol: proto.String("tcp"), Vrf: proto.String("default")}},
			Users:  []*vrxv1.ManagementUser{{Username: proto.String("admin"), Role: proto.String("admin")}},
		},
	}
}

func TestHostServicesProjection(t *testing.T) {
	s := &recSink{}
	HostServices(s, doc(), true, true)
	want := "unbound.config/vrx dns.name-server/192.0.2.53 dns.name-server/2001:db8::53 dns.enable/global chrony.config/vrx rsyslog.config/vrx"
	if got := s.keys(); got != want {
		t.Fatalf("keys\n got %s\nwant %s", got, want)
	}
	if s.ptrs[unbound.Key] != "/services/dns/resolvers" || s.ptrs[chrony.Key] != "/services/ntp" || s.ptrs[rsyslog.Key] != "/management/syslog" {
		t.Fatalf("pointers %v", s.ptrs)
	}
	wantIssues := []string{
		"W /services/dns/vppCache agent.unsupported-field", // write-only: never compared by /state/drift
		"W /services/dns/vppCache/enabled services.dns-vpp-cache-exposure", // M5: answers on every VPP address
		"W /services/snmp agent.unsupported-field",
		"W /management/users agent.unsupported-field",
	}
	if strings.Join(s.issues, "|") != strings.Join(wantIssues, "|") {
		t.Fatalf("issues\n got %v\nwant %v", s.issues, wantIssues)
	}
	// only the authoritative domains
	s = &recSink{}
	HostServices(s, doc(), false, true)
	if s.keys() != "rsyslog.config/vrx" {
		t.Fatalf("management only: %s", s.keys())
	}
}

func TestHostServicesRefusals(t *testing.T) {
	d := doc()
	d.Services.Ntp.Servers[0].KeyRef = proto.String("key/ntp")
	d.Services.Dns.VppCache = &vrxv1.DnsService_VppCache{Enabled: proto.Bool(true), Upstreams: []string{"bogus"}}
	d.Management.Syslog = append(d.Management.Syslog,
		&vrxv1.SyslogTarget{Address: proto.String("192.0.2.9"), Protocol: proto.String("tls"), Tls: &vrxv1.SyslogTls{CaRef: proto.String("cert/ca")}},
		&vrxv1.SyslogTarget{Address: proto.String("192.0.2.8"), Vrf: proto.String("mgmt")})
	s := &recSink{}
	HostServices(s, d, true, true)
	joined := strings.Join(s.issues, "|")
	for _, w := range []string{
		"E /services/ntp/servers/0/keyRef " + RuleSecretChannel,
		"E /services/dns/vppCache/upstreams/0 services.dns-vpp-cache-upstream",
		"E /services/dns/vppCache/upstreams services.dns-vpp-cache-upstream",
		"E /management/syslog/1/tls " + RuleSecretChannel,
		"W /management/syslog/2/vrf agent.unsupported-field",
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in %v", w, s.issues)
		}
	}
	if strings.Contains(s.keys(), chrony.Name) || strings.Contains(s.keys(), rsyslog.Name) || strings.Contains(s.keys(), dnsd.NameEnable) {
		t.Fatalf("refused objects projected: %s", s.keys())
	}
}

func TestHostServicesDisabledAndEmpty(t *testing.T) {
	d := &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{
		Dns: &vrxv1.DnsService{},
		Ntp: &vrxv1.NtpService{Enabled: proto.Bool(false), Port: proto.Uint32(123)},
	}, Management: &vrxv1.ManagementConfig{}}
	s := &recSink{}
	HostServices(s, d, true, true)
	if len(s.kvs) != 0 {
		t.Fatalf("objects %s", s.keys())
	}
	if strings.Join(s.issues, "|") != "W /services/ntp agent.unsupported-field" {
		t.Fatalf("issues %v", s.issues)
	}
}

func TestHostServicesRoundTrip(t *testing.T) {
	s := &recSink{}
	HostServices(s, doc(), true, true)
	out := &vrxv1.DesiredState{}
	AssembleHostServices(out, s.kvs, true, true)
	again := &recSink{}
	HostServices(again, out, true, true)
	if s.keys() != again.keys() {
		t.Fatalf("round trip\n got %s\nwant %s", again.keys(), s.keys())
	}
	for i := range s.kvs {
		if !proto.Equal(s.kvs[i].Value, again.kvs[i].Value) {
			t.Fatalf("%s differs after the round trip", s.kvs[i].Key)
		}
	}
	if !proto.Equal(out.GetServices().GetNtp(), doc().GetServices().GetNtp()) || len(out.GetManagement().GetSyslog()) != 1 {
		t.Fatalf("assembled %v", out)
	}
	// a drift Value (not the input type) contributes nothing
	drift := []scheduler.KV{{Key: unbound.Key, Value: unbound.DriftValue([]string{"x"})}}
	out = &vrxv1.DesiredState{}
	AssembleHostServices(out, drift, true, true)
	if out.GetServices().GetDns() != nil {
		t.Fatal("drift assembled as configuration")
	}
}

// H1 (review): IPv6-only upstreams crash VPP 26.06 (NULL IPv4 server vector): the projection refuses them at the
// upstreams pointer and emits no dns.* object. The old builder projected them.
func TestVPPCacheNeedsAnIPv4Upstream(t *testing.T) {
	d := doc()
	d.Services.Dns.VppCache = &vrxv1.DnsService_VppCache{Enabled: proto.Bool(true), Upstreams: []string{"2001:db8::53", "2001:db8::54"}}
	s := &recSink{}
	HostServices(s, d, true, false)
	if !strings.Contains(strings.Join(s.issues, "|"), "E /services/dns/vppCache/upstreams services.dns-vpp-cache-upstream") {
		t.Fatalf("issues %v", s.issues)
	}
	if strings.Contains(s.keys(), dnsd.NameEnable) {
		t.Fatalf("an IPv6-only cache was projected: %s", s.keys())
	}
	// one IPv4 upstream is enough (IPv6 ones may accompany it)
	d.Services.Dns.VppCache.Upstreams = []string{"2001:db8::53", "192.0.2.53"}
	s = &recSink{}
	HostServices(s, d, true, false)
	if strings.Contains(strings.Join(s.issues, "|"), "E ") || !strings.Contains(s.keys(), dnsd.NameEnable) {
		t.Fatalf("issues %v keys %s", s.issues, s.keys())
	}
}
