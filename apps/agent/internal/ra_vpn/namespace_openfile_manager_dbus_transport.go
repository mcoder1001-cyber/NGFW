package ravpn

import (
	"bytes"
	"encoding/hex"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// managerDBusTransport supplies only prevalidated bounded bytes to godbus's
// generic transport, which does not negotiate or accept Unix descriptors.
type managerDBusTransport struct {
	conn      *net.UnixConn
	mu        sync.Mutex
	readMu    sync.Mutex
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
	binary    bool
	authStage uint8
	written   int
	read      int
	buffer    []byte
	pending   map[uint32]bool
}

func (t *managerDBusTransport) Close() error {
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		t.closeErr = t.conn.Close()
		t.readMu.Lock()
		t.buffer = nil // Reader joined; discard any remaining authenticated reply bytes.
		t.readMu.Unlock()
	})
	return t.closeErr
}

// closeManagerDBusRights closes each parseable rights message before rejecting
// the control buffer, even when a subsequent control message is malformed.
func closeManagerDBusRights(control []byte) {
	for len(control) >= unix.SizeofCmsghdr {
		header, data, rest, err := unix.ParseOneSocketControlMessage(control)
		if err != nil {
			return
		}
		if header.Level == unix.SOL_SOCKET && header.Type == unix.SCM_RIGHTS {
			message := unix.SocketControlMessage{Header: header, Data: data}
			fds, err := unix.ParseUnixRights(&message)
			if err == nil {
				for _, fd := range fds {
					_ = unix.Close(fd)
				}
			}
		}
		if len(rest) >= len(control) {
			return
		}
		control = rest
	}
}

func rejectManagerDBusAncillary(control []byte, flags int) error {
	if len(control) != 0 {
		closeManagerDBusRights(control)
	}
	if len(control) != 0 || flags&(unix.MSG_CTRUNC|unix.MSG_TRUNC) != 0 {
		return ErrBoundary
	}
	return nil
}

func (t *managerDBusTransport) rawRead(dst []byte) error {
	for len(dst) > 0 {
		var control [4096]byte
		n, oob, flags, _, err := t.conn.ReadMsgUnix(dst, control[:])
		if rejectManagerDBusAncillary(control[:oob], flags) != nil {
			return ErrBoundary
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		t.read += n
		if t.read > managerDBusLimit {
			return ErrBoundary
		}
		dst = dst[n:]
	}
	return nil
}

func (t *managerDBusTransport) Read(dst []byte) (int, error) {
	t.readMu.Lock()
	defer t.readMu.Unlock()
	if t.closed.Load() {
		return 0, net.ErrClosed
	}
	if len(dst) == 0 {
		return 0, nil
	}
	if len(t.buffer) == 0 {
		t.mu.Lock()
		binaryPhase := t.binary
		authStage := t.authStage
		t.mu.Unlock()
		if !binaryPhase {
			line := make([]byte, 0, 256)
			for len(line) < 256 {
				var b [1]byte
				if err := t.rawRead(b[:]); err != nil {
					return 0, err
				}
				line = append(line, b[0])
				if b[0] == '\n' {
					break
				}
			}
			if len(line) < 2 || line[len(line)-1] != '\n' || line[len(line)-2] != '\r' {
				return 0, ErrBoundary
			}
			switch authStage {
			case 2:
				if !bytes.Equal(line, []byte("REJECTED EXTERNAL\r\n")) && !bytes.Equal(line, []byte("REJECTED EXTERNAL ANONYMOUS\r\n")) {
					return 0, ErrBoundary
				}
			case 3:
				if len(line) != 37 || !bytes.HasPrefix(line, []byte("OK ")) {
					return 0, ErrBoundary
				}
				if _, err := hex.DecodeString(string(line[3:35])); err != nil {
					return 0, ErrBoundary
				}
			default:
				return 0, ErrBoundary
			}
			t.buffer = line
		} else {
			var header [16]byte
			if err := t.rawRead(header[:]); err != nil {
				return 0, err
			}
			size, _, ok := managerDBusFrameSize(header[:])
			if !ok || size-16 > managerDBusLimit-t.read {
				return 0, ErrBoundary
			}
			frame := make([]byte, size)
			copy(frame, header[:])
			if err := t.rawRead(frame[16:]); err != nil {
				return 0, err
			}
			reply, err := validateManagerDBusFrame(frame)
			if err != nil {
				return 0, err
			}
			t.mu.Lock()
			valid := t.pending[reply]
			delete(t.pending, reply)
			t.mu.Unlock()
			if !valid {
				return 0, ErrBoundary
			}
			t.buffer = frame
		}
	}
	n := copy(dst, t.buffer)
	t.buffer = t.buffer[n:]
	return n, nil
}

func (t *managerDBusTransport) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	originalLength := len(data)
	if t.closed.Load() || len(data) == 0 {
		return 0, ErrBoundary
	}
	if t.binary {
		if len(data) < 16 || data[1] != 1 || data[3] != 1 {
			return 0, ErrBoundary
		}
		var serial uint32
		switch data[0] {
		case 'l':
			serial = uint32(data[8]) | uint32(data[9])<<8 | uint32(data[10])<<16 | uint32(data[11])<<24
		case 'B':
			serial = uint32(data[11]) | uint32(data[10])<<8 | uint32(data[9])<<16 | uint32(data[8])<<24
		default:
			return 0, ErrBoundary
		}
		if serial == 0 || t.pending[serial] {
			return 0, ErrBoundary
		}
		t.pending[serial] = true
	} else {
		switch t.authStage {
		case 0:
			if !bytes.Equal(data, []byte{0}) {
				return 0, ErrBoundary
			}
		case 1:
			if !bytes.Equal(data, []byte("AUTH\r\n")) {
				return 0, ErrBoundary
			}
		case 2:
			if !bytes.Equal(data, []byte("AUTH EXTERNAL\r\n")) {
				return 0, ErrBoundary
			}
			// Pinned godbus drops FirstData's UID argument. Enforce the explicit
			// fixed UID0 on wire, while retaining its generic authentication parser.
			data = []byte("AUTH EXTERNAL 30\r\n")
		case 3:
			if !bytes.Equal(data, []byte("BEGIN\r\n")) {
				return 0, ErrBoundary
			}
			t.binary = true
		default:
			return 0, ErrBoundary
		}
		t.authStage++
	}
	if len(data) > managerDBusLimit-t.written {
		return 0, ErrBoundary
	}
	n, err := t.conn.Write(data)
	t.written += n
	if n != len(data) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return 0, err
	}
	return originalLength, err
}
