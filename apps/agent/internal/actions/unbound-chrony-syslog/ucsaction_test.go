package ucsaction

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ngfw/agent/binapi/dns"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/renderers"
)

var now = time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)

func code(err error) codes.Code { return status.Code(err) }

func TestParseRequest(t *testing.T) {
	q, err := ParseRequest(&vrxv1.SyslogEntriesRequest{}, now)
	if err != nil || !q.Since.Equal(now.Add(-time.Hour)) || q.Priority != -1 || q.Page != 1 || q.PageSize != DefaultPageSize {
		t.Fatalf("defaults %+v %v", q, err)
	}
	q, err = ParseRequest(&vrxv1.SyslogEntriesRequest{
		Since: timestamppb.New(now.Add(-400 * 24 * time.Hour)), Severity: "warning", Facility: "local7", Query: "Unbound", Page: 3, PageSize: 50,
	}, now)
	if err != nil || !q.Since.Equal(now.Add(-MaxWindow)) || q.Priority != 4 || q.Facility != "local7" || q.Text != "unbound" || q.Page != 3 || q.PageSize != 50 {
		t.Fatalf("clamped/filters %+v %v", q, err)
	}
	for name, req := range map[string]*vrxv1.SyslogEntriesRequest{
		"future":        {Since: timestamppb.New(now.Add(time.Minute))},
		"severity":      {Severity: "fatal"},
		"facility":      {Facility: "kernel"},
		"facility sh":   {Facility: "local0; rm -rf /"},
		"query long":    {Query: strings.Repeat("a", MaxQueryLen+1)},
		"query newline": {Query: "a\nb"},
		"query u2028":   {Query: "a b"},
		"page size":     {PageSize: MaxPageSize + 1},
		"page":          {Page: ScanLimit + 1},
	} {
		if _, err := ParseRequest(req, now); code(err) != codes.InvalidArgument {
			t.Errorf("%s: want InvalidArgument, got %v", name, err)
		}
	}
}

func TestArgvIsFixed(t *testing.T) {
	q, _ := ParseRequest(&vrxv1.SyslogEntriesRequest{Severity: "error", Facility: "daemon", Query: `"; rm -rf / --grep=x`}, now)
	got := strings.Join(q.Argv(), " ")
	want := fmt.Sprintf("--no-pager --quiet --output=json --output-fields=%s --reverse --lines=%d --since=@%d --priority=3 --facility=daemon",
		outputFields, ScanLimit, now.Add(-time.Hour).Unix())
	if got != want {
		t.Fatalf("argv\n got %s\nwant %s", got, want)
	}
	if strings.Contains(got, "rm") || strings.Contains(got, "grep") {
		t.Fatal("the free-text query reached journalctl's argv")
	}
}

func line(us int64, prio, fac, ident, msg string) string {
	return fmt.Sprintf(`{"__REALTIME_TIMESTAMP":"%d","PRIORITY":%q,"SYSLOG_FACILITY":%q,"SYSLOG_IDENTIFIER":%q,"_PID":"42","_HOSTNAME":"vrx-a","_SYSTEMD_UNIT":"x.service","MESSAGE":%s}`,
		us, prio, fac, ident, msg)
}

func TestRunPagesAndFilters(t *testing.T) {
	var lines []string
	for i := range 7 {
		lines = append(lines, line(int64(1790000000000000-i), "6", "3", "unbound", fmt.Sprintf("%q", fmt.Sprintf("query %d", i))))
	}
	lines = append(lines,
		line(1789999999000000, "3", "23", "vrx-test", `[104,105,255]`), // binary MESSAGE (byte array, invalid UTF-8)
		`{"__REALTIME_TIMESTAMP":"17899`,                               // partial last line at the output bound
	)
	rr := renderers.NewRecordingRunner().Succeed(JournalctlBin, strings.Join(lines, "\n"))
	q, _ := ParseRequest(&vrxv1.SyslogEntriesRequest{Query: "QUERY", Page: 2, PageSize: 3}, now)
	resp, err := Run(context.Background(), rr, q)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetScanned() != 8 || resp.GetTotal() != 7 || len(resp.GetEntries()) != 3 || resp.GetTruncated() || resp.GetSource() != "journald" {
		t.Fatalf("page %+v", resp)
	}
	e := resp.GetEntries()[0]
	if e.GetMessage() != "query 3" || e.GetSeverity() != "info" || e.GetFacility() != "daemon" || e.GetIdentifier() != "unbound" || e.GetPid() != 42 || e.GetUnit() != "x.service" {
		t.Fatalf("entry %+v", e)
	}
	q, _ = ParseRequest(&vrxv1.SyslogEntriesRequest{Page: 1, PageSize: 500}, now)
	resp, _ = Run(context.Background(), rr, q)
	last := resp.GetEntries()[len(resp.GetEntries())-1]
	if last.GetMessage() != "hi�" || last.GetFacility() != "local7" || last.GetSeverity() != "error" {
		t.Fatalf("binary entry %+v", last)
	}
	if c := rr.Calls(); len(c) != 2 || c[0].Path != JournalctlBin {
		t.Fatalf("calls %v", c)
	}
}

func TestRunNoMatchAndFailure(t *testing.T) {
	q, _ := ParseRequest(&vrxv1.SyslogEntriesRequest{Facility: "uucp"}, now)
	rr := renderers.NewRecordingRunner().FailWith(JournalctlBin, 1, "")
	resp, err := Run(context.Background(), rr, q)
	if err != nil || resp.GetTotal() != 0 {
		t.Fatalf("no entry: %v %v", resp, err)
	}
	rr = renderers.NewRecordingRunner().FailWith(JournalctlBin, 2, "Failed to open journal")
	if _, err := Run(context.Background(), rr, q); code(err) != codes.Internal || !strings.Contains(err.Error(), "Failed to open journal") {
		t.Fatalf("failure: %v", err)
	}
}

func TestLookup(t *testing.T) {
	f := dfkittest.NewFake()
	f.On("dns_resolve_name", func(m api.Message) ([]api.Message, error) {
		name := strings.TrimRight(string(m.(*dns.DNSResolveName).Name), "\x00")
		if name == "fail.example" {
			return []api.Message{&dns.DNSResolveNameReply{Retval: int32(api.NO_NAME_SERVERS)}}, nil
		}
		return []api.Message{&dns.DNSResolveNameReply{IP4Set: 1, IP4Address: []byte{192, 0, 2, 1}, IP6Set: 1,
			IP6Address: []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}}}, nil
	})
	var out []*vrxv1.ActionOutput
	send := func(o *vrxv1.ActionOutput) error { out = append(out, o); return nil }
	if err := Lookup(context.Background(), f, &vrxv1.DnsLookupAction{Name: "gw.lab.example"}, true, send); err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].GetLine() != "A 192.0.2.1" || out[1].GetLine() != "AAAA 2001:db8::1" ||
		out[2].GetDone().GetExitCode() != 0 || out[2].GetDone().GetStats()["ipv6"] != "2001:db8::1" {
		t.Fatalf("output %v", out)
	}
	out = nil
	if err := Lookup(context.Background(), f, &vrxv1.DnsLookupAction{Name: "fail.example", TimeoutMs: 1000}, true, send); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].GetDone().GetExitCode() != 1 || !strings.Contains(out[0].GetDone().GetSummary(), "lookup failed") {
		t.Fatalf("failure output %v", out)
	}
	for _, bad := range []*vrxv1.DnsLookupAction{{Name: "a b"}, {Name: "$(id)"}, {Name: ""}, {Name: "x.example", TimeoutMs: 30001}} {
		if err := Lookup(context.Background(), f, bad, true, send); code(err) != codes.InvalidArgument {
			t.Errorf("%v: %v", bad, err)
		}
	}
	if err := Lookup(context.Background(), nil, &vrxv1.DnsLookupAction{Name: "x.example"}, true, send); code(err) != codes.Unavailable {
		t.Errorf("no VPP: %v", err)
	}
}

// The 2026-09-25 04:27 incident: dns_resolve_name on a VPP whose dns plugin has no name server crashes VPP. Without
// the proof that this agent enabled the cache, the request must never be sent.
func TestLookupRefusedWithoutAReadyCache(t *testing.T) {
	f := dfkittest.NewFake()
	f.On("dns_resolve_name", func(api.Message) ([]api.Message, error) {
		t.Fatal("dns_resolve_name reached VPP although the cache is not ready")
		return nil, nil
	})
	send := func(*vrxv1.ActionOutput) error { t.Fatal("output sent"); return nil }
	err := Lookup(context.Background(), f, &vrxv1.DnsLookupAction{Name: "gw.lab.example"}, false, send)
	if code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "ip4_sas") {
		t.Fatalf("want FailedPrecondition naming the crash, got %v", err)
	}
}
