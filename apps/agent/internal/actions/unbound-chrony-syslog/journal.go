// Package ucsaction holds the read-only actions of F-unbound-chrony-syslog: the log explorer (a bounded, paged
// query of the local journal) and the DNS lookup through VPP's DNS cache.
//
// The log explorer's source is journald (decision, docs/status/tasks/F-unbound-chrony-syslog.md): the RF-4 export
// renders only omfwd and never a local file (omfile is outside its fixed template set), so there is no
// rsyslog-written file to read. journalctl runs with a fixed argv through renderers.Runner (allow-listed in
// internal/renderers/ALLOWLIST.md); every filter is a validated enum value or a number — the free-text query is
// matched here, never passed to journalctl (no --grep, no pattern, no shell).
package ucsaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// JournalctlBin is the log explorer's only binary (internal/renderers/ALLOWLIST.md).
const JournalctlBin = "/usr/bin/journalctl"

// Bounds of one query.
const (
	// ScanLimit is the most entries one query reads from the journal (newest first).
	ScanLimit = 5000
	// MaxPageSize bounds a page; DefaultPageSize is used for 0.
	MaxPageSize     = 500
	DefaultPageSize = 100
	// DefaultWindow is the look-back for an unset `since`; MaxWindow the oldest allowed.
	DefaultWindow = time.Hour
	MaxWindow     = 30 * 24 * time.Hour
	// MaxQueryLen bounds the free-text filter; MaxMessageLen one returned message (characters).
	MaxQueryLen   = 128
	MaxMessageLen = 4096
	// maxOutput bounds journalctl's captured stdout (ScanLimit entries of the selected fields).
	maxOutput      = 16 << 20
	journalTimeout = 20 * time.Second
)

// Severities in RFC 5424 order (PRIORITY 0..7), the names the schema uses.
var Severities = []string{"emergency", "alert", "critical", "error", "warning", "notice", "info", "debug"}

// facilityNames maps RFC 5424 facility numbers to the names journalctl --facility accepts.
var facilityNames = map[int]string{
	0: "kern", 1: "user", 2: "mail", 3: "daemon", 4: "auth", 5: "syslog", 6: "lpr", 7: "news", 8: "uucp", 9: "cron",
	10: "authpriv", 11: "ftp", 16: "local0", 17: "local1", 18: "local2", 19: "local3", 20: "local4", 21: "local5",
	22: "local6", 23: "local7",
}

// outputFields are the journal fields a query reads (journalctl adds its own __REALTIME_TIMESTAMP etc.).
const outputFields = "MESSAGE,PRIORITY,SYSLOG_FACILITY,SYSLOG_IDENTIFIER,_COMM,_PID,_HOSTNAME,_SYSTEMD_UNIT"

// NewRunner returns the production runner of the log explorer (journalctl only, bounded output).
func NewRunner() *renderers.SystemRunner {
	r := renderers.NewSystemRunner(renderers.NewAllowlist(JournalctlBin))
	r.MaxOutput = maxOutput
	return r
}

// Query is one validated log-explorer request.
type Query struct {
	Since    time.Time
	Priority int    // maximum PRIORITY (0..7); -1 = all
	Facility string // "" = all
	Text     string // lower-cased substring; "" = all
	Page     int    // 1-based
	PageSize int
}

// ParseRequest validates req (now = the agent clock). Errors are InvalidArgument.
func ParseRequest(req *vrxv1.SyslogEntriesRequest, now time.Time) (Query, error) {
	q := Query{Priority: -1, Page: int(req.GetPage()), PageSize: int(req.GetPageSize())}
	q.Since = now.Add(-DefaultWindow)
	if req.GetSince() != nil {
		if err := req.GetSince().CheckValid(); err != nil {
			return q, status.Errorf(codes.InvalidArgument, "since: %v", err)
		}
		q.Since = req.GetSince().AsTime()
	}
	if oldest := now.Add(-MaxWindow); q.Since.Before(oldest) {
		q.Since = oldest
	}
	if q.Since.After(now) {
		return q, status.Error(codes.InvalidArgument, "since is in the future")
	}
	if s := req.GetSeverity(); s != "" {
		q.Priority = -1
		for i, n := range Severities {
			if n == s {
				q.Priority = i
			}
		}
		if q.Priority < 0 {
			return q, status.Errorf(codes.InvalidArgument, "severity %q is not one of %s", s, strings.Join(Severities, ", "))
		}
	}
	if f := req.GetFacility(); f != "" {
		ok := false
		for _, n := range facilityNames {
			ok = ok || n == f
		}
		if !ok {
			return q, status.Errorf(codes.InvalidArgument, "facility %q is not a syslog facility name", f)
		}
		q.Facility = f
	}
	if t := req.GetQuery(); t != "" {
		if utf8.RuneCountInString(t) > MaxQueryLen || !utf8.ValidString(t) {
			return q, status.Errorf(codes.InvalidArgument, "query must be at most %d characters of valid UTF-8", MaxQueryLen)
		}
		for _, r := range t {
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x2028 || r == 0x2029 {
				return q, status.Error(codes.InvalidArgument, "query must not contain control characters")
			}
		}
		q.Text = strings.ToLower(t)
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = DefaultPageSize
	}
	if q.PageSize < 1 || q.PageSize > MaxPageSize {
		return q, status.Errorf(codes.InvalidArgument, "page_size %d outside 1..%d", q.PageSize, MaxPageSize)
	}
	if q.Page > ScanLimit {
		return q, status.Errorf(codes.InvalidArgument, "page %d outside 1..%d", q.Page, ScanLimit)
	}
	return q, nil
}

// Argv is journalctl's fixed argument vector for q: JSON, newest first, the scan bound, the time window and the
// enum filters. The free-text filter is never part of it.
func (q Query) Argv() []string {
	args := []string{"--no-pager", "--quiet", "--output=json", "--output-fields=" + outputFields, "--reverse",
		"--lines=" + strconv.Itoa(ScanLimit), "--since=@" + strconv.FormatInt(q.Since.Unix(), 10)}
	if q.Priority >= 0 {
		args = append(args, "--priority="+strconv.Itoa(q.Priority))
	}
	if q.Facility != "" {
		args = append(args, "--facility="+q.Facility)
	}
	return args
}

// Run executes q through runner and returns one page (newest first).
func Run(ctx context.Context, runner renderers.Runner, q Query) (*vrxv1.SyslogEntriesResponse, error) {
	out, err := runner.Run(ctx, renderers.Command{Path: JournalctlBin, Args: q.Argv(), Timeout: journalTimeout})
	var ee *renderers.ExitError
	switch {
	case errors.As(err, &ee) && len(out.Stdout) == 0 && ee.Output.ExitCode == 1:
		// journalctl exits 1 when no entry matches the filters (e.g. an empty facility)
	case err != nil:
		return nil, status.Errorf(codes.Internal, "journalctl: %v %s", err, firstLine(out.Stderr))
	}
	resp := &vrxv1.SyslogEntriesResponse{Page: uint32(q.Page), PageSize: uint32(q.PageSize), Source: "journald"} //nolint:gosec // bounded above
	var matches []*vrxv1.SyslogEntry
	for _, line := range bytes.Split(out.Stdout, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		e, ok := parseEntry(line)
		if !ok {
			continue // a partial last line at the output bound
		}
		resp.Scanned++
		if q.Text != "" && !strings.Contains(strings.ToLower(e.GetMessage()), q.Text) && !strings.Contains(strings.ToLower(e.GetIdentifier()), q.Text) {
			continue
		}
		matches = append(matches, e)
	}
	resp.Truncated = resp.Scanned >= ScanLimit || len(out.Stdout) >= maxOutput
	resp.Total = uint32(len(matches)) //nolint:gosec // ≤ ScanLimit
	start := (q.Page - 1) * q.PageSize
	if start < len(matches) {
		resp.Entries = matches[start:min(start+q.PageSize, len(matches))]
	}
	return resp, nil
}

func firstLine(b []byte) string {
	l, _, _ := bytes.Cut(bytes.TrimSpace(b), []byte("\n"))
	if len(l) > 256 {
		l = l[:256]
	}
	return string(bytes.ToValidUTF8(l, []byte("?")))
}

// parseEntry decodes one `journalctl -o json` line. Field values are strings, or — for binary data — arrays of
// byte values (and arrays of strings when a field repeats; the first one is used).
func parseEntry(line []byte) (*vrxv1.SyslogEntry, bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(line, &raw) != nil {
		return nil, false
	}
	field := func(k string) string {
		v, ok := raw[k]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
		var bs []byte
		var nums []int
		if json.Unmarshal(v, &nums) == nil {
			for _, n := range nums {
				bs = append(bs, byte(n)) //nolint:gosec // journald byte values 0..255
			}
			return string(bs)
		}
		var list []string
		if json.Unmarshal(v, &list) == nil && len(list) > 0 {
			return list[0]
		}
		return ""
	}
	us, err := strconv.ParseInt(field("__REALTIME_TIMESTAMP"), 10, 64)
	if err != nil {
		return nil, false
	}
	e := &vrxv1.SyslogEntry{Time: timestamppb.New(time.UnixMicro(us)), Hostname: field("_HOSTNAME"), Unit: field("_SYSTEMD_UNIT")}
	if p, err := strconv.Atoi(field("PRIORITY")); err == nil && p >= 0 && p < len(Severities) {
		e.Severity = Severities[p]
	}
	if f, err := strconv.Atoi(field("SYSLOG_FACILITY")); err == nil {
		e.Facility = facilityNames[f]
	}
	e.Identifier = field("SYSLOG_IDENTIFIER")
	if e.Identifier == "" {
		e.Identifier = field("_COMM")
	}
	if pid, err := strconv.ParseUint(field("_PID"), 10, 32); err == nil {
		e.Pid = uint32(pid)
	}
	msg := strings.ToValidUTF8(field("MESSAGE"), "�")
	if utf8.RuneCountInString(msg) > MaxMessageLen {
		msg = string([]rune(msg)[:MaxMessageLen]) + "…"
	}
	e.Message = msg
	e.Hostname = strings.ToValidUTF8(e.Hostname, "?")
	e.Identifier = strings.ToValidUTF8(e.Identifier, "?")
	e.Unit = strings.ToValidUTF8(e.Unit, "?")
	return e, true
}

// String describes q for logs.
func (q Query) String() string {
	return fmt.Sprintf("since=%s priority<=%d facility=%q query=%q page=%d/%d", q.Since.UTC().Format(time.RFC3339), q.Priority, q.Facility, q.Text, q.Page, q.PageSize)
}
