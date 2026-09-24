package vrfstaticecmp

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ping"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/vpp"
)

// Ping limits. VPP's ping API (want_ping_finished_events) runs the whole ping inside the binary-API process: every other
// API client of the VPP instance waits for count × interval, so the total is capped (docs/vpp-code-track.md, V-new of
// F-vrf-static-ecmp).
const (
	DefaultCount     = 5
	MaxCount         = 100
	DefaultInterval  = time.Second
	MinInterval      = 100 * time.Millisecond
	MaxPingDuration  = 5 * time.Second
	eventGracePeriod = 5 * time.Second
)

// VItem names the vpp-code-track item for what the VPP ping API cannot do.
const VItem = "docs/vpp-code-track.md V-new (F-vrf-static-ecmp)"

// Errors (the RPC maps ErrInvalid to INVALID_ARGUMENT, ErrBusy to UNAVAILABLE, ErrUnimplemented to UNIMPLEMENTED).
var (
	ErrInvalid       = errors.New("invalid action argument")
	ErrBusy          = errors.New("another ping is running")
	ErrUnimplemented = errors.New("not implemented by VPP")
)

// pingMu serialises pings of this agent: ping_finished_event carries no request id, so two pings on one API connection
// could not tell their results apart.
var pingMu sync.Mutex

// PingPlan is a validated ping request.
type PingPlan struct {
	Target   netip.Addr
	Count    uint32
	Interval time.Duration
}

// ValidatePing checks a PingAction against what VPP's ping API can do: the default table only (no VRF), no source
// address, no payload size, count × interval ≤ MaxPingDuration.
func ValidatePing(p *vrxv1.PingAction) (PingPlan, error) {
	var out PingPlan
	a, err := netip.ParseAddr(strings.TrimSpace(p.GetTarget()))
	if err != nil || a.Zone() != "" {
		return out, fmt.Errorf("%w: target %q is not an IPv4 or IPv6 address (the agent does not resolve names)", ErrInvalid, p.GetTarget())
	}
	out.Target = a.Unmap()
	if v := p.GetVrf(); v != "" && v != "default" {
		return out, fmt.Errorf("%w: ping in VRF %q: VPP's ping API has no table and always pings from the default VRF (%s)", ErrInvalid, v, VItem)
	}
	if p.GetSource() != "" {
		return out, fmt.Errorf("%w: a source address is not supported: VPP's ping API chooses it from the FIB (%s)", ErrInvalid, VItem)
	}
	if p.GetSize() != 0 {
		return out, fmt.Errorf("%w: a payload size is not supported: VPP's ping API always sends the default size (%s)", ErrInvalid, VItem)
	}
	out.Count = p.GetCount()
	if out.Count == 0 {
		out.Count = DefaultCount
	}
	if out.Count > MaxCount {
		return out, fmt.Errorf("%w: count %d > %d", ErrInvalid, out.Count, MaxCount)
	}
	out.Interval = time.Duration(p.GetIntervalMs()) * time.Millisecond
	if out.Interval == 0 {
		out.Interval = DefaultInterval
	}
	if out.Interval < MinInterval {
		return out, fmt.Errorf("%w: interval %v < %v", ErrInvalid, out.Interval, MinInterval)
	}
	if d := time.Duration(out.Count) * out.Interval; d > MaxPingDuration {
		return out, fmt.Errorf("%w: count × interval = %v > %v (VPP's ping API holds the binary API for the whole ping)", ErrInvalid, d, MaxPingDuration)
	}
	return out, nil
}

// Ping runs one validated ping through VPP's ping plugin (want_ping_finished_events → ping_finished_event) and sends
// one summary line and the terminal done.
func Ping(ctx context.Context, c vpp.Client, p PingPlan, send func(*vrxv1.ActionOutput) error) error {
	if !pingMu.TryLock() {
		return ErrBusy
	}
	defer pingMu.Unlock()
	wctx, cancel := context.WithTimeout(ctx, time.Duration(p.Count)*p.Interval+eventGracePeriod)
	defer cancel()
	w, err := c.WatchEvent(wctx, &ping.PingFinishedEvent{})
	if err != nil {
		return fmt.Errorf("watch ping_finished_event: %w", err)
	}
	defer w.Close()
	addr, err := ip_types.ParseAddress(p.Target.String())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if _, err := ping.NewServiceClient(c).WantPingFinishedEvents(wctx, &ping.WantPingFinishedEvents{Address: addr, Repeat: p.Count, Interval: p.Interval.Seconds()}); err != nil {
		return fmt.Errorf("want_ping_finished_events: %w", err)
	}
	var ev *ping.PingFinishedEvent
	for ev == nil {
		select {
		case m, ok := <-w.Events():
			if !ok {
				return fmt.Errorf("ping_finished_event: watcher closed (%w)", wctx.Err())
			}
			ev, _ = m.(*ping.PingFinishedEvent)
		case <-wctx.Done():
			return fmt.Errorf("ping_finished_event not received: %w", wctx.Err())
		}
	}
	loss := 100.0
	if ev.RequestCount > 0 {
		loss = 100 * float64(ev.RequestCount-min(ev.ReplyCount, ev.RequestCount)) / float64(ev.RequestCount)
	}
	summary := fmt.Sprintf("PING %s (default VRF): %d packets transmitted, %d received, %.0f%% packet loss", p.Target, ev.RequestCount, ev.ReplyCount, loss)
	if err := send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Line{Line: summary}}); err != nil {
		return err
	}
	exit := int32(0)
	if ev.ReplyCount == 0 {
		exit = 1
	}
	return send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{
		Summary:  summary,
		ExitCode: exit,
		Stats: map[string]string{
			"transmitted": strconv.FormatUint(uint64(ev.RequestCount), 10),
			"received":    strconv.FormatUint(uint64(ev.ReplyCount), 10),
			"loss_pct":    strconv.FormatFloat(loss, 'f', 0, 64),
		},
	}}})
}

// Traceroute is not available: VPP has no traceroute API and there is no Linux path into the data plane before
// linux-cp (P12).
func Traceroute(*vrxv1.TracerouteAction) error {
	return fmt.Errorf("%w: traceroute: VPP has no traceroute API; it needs a linux-cp host path (P12) or a VPP change (%s)", ErrUnimplemented, VItem)
}
