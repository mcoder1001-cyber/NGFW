package ravpn

import (
	"context"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestNumericPublisherTerminalQueuedOKAndNormalClose(t *testing.T) {
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Close(pair[0]); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stop := watchNumericPublisherPeer(ctx, pair[0], cancel)
	defer stop()
	if enterNumericPublisherTerminal(ctx, pair[0], stop) != nil {
		t.Fatal("terminal ownership refused")
	}
	if err := unix.Sendmsg(pair[0], []byte("final reply"), nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	reply, rights, err := receiveUnitObserverPacket(pair[1], 0)
	closeUnitObserverFiles(rights)
	if err != nil || string(reply) != "final reply" {
		t.Fatal("final reply missing")
	}
	if err := unix.Sendmsg(pair[1], []byte("OK"), nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(pair[1]); err != nil {
		t.Fatal(err)
	}
	// Give the exact watcher multiple former poll periods to reproduce the
	// observed OK+HUP ordering. It must already be joined, not cancel this phase.
	time.Sleep(110 * time.Millisecond)
	if receiveNumericPublisherTerminalACK(ctx, pair[0]) != nil || ctx.Err() != nil {
		t.Fatal("normal queued OK and close canceled completion")
	}
}

func TestNumericPublisherTerminalEarlyCloseWithoutOK(t *testing.T) {
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Close(pair[0]); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stop := watchNumericPublisherPeer(ctx, pair[0], cancel)
	defer stop()
	if enterNumericPublisherTerminal(ctx, pair[0], stop) != nil {
		t.Fatal("terminal ownership refused")
	}
	if err := unix.Close(pair[1]); err != nil {
		t.Fatal(err)
	}
	if receiveNumericPublisherTerminalACK(ctx, pair[0]) == nil {
		t.Fatal("EOF accepted as OK")
	}
}

func TestNumericPublisherTerminalStopIdempotentConcurrentJoin(t *testing.T) {
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, fd := range pair {
			if err := unix.Close(fd); err != nil {
				t.Error(err)
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stop := watchNumericPublisherPeer(ctx, pair[0], cancel)
	var joined sync.WaitGroup
	for range 8 {
		joined.Go(func() { stop() })
	}
	joined.Wait()
	stop()
	if ctx.Err() != nil {
		t.Fatal("joining canceled valid context")
	}
}

func TestNumericPublisherTerminalRebindClipsOriginalRemainingDeadline(t *testing.T) {
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, fd := range pair {
			if err := unix.Close(fd); err != nil {
				t.Error(err)
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	stop := watchNumericPublisherPeer(ctx, pair[0], cancel)
	defer stop()
	// This models completed manager cost before terminal handoff. Rebinding
	// must clip the original deadline, rather than restart another IPC5 clock.
	time.Sleep(70 * time.Millisecond)
	if enterNumericPublisherTerminal(ctx, pair[0], stop) != nil {
		t.Fatal("handoff refused")
	}
	timeout, err := unix.GetsockoptTimeval(pair[0], unix.SOL_SOCKET, unix.SO_RCVTIMEO)
	if err != nil || timeout.Nano() > int64(100*time.Millisecond) {
		t.Fatal("terminal timeout widened original remaining budget")
	}
	started := time.Now()
	if receiveNumericPublisherTerminalACK(ctx, pair[0]) == nil || time.Since(started) > 300*time.Millisecond {
		t.Fatal("terminal timeout failed finite original bound")
	}
	<-ctx.Done()
	if enterNumericPublisherTerminal(ctx, pair[0], stop) == nil {
		t.Fatal("expired context reset after join")
	}
}
