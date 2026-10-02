//go:build linux

package multiwan

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
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
	dial := func(_ context.Context, network, _ string) (net.Conn, error) {
		if network != "ping4" {
			t.Errorf("ICMP must use restricted ping socket: %s", network)
		}
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
			echo := request.Body.(*icmp.Echo)
			if echo.ID != 31337 {
				t.Errorf("kernel-assigned echo ID ignored: %d", echo.ID)
			}
			wrong := *echo
			wrong.ID++
			wrongReply := icmp.Message{Type: ipv4.ICMPTypeEchoReply, Body: &wrong}
			wrongPacket, _ := wrongReply.Marshal(nil)
			_, _ = b.Write(wrongPacket)
			reply := icmp.Message{Type: ipv4.ICMPTypeEchoReply, Body: request.Body}
			packet, err := reply.Marshal(nil)
			if err == nil {
				_, _ = b.Write(packet)
			}
		}()
		return &pingConn{Conn: a, id: 31337}, nil
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

func TestICMPPolicyFailureIsUnavailableNotPacketLoss(t *testing.T) {
	for _, cause := range []error{unix.EACCES, unix.EPERM, unix.EPROTONOSUPPORT, unix.ESOCKTNOSUPPORT} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		var sequence atomic.Uint32
		resolve := func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}}, nil
		}
		dial := func(context.Context, string, string) (net.Conn, error) { return nil, pingSocketError(cause) }
		result := probeTarget(ctx, &vrxv1.WanMonitor{Type: proto.String("icmp"), Target: proto.String("192.0.2.1")}, dial, resolve, &sequence)
		cancel()
		if !result.Unavailable || result.Received != 0 {
			t.Fatalf("policy failure misreported: %v", result)
		}
	}
	if errors.Is(pingSocketError(unix.ENETUNREACH), ErrICMPUnavailable) {
		t.Fatal("ordinary route failure mislabeled unsupported")
	}
}

// Opens and closes only; sends no packets and never changes host policy.
func TestPingSocketSetupUsesDatagramOrExplicitPolicyError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dialPing(ctx, "lo", "127.0.0.1")
	if errors.Is(err, ErrICMPUnavailable) {
		t.Log("host policy denies ping socket; explicit unavailable path verified, positive socket path NOT EXERCISED")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ping, ok := conn.(*pingConn)
	if !ok || ping.EchoID() == 0 {
		t.Fatal("missing kernel-assigned echo identifier")
	}
	raw, err := ping.Conn.(syscall.Conn).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Control(func(fd uintptr) {
		kind, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_TYPE)
		if err != nil || kind != unix.SOCK_DGRAM {
			t.Errorf("not a datagram socket: %d %v", kind, err)
		}
		protocol, err := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PROTOCOL)
		if err != nil || protocol != unix.IPPROTO_ICMP {
			t.Errorf("not ICMP: %d %v", protocol, err)
		}
		device, err := unix.GetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE)
		if err != nil || device != "lo" {
			t.Errorf("unbound device: %q %v", device, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
}
