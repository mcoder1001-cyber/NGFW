//go:build linux

package multiwan

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"google.golang.org/protobuf/proto"
	vrxv1 "ngfw/agent/gen/vrx/v1"
)

func TestHTTPProbeHEADAndNoRedirect(t *testing.T) {
	for _, code := range []int{200, 302, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var sequence atomic.Uint32
			var calls atomic.Int32
			dial := func(context.Context, string, string) (net.Conn, error) {
				calls.Add(1)
				a, b := net.Pipe()
				go func() {
					defer func() { _ = b.Close() }()
					request, err := http.ReadRequest(bufio.NewReader(b))
					if err != nil {
						return
					}
					if request.Method != http.MethodHead {
						t.Error("probe was not HEAD")
					}
					_, _ = fmt.Fprintf(b, "HTTP/1.1 %d status\r\nLocation: http://redirect.test/\r\nContent-Length: 0\r\n\r\n", code)
				}()
				return a, nil
			}
			result := probeTarget(ctx, &vrxv1.WanMonitor{Type: proto.String("http"), Target: proto.String("probe.test")}, dial, nil, &sequence)
			want := 0
			if code < 400 {
				want = 1
			}
			if result.Received != want || calls.Load() != 1 {
				t.Fatalf("result %v calls%d", result, calls.Load())
			}
		})
	}
}

func TestDNSProbeValidatesExchange(t *testing.T) {
	for _, badID := range []bool{false, true} {
		t.Run(fmt.Sprint(badID), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var sequence atomic.Uint32
			dial := func(_ context.Context, network, address string) (net.Conn, error) {
				if network != "udp" || address != "192.0.2.53:53" {
					t.Errorf("unexpected DNS dial %s %s", network, address)
				}
				a, b := net.Pipe()
				go func() {
					defer func() { _ = b.Close() }()
					buffer := make([]byte, 4096)
					n, err := b.Read(buffer)
					if err != nil {
						return
					}
					var query dnsmessage.Message
					if query.Unpack(buffer[:n]) != nil {
						return
					}
					reply := dnsmessage.Message{Header: dnsmessage.Header{ID: query.ID, Response: true}, Questions: query.Questions, Answers: []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: dnsmessage.MustNewName("."), Type: dnsmessage.TypeNS, Class: dnsmessage.ClassINET, TTL: 1}, Body: &dnsmessage.NSResource{NS: dnsmessage.MustNewName("ns.example.")}}}}
					if badID {
						reply.ID++
					}
					packet, err := reply.Pack()
					if err == nil {
						_, _ = b.Write(packet)
					}
				}()
				return a, nil
			}
			result := probeTarget(ctx, &vrxv1.WanMonitor{Type: proto.String("dns"), Target: proto.String("192.0.2.53")}, dial, nil, &sequence)
			want := 1
			if badID {
				want = 0
			}
			if result.Received != want {
				t.Fatalf("result %v", result)
			}
		})
	}
}

func TestICMPProbeMatchesEchoAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var sequence atomic.Uint32
	resolve := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}}, nil
	}
	dial := func(context.Context, string, string) (net.Conn, error) {
		a, b := net.Pipe()
		go func() {
			defer func() { _ = b.Close() }()
			buffer := make([]byte, 2048)
			n, err := b.Read(buffer)
			if err != nil {
				return
			}
			request, err := icmp.ParseMessage(1, buffer[:n])
			if err != nil {
				return
			}
			reply := icmp.Message{Type: ipv4.ICMPTypeEchoReply, Body: request.Body}
			packet, err := reply.Marshal(nil)
			if err == nil {
				_, _ = b.Write(packet)
			}
		}()
		return a, nil
	}
	result := probeTarget(ctx, &vrxv1.WanMonitor{Type: proto.String("icmp"), Target: proto.String("192.0.2.1")}, dial, resolve, &sequence)
	if result.Received != 1 {
		t.Fatalf("echo %v", result)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	done := make(chan struct{})
	silent := func(context.Context, string, string) (net.Conn, error) {
		a, b := net.Pipe()
		go func() {
			defer close(done)
			defer func() { _ = b.Close() }()
			buffer := make([]byte, 2048)
			_, _ = b.Read(buffer)
			_, _ = b.Read(buffer)
		}()
		return a, nil
	}
	result = probeTarget(ctx2, &vrxv1.WanMonitor{Type: proto.String("icmp"), Target: proto.String("192.0.2.1")}, silent, resolve, &sequence)
	if result.Received != 0 {
		t.Fatalf("timeout %v", result)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("probe cancellation leaked peer")
	}
}

func TestDeviceProbeNeverFallsBackWithoutLCP(t *testing.T) {
	probe := DeviceProbe(func(string) (string, error) { return "", UnsupportedDevice() })
	if result := probe(context.Background(), "missing", runtimeGroup().Monitors[0]); result.Received != 0 {
		t.Fatalf("missing device: %v", result)
	}
}
