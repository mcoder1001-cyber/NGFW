package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	ucsaction "ngfw/agent/internal/actions/unbound-chrony-syslog"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// F-unbound-chrony-syslog: the host services through the agent service (fake VPP; the renderers' own checkers
// unbound-checkconf / chronyd -p / rsyslogd -N1 run on staged copies — read-only — and no daemon is started or
// signalled: a non-owner agent only records start/restart requests).

const hostServicesDoc = `{
  "services": {
    "dns": {"resolvers": {"lab": {"listen": [{"address": "127.0.0.1", "port": 31953}],
      "localZones": [{"zone": "lab.example.", "records": [{"name": "gw.lab.example.", "type": "A", "data": "192.0.2.1"}]}]}}},
    "ntp": {"enabled": true, "servers": [{"address": "127.0.0.1"}], "port": 0}
  },
  "management": {"syslog": [{"address": "127.0.0.1", "port": 31916, "protocol": "tcp", "facilities": ["local7"], "queueSize": 500}]}
}`

func needHostTools(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (the chrony instance directory is chowned to _chrony)")
	}
	for _, b := range []string{"/usr/sbin/unbound-checkconf", "/usr/sbin/chronyd", "/usr/sbin/rsyslogd", "/usr/share/dns/root.key"} {
		if _, err := os.Stat(b); err != nil {
			t.Skipf("%s not installed", b)
		}
	}
}

func TestHostServicesApplyRetrieveRollback(t *testing.T) {
	needHostTools(t)
	base := t.TempDir()
	t.Setenv(subsystems.EnvHostServicesDir, base)
	ctx := context.Background()
	s := newSvc(t, coretest.New(), t.TempDir())
	g := &server{svc: s}

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "h1", DesiredState: doc(t, hostServicesDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	for _, f := range []string{"unbound/unbound.conf", "chrony/agent/chrony.conf", "chrony/agent/sources.d/vrx.sources", "rsyslog/rsyslog.conf"} {
		if _, err := os.Stat(filepath.Join(base, f)); err != nil {
			t.Fatalf("not rendered: %v", err)
		}
	}
	got, err := s.Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"services", "management"}})
	if err != nil {
		t.Fatal(err)
	}
	want := doc(t, hostServicesDoc)
	if !proto.Equal(got.GetDesiredState().GetServices().GetDns(), want.GetServices().GetDns()) ||
		!proto.Equal(got.GetDesiredState().GetServices().GetNtp(), want.GetServices().GetNtp()) ||
		!proto.Equal(got.GetDesiredState().GetManagement(), want.GetManagement()) {
		t.Fatalf("retrieve != desired:\n%v", got.GetDesiredState())
	}
	// nothing runs: every daemon has a start request, none of them is an error
	dnsSt, err := g.DnsState(ctx, &vrxv1.DnsStateRequest{})
	if err != nil || dnsSt.GetRunning() || len(dnsSt.GetPendingActions()) != 1 || dnsSt.GetPendingActions()[0].GetAction() != "start" ||
		dnsSt.GetConfigPath() != filepath.Join(base, "unbound/unbound.conf") || dnsSt.GetVppCache().GetConfigured() {
		t.Fatalf("dns state %v %v", dnsSt, err)
	}
	ntpSt, err := g.NtpState(ctx, &vrxv1.NtpStateRequest{})
	if err != nil || ntpSt.GetRunning() || len(ntpSt.GetPendingActions()) != 1 {
		t.Fatalf("ntp state %v %v", ntpSt, err)
	}
	sysSt, err := g.SyslogState(ctx, &vrxv1.SyslogStateRequest{})
	if err != nil || len(sysSt.GetTargets()) != 1 || sysSt.GetTargets()[0].GetReported() || len(sysSt.GetPendingActions()) != 1 {
		t.Fatalf("syslog state %v %v", sysSt, err)
	}
	if _, err := g.DnsState(ctx, &vrxv1.DnsStateRequest{Owner: "w3"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner: %v", err)
	}

	// idempotent: the same document again plans nothing
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "h2", DesiredState: doc(t, hostServicesDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := len(resp.GetResults()); n != 0 {
		t.Fatalf("re-apply changed %d objects: %v", n, resp.GetResults())
	}
	// removal (rollback of the feature): empty services/management → idle / disabled / empty renderings
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "h3", DesiredState: doc(t, `{"services": {}, "management": {}}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err = s.Retrieve(ctx, &vrxv1.RetrieveRequest{Subsystems: []string{"services", "management"}})
	if err != nil || got.GetDesiredState().GetServices() != nil || got.GetDesiredState().GetManagement() != nil {
		t.Fatalf("after removal: %v %v", got.GetDesiredState(), err)
	}
	conf, _ := os.ReadFile(filepath.Join(base, "unbound/unbound.conf")) //nolint:gosec // the test's own temp dir
	if !strings.Contains(string(conf), "# no enabled resolver") {
		t.Fatalf("unbound not idle:\n%s", conf)
	}
}

func TestHostServicesRefusedAtDryRun(t *testing.T) {
	t.Setenv(subsystems.EnvHostServicesDir, t.TempDir())
	s := newSvc(t, coretest.New(), t.TempDir())
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: doc(t, `{"management": {"syslog": [
	  {"address": "192.0.2.1", "protocol": "tls", "tls": {"caRef": "cert/ca"}}]}}`)})
	if err != nil || rep.GetOk() {
		t.Fatalf("dry run %v %v", rep, err)
	}
	if e := rep.GetErrors()[0]; e.GetPointer() != "/management/syslog/0/tls" || e.GetRule() != "agent.secret-channel-pending" {
		t.Fatalf("issue %v", e)
	}
}

func TestSyslogEntriesRPC(t *testing.T) {
	s := newSvc(t, coretest.New(), t.TempDir())
	g := &server{svc: s}
	rr := renderers.NewRecordingRunner().Succeed(ucsaction.JournalctlBin,
		`{"__REALTIME_TIMESTAMP":"1790282630081753","PRIORITY":"6","SYSLOG_FACILITY":"3","SYSLOG_IDENTIFIER":"unbound","MESSAGE":"info: start of service"}`)
	_ = logRunner() // the Once has run: the variable below is what every later call uses
	prev := journalRunner
	journalRunner = rr
	t.Cleanup(func() { journalRunner = prev })
	resp, err := g.SyslogEntries(context.Background(), &vrxv1.SyslogEntriesRequest{Severity: "info", Query: "SERVICE"})
	if err != nil || resp.GetTotal() != 1 || resp.GetEntries()[0].GetIdentifier() != "unbound" || resp.GetOwner() != testOwner {
		t.Fatalf("entries %v %v", resp, err)
	}
	if _, err := g.SyslogEntries(context.Background(), &vrxv1.SyslogEntriesRequest{Severity: "loud"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("bad severity: %v", err)
	}
}

// actionStream is a minimal grpc.ServerStreamingServer[vrxv1.ActionOutput].
type actionStream struct {
	grpc.ServerStream
	ctx context.Context
	out []*vrxv1.ActionOutput
}

func (s *actionStream) Context() context.Context         { return s.ctx }
func (s *actionStream) Send(o *vrxv1.ActionOutput) error { s.out = append(s.out, o); return nil }

// newOwnerSvc is newSvc as the globals owner (the product agent): DF-8's dns.* descriptors are registered for real.
func newOwnerSvc(t *testing.T, v *coretest.VPP, dir string) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs, GlobalsOwner: true})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: scheduler.New(reg, nil), StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return svc
}

// Review H2/L6: dns_lookup readiness comes from DF-8's live fact, never from the stored document. The stored document
// says "VPP cache enabled with an IPv4 upstream" (as after a VPP restart, or while a resync keeps failing), yet nothing
// was applied on the running VPP: the lookup is refused and no dns.api message is sent — for a slot agent and for the
// globals owner alike. The old guard (owner && stored enabled && upstreams) sent dns_resolve_name here.
func TestDNSLookupReadinessIsLiveNotStored(t *testing.T) {
	t.Setenv(subsystems.EnvHostServicesDir, t.TempDir())
	cached := doc(t, `{"services": {"dns": {"vppCache": {"enabled": true, "upstreams": ["192.0.2.53"]}}}}`)
	for name, mk := range map[string]func(*testing.T, *coretest.VPP, string) *Service{"slot agent": newSvc, "globals owner": newOwnerSvc} {
		t.Run(name, func(t *testing.T) {
			v := coretest.New()
			s := mk(t, v, t.TempDir())
			s.st.desired = cached // the stored document, as a VPP restart leaves it
			v.Reset()
			g := &server{svc: s, log: s.log}
			st := &actionStream{ctx: context.Background()}
			err := g.Action(&vrxv1.ActionRequest{Action: &vrxv1.ActionRequest_DnsLookup{DnsLookup: &vrxv1.DnsLookupAction{Name: "gw.lab.example"}}}, st)
			if grpcCode(err) != codes.FailedPrecondition || len(st.out) != 0 {
				t.Fatalf("want FailedPrecondition and no output, got %v %v", err, st.out)
			}
			for _, m := range v.Calls() {
				if n := m.GetMessageName(); strings.HasPrefix(n, "dns_") {
					t.Fatalf("%s reached VPP", n)
				}
			}
			dnsSt, err := g.DnsState(context.Background(), &vrxv1.DnsStateRequest{})
			if err != nil || !dnsSt.GetVppCache().GetConfigured() || dnsSt.GetVppCache().GetAppliedByThisAgent() {
				t.Fatalf("vppCache state %v %v", dnsSt.GetVppCache(), err)
			}
		})
	}
}

// Review M3: one walk of a kind in flight; a second caller gets UNAVAILABLE after walkWait.
func TestStateWalksAreSerialised(t *testing.T) {
	t.Setenv(subsystems.EnvHostServicesDir, t.TempDir())
	s := newSvc(t, coretest.New(), t.TempDir())
	g := &server{svc: s, log: s.log}
	prev := walkWait
	walkWait = 50 * time.Millisecond
	t.Cleanup(func() { walkWait = prev })
	for name, w := range map[string]*walkLimit{"dns": dnsWalk, "ntp": ntpWalk, "syslog": syslogWalk, "journal": journalWalk} {
		release, err := w.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var callErr error
		switch name {
		case "dns":
			_, callErr = g.DnsState(context.Background(), &vrxv1.DnsStateRequest{})
		case "ntp":
			_, callErr = g.NtpState(context.Background(), &vrxv1.NtpStateRequest{})
		case "syslog":
			_, callErr = g.SyslogState(context.Background(), &vrxv1.SyslogStateRequest{})
		case "journal":
			_, callErr = g.SyslogEntries(context.Background(), &vrxv1.SyslogEntriesRequest{})
		}
		release()
		if grpcCode(callErr) != codes.Unavailable {
			t.Fatalf("%s: a second walk while one is in flight: %v", name, callErr)
		}
	}
	if _, err := g.DnsState(context.Background(), &vrxv1.DnsStateRequest{}); err != nil {
		t.Fatalf("after the release: %v", err)
	}
}
