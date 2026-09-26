package desired

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/hoststack"
	"ngfw/agent/internal/scheduler"
)

type hsSink struct {
	kvs   []scheduler.KV
	errs  []string
	warns []string
}

func (s *hsSink) Add(k scheduler.Key, v proto.Message, _ string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
}
func (s *hsSink) Errorf(p, rule, _ string, _ ...any) { s.errs = append(s.errs, p+" "+rule) }
func (s *hsSink) Warnf(p, rule, _ string, _ ...any)  { s.warns = append(s.warns, p+" "+rule) }

func hsVRF(name string) (uint32, bool) {
	switch name {
	case "", "default":
		return 0, true
	case "hs":
		return 1300, true
	}
	return 0, false
}

func TestHostStackProjection(t *testing.T) {
	t.Setenv(hoststack.EnvHTTPStatic, "")
	hs := &vrxv1.HostStackService{
		Enabled: proto.Bool(true),
		Namespaces: map[string]*vrxv1.HostStackNamespace{
			"w13-app":    {Interface: proto.String("loop13"), Vrf: proto.String("hs")},
			"w13-secret": {SecretRef: proto.String("key/ns"), Vrf: proto.String("default")},
		},
		SessionRules: []*vrxv1.HostStackSessionRule{
			{Tag: proto.String("w13-a"), Transport: proto.String("tcp"), Local: proto.String("10.13.1.0/24"),
				Remote: proto.String("10.13.2.0/24"), Action: proto.String("deny"), LocalPort: proto.Uint32(31390)},
			{Tag: proto.String("w13-b"), Scope: proto.String("local"), Transport: proto.String("udp"), Local: proto.String("fd00:13::/48"),
				Remote: proto.String("::/0"), Action: proto.String("allow"), AppNamespace: proto.String("w13-app")},
		},
		TcpSourceAddresses: &vrxv1.HostStackTcpSource{First: proto.String("10.13.1.10"), Last: proto.String("10.13.1.20"), Vrf: proto.String("hs")},
		HttpStatic:         &vrxv1.HostStackHttpStatic{Enabled: proto.Bool(true), WwwRootPath: proto.String("/var/lib/vrx/www/x"), Uri: proto.String("tcp://10.1.1.1/80")},
	}
	s := &hsSink{}
	HostStack(s, hs, hsVRF)
	var keys []string
	for _, kv := range s.kvs {
		keys = append(keys, string(kv.Key))
	}
	want := "hoststack.session/global hoststack.namespace/w13-app hoststack.session-rule/w13-a hoststack.session-rule/w13-b hoststack.tcp-src/1300"
	if strings.Join(keys, " ") != want {
		t.Fatalf("keys %v\nwant %s", keys, want)
	}
	errs := strings.Join(s.errs, "\n")
	for _, w := range []string{"/services/hostStack/namespaces/w13-secret/secretRef services.host-stack-secret-channel",
		"/services/hostStack/httpStatic/enabled services.host-stack-http-static"} {
		if !strings.Contains(errs, w) {
			t.Errorf("missing error %q in\n%s", w, errs)
		}
	}

	// opt-in: http_static projected; a bad web root refused
	t.Setenv(hoststack.EnvHTTPStatic, "1")
	s = &hsSink{}
	HostStack(s, &vrxv1.HostStackService{HttpStatic: hs.HttpStatic}, hsVRF)
	if len(s.kvs) != 1 || s.kvs[0].Key != hoststack.KeyHTTPStatic {
		t.Fatalf("opt-in: %v %v", s.kvs, s.errs)
	}
	s = &hsSink{}
	HostStack(s, &vrxv1.HostStackService{HttpStatic: &vrxv1.HostStackHttpStatic{Enabled: proto.Bool(true), WwwRootPath: proto.String("/var/lib/vrx/www/../etc")}}, hsVRF)
	if len(s.kvs) != 0 || len(s.errs) != 1 {
		t.Fatalf("bad web root: %v %v", s.kvs, s.errs)
	}

	// assemble: only the rules come back, canonical
	ds := &vrxv1.DesiredState{}
	s = &hsSink{}
	t.Setenv(hoststack.EnvHTTPStatic, "")
	HostStack(s, &vrxv1.HostStackService{SessionRules: hs.SessionRules, Namespaces: map[string]*vrxv1.HostStackNamespace{"w13-app": hs.Namespaces["w13-app"]}}, hsVRF)
	HostStackAssemble(ds, s.kvs)
	got := ds.GetServices().GetHostStack()
	if len(got.GetSessionRules()) != 2 || got.GetEnabled() || len(got.GetNamespaces()) != 0 || got.GetSessionRules()[0].GetScope() != "global" {
		t.Fatalf("assembled %v", got)
	}
}

func TestHostStackCoverageNotes(t *testing.T) {
	s := &hsSink{}
	HostStack(s, &vrxv1.HostStackService{Enabled: proto.Bool(true),
		Namespaces:         map[string]*vrxv1.HostStackNamespace{"a": {Vrf: proto.String("default")}},
		TcpSourceAddresses: &vrxv1.HostStackTcpSource{First: proto.String("10.0.0.1"), Last: proto.String("10.0.0.2")}}, hsVRF)
	got := strings.Join(s.warns, "\n")
	for _, w := range []string{"/services/hostStack/enabled agent.write-only-field", "/services/hostStack/namespaces agent.write-only-field",
		"/services/hostStack/tcpSourceAddresses agent.write-only-field"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	if strings.Contains(got, "sessionRules") {
		t.Errorf("session rules are retrievable, no note: %s", got)
	}
}

func TestUnsupportedServicesNoSilentDrop(t *testing.T) {
	s := &hsSink{}
	ServicesUnsupported(s, &vrxv1.ServicesConfig{ // the one services reporter (desired/qos_services.go)
		Snmp:      &vrxv1.SnmpService{Enabled: proto.Bool(true)},
		Dns:       &vrxv1.DnsService{}, // empty: not reported
		Lldp:      &vrxv1.LldpService{Interfaces: []*vrxv1.LldpService_Interface{{Interface: proto.String("loop0")}}},
		HostStack: &vrxv1.HostStackService{Enabled: proto.Bool(true)},
		Ipfix:     &vrxv1.IpfixService{Exporters: map[string]*vrxv1.IpfixService_Exporter{"x": {}}},
		Dhcp:      &vrxv1.DhcpService{Servers: map[string]*vrxv1.DhcpServer{"x": {}}}, // no builder in this build
	})
	if got := strings.Join(s.warns, ","); got != "/services/dhcp agent.unsupported-field" {
		t.Fatalf("got %q", got)
	}
}
