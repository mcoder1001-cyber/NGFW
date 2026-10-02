//go:build linux

package multiwan

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// DeviceProbe binds every connection, including DNS resolution, to the member's
// Linux LCP device. It never falls back to an unbound route or shell command.
// Lookup returns only the default-namespace/default-VRF device; unsupported
// namespaces/VRFs are explicitly unavailable until their networking is wired.
func DeviceProbe(lookup func(string) (string, error)) Probe {
	var sequence atomic.Uint32
	return func(ctx context.Context, member string, monitor *vrxv1.WanMonitor) CheckResult {
		fail := CheckResult{Sent: 1}
		device, err := lookup(member)
		if err != nil || device == "" {
			return fail
		}
		dialer := &net.Dialer{Control: func(_, _ string, raw syscall.RawConn) error {
			var bindErr error
			err := raw.Control(func(fd uintptr) {
				bindErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, device)
			})
			if err != nil {
				return err
			}
			return bindErr
		}}
		resolver := &net.Resolver{PreferGo: true, Dial: func(c context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(c, network, address)
		}}
		dialer.Resolver = resolver
		return probeTarget(ctx, monitor, dialer.DialContext, resolver.LookupIPAddr, &sequence)
	}
}

func probeTarget(ctx context.Context, monitor *vrxv1.WanMonitor, dial func(context.Context, string, string) (net.Conn, error), resolve func(context.Context, string) ([]net.IPAddr, error), sequence *atomic.Uint32) CheckResult {
	fail := CheckResult{Sent: 1}
	start := time.Now()
	success := func() CheckResult {
		return CheckResult{Sent: 1, Received: 1, AvgLatencyMs: int(time.Since(start) / time.Millisecond)}
	}
	target := monitor.GetTarget()
	if target == "" || strings.ContainsAny(target, "/\\\x00\r\n:@") {
		return fail
	}
	switch monitor.GetType() {
	case "http":
		transport := &http.Transport{DialContext: dial, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		endpoint := url.URL{Scheme: "http", Host: net.JoinHostPort(target, "80")}
		request, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint.String(), nil)
		if err != nil {
			return fail
		}
		response, err := client.Do(request)
		if err != nil {
			return fail
		}
		_ = response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 400 {
			return success()
		}
	case "dns":
		// Target is the DNS server. Query the root NS RRset, so literal
		// server addresses perform a real bound DNS exchange as well.
		conn, err := dial(ctx, "udp", net.JoinHostPort(target, "53"))
		if err != nil {
			return fail
		}
		defer func() { _ = conn.Close() }()
		deadline, ok := ctx.Deadline()
		if !ok {
			return fail
		}
		if err := conn.SetDeadline(deadline); err != nil {
			return fail
		}
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		id := uint16(sequence.Add(1) & 0xffff)
		query := dnsmessage.Message{Header: dnsmessage.Header{ID: id, RecursionDesired: true}, Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName("."), Type: dnsmessage.TypeNS, Class: dnsmessage.ClassINET}}}
		packet, err := query.Pack()
		if err != nil {
			return fail
		}
		if _, err := conn.Write(packet); err != nil {
			return fail
		}
		buffer := make([]byte, 4096)
		n, err := conn.Read(buffer)
		if err != nil {
			return fail
		}
		var response dnsmessage.Message
		if response.Unpack(buffer[:n]) != nil || response.ID != id || !response.Response || response.Truncated || response.RCode != dnsmessage.RCodeSuccess {
			return fail
		}
		if len(response.Questions) != 1 || response.Questions[0] != query.Questions[0] || len(response.Answers) == 0 {
			return fail
		}
		return success()
	case "icmp":
		addresses, err := resolve(ctx, target)
		if err != nil {
			return fail
		}
		var address net.IP
		for _, candidate := range addresses {
			if candidate.IP.To4() != nil {
				address = candidate.IP.To4()
				break
			}
		}
		if address == nil {
			return fail
		}
		conn, err := dial(ctx, "ip4:icmp", address.String())
		if err != nil {
			return fail
		}
		defer func() { _ = conn.Close() }()
		deadline, ok := ctx.Deadline()
		if !ok {
			return fail
		}
		if err := conn.SetDeadline(deadline); err != nil {
			return fail
		}
		// Cancellation must interrupt the pending read before configuration drain.
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		seq := int(sequence.Add(1) & 0xffff)
		message := icmp.Message{Type: ipv4.ICMPTypeEcho, Code: 0, Body: &icmp.Echo{ID: 0x5652, Seq: seq, Data: []byte("vrx-wan")}}
		packet, err := message.Marshal(nil)
		if err != nil {
			return fail
		}
		if _, err := conn.Write(packet); err != nil {
			return fail
		}
		buffer := make([]byte, 2048)
		for ctx.Err() == nil {
			n, err := conn.Read(buffer)
			if err != nil {
				return fail
			}
			reply, err := icmp.ParseMessage(1, buffer[:n])
			if err != nil {
				continue
			}
			echo, ok := reply.Body.(*icmp.Echo)
			if reply.Type == ipv4.ICMPTypeEchoReply && ok && echo.ID == 0x5652 && echo.Seq == seq && string(echo.Data) == "vrx-wan" {
				return success()
			}
		}
	}
	return fail
}

// UnsupportedDevice is a sanitized failure suitable for callers that cannot
// resolve a default namespace Linux LCP interface.
func UnsupportedDevice() error { return fmt.Errorf("WAN member has no supported Linux LCP device") }
