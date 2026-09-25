package ucsaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dns"
	"ngfw/agent/internal/vpp"
)

// Lookup bounds (DnsLookupAction.timeout_ms).
const (
	DefaultLookupTimeout = 5 * time.Second
	MaxLookupTimeout     = 30 * time.Second
)

// Lookup resolves a.name through VPP's DNS cache (dns_resolve_name, DF-8 dns.ResolveName) with a deadline and
// streams the result: one line per address ("A 192.0.2.1", "AAAA 2001:db8::1"), then done with exit_code 0 and the
// stats "ipv4"/"ipv6"; a VPP error (the plugin disabled — only the globals owner enables it, D-071 — an unreachable
// upstream, the deadline) ends the stream with done{exit_code 1} and the reason. An invalid request is
// InvalidArgument, a disconnected VPP Unavailable.
func Lookup(ctx context.Context, c vpp.Client, a *vrxv1.DnsLookupAction, send func(*vrxv1.ActionOutput) error) error {
	name := a.GetName()
	if err := dns.ValidateName(name); err != nil {
		return status.Errorf(codes.InvalidArgument, "dns_lookup: %v", err)
	}
	timeout := DefaultLookupTimeout
	if ms := a.GetTimeoutMs(); ms != 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	if timeout > MaxLookupTimeout {
		return status.Errorf(codes.InvalidArgument, "dns_lookup: timeout_ms %d above %d", a.GetTimeoutMs(), MaxLookupTimeout.Milliseconds())
	}
	if c == nil || !c.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	lctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ip4, ip6, err := dns.ResolveName(lctx, c, name)
	if err != nil {
		if errors.Is(err, vpp.ErrDisconnected) {
			return status.Error(codes.Unavailable, err.Error())
		}
		why := err.Error()
		if errors.Is(lctx.Err(), context.DeadlineExceeded) {
			why = fmt.Sprintf("no answer within %v (%v)", timeout, err)
		}
		return send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{
			Summary: fmt.Sprintf("%s: lookup failed: %s", name, why), ExitCode: 1,
		}}})
	}
	stats := map[string]string{}
	if ip4.IsValid() {
		stats["ipv4"] = ip4.String()
		if err := send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Line{Line: "A " + ip4.String()}}); err != nil {
			return err
		}
	}
	if ip6.IsValid() {
		stats["ipv6"] = ip6.String()
		if err := send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Line{Line: "AAAA " + ip6.String()}}); err != nil {
			return err
		}
	}
	summary := fmt.Sprintf("%s: %d address(es)", name, len(stats))
	if len(stats) == 0 {
		summary = name + ": no address"
	}
	return send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{Summary: summary, Stats: stats}}})
}
