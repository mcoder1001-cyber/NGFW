package ravpn

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// Encode the real public systemd signal signatures with the pinned SDK; only
// its private serial field is assigned after encoding, as with reply fixtures.
func managerDBusSignalFixture(t *testing.T, path, iface, member string, body []any, extra map[dbus.HeaderField]dbus.Variant, order binary.ByteOrder) []byte {
	t.Helper()
	fields := map[dbus.HeaderField]dbus.Variant{dbus.FieldPath: dbus.MakeVariant(dbus.ObjectPath(path)), dbus.FieldInterface: dbus.MakeVariant(iface), dbus.FieldMember: dbus.MakeVariant(member)}
	if len(body) > 0 {
		fields[dbus.FieldSignature] = dbus.MakeVariant(dbus.SignatureOf(body...))
	}
	for key, value := range extra {
		fields[key] = value
	}
	m := dbus.Message{Type: dbus.TypeSignal, Flags: dbus.FlagNoReplyExpected, Headers: fields, Body: body}
	var b bytes.Buffer
	if err := m.EncodeTo(&b, order); err != nil {
		t.Fatal(err)
	}
	frame := b.Bytes()
	order.PutUint32(frame[8:12], 1)
	return frame
}

func TestManagerDBusSignalClosedPublicWhitelist(t *testing.T) {
	const p = "/org/freedesktop/systemd1"
	const iface = "org.freedesktop.systemd1.Manager"
	cases := []struct {
		member string
		body   []any
	}{
		{"UnitNew", []any{"ngfw-agent.service", dbus.ObjectPath(p + "/unit/ngfw_2dagent_2eservice")}},
		{"UnitRemoved", []any{"ngfw-agent.service", dbus.ObjectPath(p + "/unit/ngfw_2dagent_2eservice")}},
		{"JobNew", []any{uint32(1), dbus.ObjectPath(p + "/job/1"), "ngfw-agent.service"}},
		{"JobRemoved", []any{uint32(1), dbus.ObjectPath(p + "/job/1"), "ngfw-agent.service", "done"}},
		{"Reloading", []any{true}}, {"StartupFinished", []any{uint64(1), uint64(2), uint64(3), uint64(4), uint64(5), uint64(6)}},
		{"UnitFilesChanged", nil},
	}
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for _, c := range cases {
			frame := managerDBusSignalFixture(t, p, iface, c.member, c.body, nil, order)
			if err := validateManagerDBusSignalFrame(frame); err != nil {
				t.Fatalf("%s/%T: %v", c.member, order, err)
			}
			if _, err := validateManagerDBusFrame(frame); err == nil {
				t.Fatal("signal admitted as property reply")
			}
		}
	}
	job := managerDBusSignalFixture(t, p+"/job/123", "org.freedesktop.DBus.Properties", "PropertiesChanged", []any{"org.freedesktop.systemd1.Job", map[string]dbus.Variant{"State": dbus.MakeVariant("running")}, []string{}}, nil, binary.LittleEndian)
	if err := validateManagerDBusSignalFrame(job); err != nil {
		t.Fatal("public numeric job path refused", err)
	}
	for _, path := range []string{p + "/job/", p + "/job/1/extra", p + "/job/-1", p + "/unit/", p + "/unit/a/extra"} {
		if managerDBusSignalPath(path) {
			t.Fatalf("invalid public object path accepted: %s", path)
		}
	}
	frame := managerDBusSignalFixture(t, p+"/unit/ngfw_2dagent_2eservice", "org.freedesktop.DBus.Properties", "PropertiesChanged", []any{"org.freedesktop.systemd1.Unit", map[string]dbus.Variant{"ActiveState": dbus.MakeVariant("active")}, []string{}}, nil, binary.LittleEndian)
	if err := validateManagerDBusSignalFrame(frame); err != nil {
		t.Fatal("real map-bearing public signal refused", err)
	}
	// Bodies are never interpreted. Arbitrary bounded bytes cannot become a
	// property value or a successful pending reply through this discard check.
	end := 16 + int(binary.LittleEndian.Uint32(frame[12:16]))
	end = (end + 7) &^ 7
	for i := end; i < len(frame); i++ {
		frame[i] = 0xff
	}
	if err := validateManagerDBusSignalFrame(frame); err != nil {
		t.Fatal("opaque bounded body interpreted", err)
	}
	if _, err := validateManagerDBusFrame(frame); err == nil {
		t.Fatal("opaque body obtained reply authority")
	}
}

func TestManagerDBusSignalRejectsForeignMetadataAndFraming(t *testing.T) {
	const p = "/org/freedesktop/systemd1"
	const iface = "org.freedesktop.systemd1.Manager"
	body := []any{"ngfw-agent.service", dbus.ObjectPath(p + "/unit/ngfw_2dagent_2eservice")}
	for _, c := range []struct {
		name, path, iface, member string
		body                      []any
		extra                     map[dbus.HeaderField]dbus.Variant
	}{
		{"foreign-path", "/foreign", iface, "UnitNew", body, nil},
		{"foreign-interface", p, "org.foreign.Manager", "UnitNew", body, nil},
		{"foreign-member", p, iface, "Unknown", body, nil},
		{"wrong-signature", p, iface, "UnitNew", []any{"one"}, nil},
		{"signal-reply-serial", p, iface, "UnitNew", body, map[dbus.HeaderField]dbus.Variant{dbus.FieldReplySerial: dbus.MakeVariant(uint32(7))}},
		{"unix-fd-header", p, iface, "UnitNew", body, map[dbus.HeaderField]dbus.Variant{dbus.FieldUnixFDs: dbus.MakeVariant(uint32(1))}},
		{"foreign-property-path", "/org/freedesktop/systemd1/job/non_numeric", "org.freedesktop.DBus.Properties", "PropertiesChanged", []any{"org.freedesktop.systemd1.Unit", map[string]dbus.Variant{}, []string{}}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := validateManagerDBusSignalFrame(managerDBusSignalFixture(t, c.path, c.iface, c.member, c.body, c.extra, binary.LittleEndian)); err != ErrBoundary {
				t.Fatalf("foreign metadata accepted: %v", err)
			}
		})
	}
	valid := managerDBusSignalFixture(t, p, iface, "UnitNew", body, nil, binary.LittleEndian)
	for _, name := range []string{"version", "flags", "zero-serial", "huge-header", "huge-body", "truncated", "trailing", "padding"} {
		t.Run(name, func(t *testing.T) {
			f := append([]byte(nil), valid...)
			switch name {
			case "version":
				f[3] = 2
			case "flags":
				f[2] = 2
			case "zero-serial":
				binary.LittleEndian.PutUint32(f[8:12], 0)
			case "huge-header":
				binary.LittleEndian.PutUint32(f[12:16], 0xffffffff)
			case "huge-body":
				binary.LittleEndian.PutUint32(f[4:8], 0xffffffff)
			case "truncated":
				f = f[:len(f)-1]
			case "trailing":
				f = append(f, 0)
			case "padding":
				headerEnd := 16 + int(binary.LittleEndian.Uint32(f[12:16]))
				pathEnd := bytes.Index(f[:headerEnd], []byte(p)) + len(p) + 1
				if pathEnd%8 == 0 || f[pathEnd] != 0 {
					t.Fatal("expected SDK path alignment padding")
				}
				f[pathEnd] = 1
			}
			if err := validateManagerDBusSignalFrame(f); err != ErrBoundary {
				t.Fatalf("malformed signal accepted: %v", err)
			}
		})
	}
}

func TestManagerDBusSignalBeforePendingReply(t *testing.T) {
	client, server := managerDBusOwnedPair(t)
	transport := &managerDBusTransport{conn: client, binary: true, pending: map[uint32]bool{7: true}}
	signal := managerDBusSignalFixture(t, "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager", "Reloading", []any{true}, nil, binary.LittleEndian)
	reply := managerDBusReplyFixture(t, "marker")
	done := make(chan error, 1)
	go func() { _, err := server.Write(append(signal, reply...)); done <- err }()
	got := make([]byte, len(reply))
	if _, err := io.ReadFull(transport, got); err != nil {
		t.Fatal("lawful signal killed pending property reply", err)
	}
	if !bytes.Equal(got, reply) || transport.pending[7] || transport.read != len(signal)+len(reply) {
		t.Fatal("signal leaked, consumed reply authority, or escaped original budget")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestManagerDBusSignalTransportKeepsBoundaryAndOriginalBudget(t *testing.T) {
	signal := managerDBusSignalFixture(t, "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager", "Reloading", []any{true}, nil, binary.LittleEndian)
	for _, name := range []string{"flood", "orphan-reply", "unknown-metadata", "count-limit"} {
		t.Run(name, func(t *testing.T) {
			client, server := managerDBusOwnedPair(t)
			transport := &managerDBusTransport{conn: client, binary: true, pending: map[uint32]bool{8: true}}
			var wire []byte
			switch name {
			case "flood":
				wire = bytes.Repeat(signal, managerDBusLimit/len(signal)+1)
			case "orphan-reply":
				wire = append(append([]byte(nil), signal...), managerDBusReplyFixture(t, "marker")...)
			case "unknown-metadata":
				wire = managerDBusSignalFixture(t, "/org/freedesktop/systemd1", "org.foreign.Manager", "Reloading", []any{true}, nil, binary.LittleEndian)
			case "count-limit":
				transport.signals = 1024
				wire = signal
			}
			done := make(chan error, 1)
			go func() { _, err := server.Write(wire); done <- err }()
			var dst [1]byte
			if n, err := transport.Read(dst[:]); n != 0 || err != ErrBoundary {
				t.Fatalf("%s bypassed boundary: n=%d err=%v", name, n, err)
			}
			if !transport.pending[8] || len(transport.buffer) != 0 {
				t.Fatal("discard mutated pending authority, delivered bytes, or enlarged read budget")
			}
			// The unchanged rawRead counts the refused final fixed header too;
			// no frame/body past the original connection budget is admitted.
			if name == "flood" && (transport.read <= managerDBusLimit-len(signal) || transport.read > managerDBusLimit+16) {
				t.Fatal("flood did not stop at original read budget", transport.read)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestManagerDBusSignalTransportClosesRightsAtHeaderAndBody(t *testing.T) {
	for _, atBody := range []bool{false, true} {
		t.Run(fmt.Sprint(atBody), func(t *testing.T) {
			client, server := managerDBusOwnedPair(t)
			transport := &managerDBusTransport{conn: client, binary: true, pending: map[uint32]bool{7: true}}
			frame := managerDBusSignalFixture(t, "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager", "Reloading", []any{true}, nil, binary.LittleEndian)
			file, err := os.Open("/dev/null")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			before, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				if atBody {
					if _, err := server.Write(frame[:16]); err != nil {
						done <- err
						return
					}
					frame = frame[16:]
				}
				_, _, err := server.WriteMsgUnix(frame, unix.UnixRights(int(file.Fd())), nil)
				done <- err
			}()
			var dst [1]byte
			if n, err := transport.Read(dst[:]); n != 0 || err != ErrBoundary {
				t.Fatal("signal rights accepted", n, err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				t.Fatal(err)
			}
			if len(before) != len(after) || !transport.pending[7] {
				t.Fatal("signal rights leaked or consumed pending serial")
			}
		})
	}
}
