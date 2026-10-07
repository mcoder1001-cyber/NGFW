package ravpn

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

func TestNumericPublisherWorkFrameRejectsWrongPhaseAndIdentity(t *testing.T) {
	source := bootid.Identity{BootID: "kernel", PID: 12, StartTime: 34}
	server := bootid.Identity{BootID: "kernel", PID: 15, StartTime: 37}
	frame := numericPublisherWorkFrame{Version: 1, Phase: "publication-complete", Source: source, Server: server, Token: strings.Repeat("a", 64)}
	if validateNumericPublisherWorkFrame(frame, "publication-complete", source, server, frame.Token) != nil {
		t.Fatal("valid frame refused")
	}
	for _, alter := range []func(*numericPublisherWorkFrame){func(f *numericPublisherWorkFrame) { f.Phase = "publication-ack" }, func(f *numericPublisherWorkFrame) { f.Version = 2 }, func(f *numericPublisherWorkFrame) { f.Source.StartTime++ }, func(f *numericPublisherWorkFrame) { f.Server.StartTime++ }} {
		bad := frame
		alter(&bad)
		if validateNumericPublisherWorkFrame(bad, "publication-complete", source, server, frame.Token) == nil {
			t.Fatal("invalid transition accepted")
		}
	}
	for _, pid := range []int{0, -1, 1} {
		invalidSource := source
		invalidSource.PID = pid
		bad := frame
		bad.Source = invalidSource
		if validateNumericPublisherWorkFrame(bad, "publication-complete", invalidSource, server, frame.Token) == nil {
			t.Fatal("invalid source PID accepted")
		}
		invalidServer := server
		invalidServer.PID = pid
		bad = frame
		bad.Server = invalidServer
		if validateNumericPublisherWorkFrame(bad, "publication-complete", source, invalidServer, frame.Token) == nil {
			t.Fatal("invalid server PID accepted")
		}
	}

}

func TestNumericPublisherPeerWatcherPreservesQueuedPacketThenCancelsHUP(t *testing.T) {
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
	join := watchNumericPublisherPeer(ctx, pair[0], cancel)
	defer join()
	if err := unix.Sendmsg(pair[1], []byte("queued request"), nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("queued packet canceled work")
	}
	data, files, err := receiveUnitObserverPacket(pair[0], 0)
	closeUnitObserverFiles(files)
	if err != nil || string(data) != "queued request" {
		t.Fatal("watcher consumed packet")
	}
	if err := unix.Close(pair[1]); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			t.Fatal("HUP did not cancel work")
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("HUP cancellation stalled")
	}
}

func TestNumericPublisherWorkSocketClipsOriginalWholeDeadline(t *testing.T) {
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
	whole, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()
	work, workCancel := context.WithTimeout(whole, NumericPublisherWorkBudget)
	defer workCancel()
	if boundNumericPublisherWorkSocket(work, pair[0]) != nil {
		t.Fatal("work bound refused")
	}
	timeout, err := unix.GetsockoptTimeval(pair[0], unix.SOL_SOCKET, unix.SO_RCVTIMEO)
	if err != nil || timeout.Nano() > int64(100*time.Millisecond) {
		t.Fatal("short caller deadline widened")
	}
	started := time.Now()
	_, files, receiveErr := receiveUnitObserverPacket(pair[0], 0)
	closeUnitObserverFiles(files)
	if receiveErr == nil || time.Since(started) > 300*time.Millisecond {
		t.Fatal("socket receive exceeded original short whole deadline")
	}
	<-whole.Done()
	final, finalCancel := context.WithTimeout(whole, NumericPublisherIPCBudget)
	defer finalCancel()
	if boundUnitObserverSocket(final, pair[0]) == nil {
		t.Fatal("final IPC reset expired whole deadline")
	}
}

func TestNumericPublisherHUPCancelsAndReapsOwnedChild(t *testing.T) {
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
	join := watchNumericPublisherPeer(ctx, pair[0], cancel)
	defer join()
	// A fixed owned disposable child proves actual cancellation/reaping; no
	// manager command or service is invoked by this portable test.
	child := exec.CommandContext(ctx, "/usr/bin/sleep", "30")
	child.WaitDelay = 100 * time.Millisecond
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(pair[1]); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- child.Wait() }()
	select {
	case err := <-waited:
		if err == nil || child.ProcessState == nil || ctx.Err() != context.Canceled {
			t.Fatal("child was not canceled and reaped")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("owned child reaping stalled")
	}
}

func TestNumericPublisherWrongRightsClosedBeforeFailure(t *testing.T) {
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
	pipe := make([]int, 2)
	if err := unix.Pipe2(pipe, unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Close(pipe[0]); err != nil {
			t.Error(err)
		}
	}()
	if err := unix.Sendmsg(pair[1], []byte("early ack"), unix.UnixRights(pipe[1]), nil, 0); err != nil {
		t.Fatal(err)
	}
	_, files, err := receiveUnitObserverPacket(pair[0], 0)
	closeUnitObserverFiles(files)
	if err == nil {
		t.Fatal("acknowledgment rights accepted")
	}
	if err := unix.Close(pipe[1]); err != nil {
		t.Fatal(err)
	}
	var data [1]byte
	if n, err := unix.Read(pipe[0], data[:]); n != 0 || err != nil {
		t.Fatal("rejected ancillary writer leaked instead of EOF")
	}
}

func TestNumericPublisherSocketRejectsShapedEarlyAndDuplicateACK(t *testing.T) {
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
	source := bootid.Identity{BootID: "kernel", PID: 12, StartTime: 34}
	server := bootid.Identity{BootID: "kernel", PID: 15, StartTime: 37}
	// This is correctly shaped and identity-bound, unlike an invalid-rights ACK,
	// but arrives before work completes and before the fresh token exists.
	if sendNumericPublisherWorkFrame(pair[1], "publication-ack", source, server, strings.Repeat("a", 64), -1) != nil {
		t.Fatal("send early ACK")
	}
	token, err := newNumericPublisherWorkToken()
	if err != nil {
		t.Fatal(err)
	}
	data, files, err := receiveUnitObserverPacket(pair[0], 0)
	closeUnitObserverFiles(files)
	var early numericPublisherWorkFrame
	if err != nil || decodeUnitObserverPacket(data, &early) != nil {
		t.Fatal("receive shaped early ACK")
	}
	if validateNumericPublisherWorkFrame(early, "publication-ack", source, server, token) == nil {
		t.Fatal("early ACK accepted")
	}
	for range 2 {
		if sendNumericPublisherWorkFrame(pair[1], "publication-ack", source, server, token, -1) != nil {
			t.Fatal("send duplicate ACK")
		}
	}
	data, files, err = receiveUnitObserverPacket(pair[0], 0)
	closeUnitObserverFiles(files)
	var first numericPublisherWorkFrame
	if err != nil || decodeUnitObserverPacket(data, &first) != nil || validateNumericPublisherWorkFrame(first, "publication-ack", source, server, token) != nil {
		t.Fatal("valid ACK refused")
	}
	if noNumericPublisherQueuedInput(pair[0]) == nil {
		t.Fatal("duplicate ACK permitted final reply")
	}
	data, files, err = receiveUnitObserverPacket(pair[0], 0)
	closeUnitObserverFiles(files)
	if err != nil || len(data) == 0 || noNumericPublisherQueuedInput(pair[0]) != nil {
		t.Fatal("queue boundary consumed duplicate instead of detecting it")
	}
}
