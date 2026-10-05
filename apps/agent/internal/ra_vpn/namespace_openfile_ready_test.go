package ravpn

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

func TestNumericPublisherReadyRejectsStaleAndForeign(t *testing.T) {
	source := bootid.Identity{BootID: "12345678-1234-1234-1234-123456789abc", PID: 100, StartTime: 7}
	server := source
	server.PID = 101
	good := numericPublisherReady{Version: 1, Phase: "validation-ready", Source: source, Server: server}
	if validateNumericPublisherReady(good, source) != nil {
		t.Fatal("complete fresh frame refused")
	}
	for _, mutate := range []func(*numericPublisherReady){
		func(f *numericPublisherReady) { f.Version = 2 },
		func(f *numericPublisherReady) { f.Phase = "publish" },
		func(f *numericPublisherReady) { f.Source.StartTime++ },
		func(f *numericPublisherReady) { f.Server = source },
		func(f *numericPublisherReady) { f.Server.StartTime = 0 },
	} {
		frame := good
		mutate(&frame)
		if validateNumericPublisherReady(frame, source) == nil {
			t.Fatal("foreign or incomplete readiness accepted")
		}
	}
}

func TestNumericPublisherValidationDeadlineSeparateFromIPC(t *testing.T) {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unix.Close(fd) != nil {
			t.Error("socket close")
		}
	}()
	if boundNumericPublisherValidationSocket(context.Background(), fd) != nil {
		t.Fatal("validation budget refused")
	}
	validation, err := unix.GetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO)
	if err != nil || validation.Sec != int64(NumericPublisherValidationBudget/time.Second) {
		t.Fatal("validation still clamped to IPC budget")
	}
	if boundUnitObserverSocket(context.Background(), fd) != nil {
		t.Fatal("IPC budget refused")
	}
	ipc, err := unix.GetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO)
	if err != nil || ipc.Sec != int64(NumericPublisherIPCBudget/time.Second) {
		t.Fatal("IPC budget widened")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if boundNumericPublisherValidationSocket(canceled, fd) == nil {
		t.Fatal("caller cancellation ignored")
	}
	short, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if boundNumericPublisherValidationSocket(short, fd) != nil {
		t.Fatal("short caller deadline refused")
	}
	bounded, err := unix.GetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO)
	if err != nil || bounded.Sec > 1 {
		t.Fatal("short caller deadline extended")
	}
}
