package ravpn

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

func managerDBusOwnedPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	var pair [2]*net.UnixConn
	for i, fd := range fds {
		file := os.NewFile(uintptr(fd), "owned-dbus-negative-socket")
		conn, err := net.FileConn(file)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatal("owned socket duplicate", err, closeErr)
		}
		var ok bool
		pair[i], ok = conn.(*net.UnixConn)
		if !ok {
			t.Fatal("unexpected owned socket type")
		}
		if pair[i].SetDeadline(time.Now().Add(2*time.Second)) != nil {
			t.Fatal("owned socket deadline")
		}
		t.Cleanup(func() {
			if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				t.Error(err)
			}
		})
	}
	return pair[0], pair[1]
}

func TestNumericPublisherManagerDBusGenericAuthenticationHasNoFDNegotiation(t *testing.T) {
	client, server := managerDBusOwnedPair(t)
	transport := &managerDBusTransport{conn: client, pending: make(map[uint32]bool)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	bus, err := dbus.NewConn(transport, dbus.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		first, err := reader.ReadBytes('\n')
		if err != nil {
			done <- err
			return
		}
		if !bytes.Equal(first, []byte("\x00AUTH\r\n")) {
			done <- ErrBoundary
			return
		}
		if _, err = server.Write([]byte("REJECTED EXTERNAL ANONYMOUS\r\n")); err != nil {
			done <- err
			return
		}
		auth, err := reader.ReadBytes('\n')
		if err != nil || !bytes.Equal(auth, []byte("AUTH EXTERNAL 30\r\n")) {
			done <- ErrBoundary
			return
		}
		if _, err = server.Write([]byte("OK 0123456789abcdef0123456789abcdef\r\n")); err != nil {
			done <- err
			return
		}
		begin, err := reader.ReadBytes('\n')
		if err != nil || !bytes.Equal(begin, []byte("BEGIN\r\n")) {
			done <- ErrBoundary
			return
		}
		done <- nil
	}()
	if err := bus.Auth([]dbus.Auth{dbus.AuthExternal("0")}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal("auth sent implicit FD negotiation/Hello or wrong UID", err)
	}
	if !transport.binary {
		t.Fatal("whole SDK BEGIN write did not enter binary phase")
	}
	callDone := make(chan error, 1)
	go func() {
		callDone <- bus.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1/unit/ngfw_2dagent_2eservice").CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", dbus.FlagNoAutoStart, "org.freedesktop.systemd1.Unit", "Id").Err
	}()
	// The authenticated SDK inbound worker remains blocked without a reply.
	// Cancellation must release both the pending call and transport reader.
	var header [16]byte
	if _, err := io.ReadFull(server, header[:]); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-callDone:
		if err == nil {
			t.Fatal("canceled SDK call succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("authenticated SDK call worker did not retire")
	}
	if err := transport.Close(); err != nil {
		t.Fatal("idempotent transport cleanup", err)
	}
}

func TestNumericPublisherManagerDBusTransportRejectsAuthOverflowAndCancelsRead(t *testing.T) {
	client, server := managerDBusOwnedPair(t)
	transport := &managerDBusTransport{conn: client, authStage: 2, pending: make(map[uint32]bool)}
	done := make(chan error, 1)
	go func() { _, err := server.Write(bytes.Repeat([]byte{'x'}, 256)); done <- err }()
	var b [1]byte
	if _, err := transport.Read(b[:]); err != ErrBoundary {
		t.Fatal("oversized auth line accepted")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	blocked := make(chan error, 1)
	go func() { _, err := transport.Read(b[:]); blocked <- err }()
	if err := transport.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-blocked:
		if err == nil {
			t.Fatal("closed read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("transport close did not join blocked read")
	}
}

func TestNumericPublisherManagerDBusTransportClosesUnexpectedRights(t *testing.T) {
	client, server := managerDBusOwnedPair(t)
	transport := &managerDBusTransport{conn: client, pending: make(map[uint32]bool)}
	file, err := os.Create(filepath.Join(t.TempDir(), "owned-right"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.WriteMsgUnix([]byte{'x'}, unix.UnixRights(int(file.Fd()), int(file.Fd()), int(file.Fd())), nil); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if err := transport.rawRead(b[:]); err != ErrBoundary {
		t.Fatal("unexpected rights accepted")
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("received descriptor leaked")
	}
	if managerDBusPeer(client, (bootid.Reader{}).ForPID(os.Getpid())) {
		t.Fatal("owned non-PID1 peer adopted as manager")
	}
}

func TestNumericPublisherManagerDBusTransportRejectsForeignReplyAndTotalOverflow(t *testing.T) {
	client, server := managerDBusOwnedPair(t)
	transport := &managerDBusTransport{conn: client, binary: true, pending: map[uint32]bool{8: true}}
	frame := managerDBusReplyFixture(t, "marker")
	done := make(chan error, 1)
	go func() { _, err := server.Write(frame); done <- err }()
	var b [1]byte
	if _, err := transport.Read(b[:]); err != ErrBoundary {
		t.Fatal("foreign reply serial accepted")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	transport.written = managerDBusLimit
	if _, err := transport.Write(append([]byte{'l', 1, 0, 1, 0, 0, 0, 0, 9, 0, 0, 0}, make([]byte, 4)...)); err != ErrBoundary {
		t.Fatal("outgoing total budget ignored")
	}
	transport.read = managerDBusLimit
	go func() { _, err := server.Write([]byte{1}); done <- err }()
	if err := transport.rawRead(b[:]); err != ErrBoundary {
		t.Fatal("incoming total budget ignored")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNumericPublisherManagerDBusTransportClosesRightsBeforeMalformedControl(t *testing.T) {
	fd, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	control := append(unix.UnixRights(fd), make([]byte, unix.SizeofCmsghdr)...)
	closeManagerDBusRights(control)
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatal("parseable rights before malformed control leaked", err)
	}
}

func TestNumericPublisherManagerDBusAncillaryTruncationStillClosesDeliveredRights(t *testing.T) {
	for _, flags := range []int{unix.MSG_CTRUNC, unix.MSG_TRUNC, unix.MSG_CTRUNC | unix.MSG_TRUNC} {
		fd, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		if rejectManagerDBusAncillary(unix.UnixRights(fd), flags) != ErrBoundary {
			t.Fatal("truncated ancillary data accepted")
		}
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err != unix.EBADF {
			t.Fatal("delivered rights leaked on truncation", err)
		}
		if rejectManagerDBusAncillary(nil, flags) != ErrBoundary {
			t.Fatal("truncation without delivered data accepted")
		}
	}
	if rejectManagerDBusAncillary(nil, 0) != nil {
		t.Fatal("plain no-rights transport rejected")
	}
}
