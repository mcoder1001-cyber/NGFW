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

// ErrCacheNotReady explains the refusal of a lookup on a VPP whose dns plugin this agent has not set up.
const ErrCacheNotReady = "dns_lookup needs the VPP DNS cache enabled with an upstream by this agent (the globals owner, D-071): " +
	"VPP 26.06 crashes on dns_resolve_name while its dns plugin has no name server (SIGSEGV in ip4_sas, docs/vpp-code-track.md), " +
	"so the lookup is never sent otherwise"

// Lookup resolves a.name through VPP's DNS cache (dns_resolve_name, DF-8 dns.ResolveName) with a deadline and
// streams the result. cacheReady must say that this agent itself enabled the plugin with at least one name server
// (the globals owner with services.dns.vppCache applied): VPP 26.06 dereferences a NULL name server in
// vnet_dns_resolve_name → ip4_sas when it has none, so without that proof the request is refused with
// FAILED_PRECONDITION and never reaches VPP (2026-09-25 04:27 incident, F-unbound-chrony-syslog questions Q2).
// Output: one line per address ("A 192.0.2.1", "AAAA 2001:db8::1"), then done with exit_code 0 and the
// stats "ipv4"/"ipv6"; a VPP error (the plugin disabled — only the globals owner enables it, D-071 — an unreachable
// upstream, the deadline) ends the stream with done{exit_code 1} and the reason. An invalid request is
// InvalidArgument, a disconnected VPP Unavailable.
func Lookup(ctx context.Context, c vpp.Client, a *vrxv1.DnsLookupAction, cacheReady bool, send func(*vrxv1.ActionOutput) error) error {
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
	if !cacheReady {
		return status.Error(codes.FailedPrecondition, ErrCacheNotReady)
	}
	if c == nil || !c.Connected() {
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	lctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ip4, ip6, err := dns.ResolveName(lctx, c, name, dns.Ready(cacheReady))
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
