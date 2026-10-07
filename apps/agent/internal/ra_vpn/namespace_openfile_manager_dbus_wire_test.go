package ravpn

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/godbus/dbus/v5"
)

func managerDBusReplyFixture(t *testing.T, value any) []byte {
	t.Helper()
	variant := dbus.MakeVariant(value)
	message := dbus.Message{Type: dbus.TypeMethodReply, Headers: map[dbus.HeaderField]dbus.Variant{dbus.FieldReplySerial: dbus.MakeVariant(uint32(7)), dbus.FieldSignature: dbus.MakeVariant(dbus.SignatureOf(variant))}, Body: []any{variant}}
	var buffer bytes.Buffer
	if err := message.EncodeTo(&buffer, binary.LittleEndian); err != nil {
		t.Fatal(err)
	}
	frame := buffer.Bytes()
	binary.LittleEndian.PutUint32(frame[8:12], 1)
	return frame
}

func TestNumericPublisherManagerDBusWireValidClosedVariants(t *testing.T) {
	values := []any{"marker", []string{"first", "second"}, uint32(1), uint64(0), true, []managerDBusListen{{"SequentialPacket", "/run/ngfw-ra-ipc/openfile.sock"}}, []managerDBusExec{{Path: "/usr/sbin/ngfw-agent", Arguments: []string{"/usr/sbin/ngfw-agent"}}}}
	for _, value := range values {
		frame := managerDBusReplyFixture(t, value)
		if serial, err := validateManagerDBusFrame(frame); err != nil || serial != 7 {
			t.Fatalf("valid %T refused: %v", value, err)
		}
		if _, err := dbus.DecodeMessage(bytes.NewReader(frame)); err != nil {
			t.Fatalf("accepted frame must decode: %v", err)
		}
		// systemd259 marks replies NO_REPLY_EXPECTED. This is not authority.
		frame[2] = 1
		if _, err := validateManagerDBusFrame(frame); err != nil {
			t.Fatal("actual sd-bus reply flag refused", err)
		}
		frame[2] = 2
		if _, err := validateManagerDBusFrame(frame); err == nil {
			t.Fatal("unsupported reply flag accepted")
		}
	}
}

func TestNumericPublisherManagerDBusWireRefusesBeforeDecoderAllocation(t *testing.T) {
	valid := managerDBusReplyFixture(t, "marker")
	cases := map[string][]byte{}
	for _, name := range []string{"huge-inner-string", "huge-header", "huge-body", "zero-serial", "wrong-protocol", "signal", "trailing", "bad-nul", "bad-padding"} {
		cases[name] = append([]byte(nil), valid...)
	}
	index := bytes.Index(cases["huge-inner-string"], []byte("marker"))
	binary.LittleEndian.PutUint32(cases["huge-inner-string"][index-4:index], 0xffffffff)
	binary.LittleEndian.PutUint32(cases["huge-header"][12:16], 0xffffffff)
	binary.LittleEndian.PutUint32(cases["huge-body"][4:8], 0xffffffff)
	binary.LittleEndian.PutUint32(cases["zero-serial"][8:12], 0)
	cases["wrong-protocol"][3] = 2
	cases["signal"][1] = 4
	cases["trailing"] = append(cases["trailing"], 0)
	cases["bad-nul"][len(valid)-1] = 1
	end := 16 + int(binary.LittleEndian.Uint32(valid[12:16]))
	padEnd := (end + 7) &^ 7
	if padEnd > end {
		cases["bad-padding"][end] = 1
	} else {
		delete(cases, "bad-padding")
	}
	cases["map"] = managerDBusReplyFixture(t, map[string]string{"foreign": "value"})
	cases["property-signature"] = managerDBusReplyFixture(t, dbus.SignatureOf("marker"))
	cases["rights-signature"] = managerDBusReplyFixture(t, dbus.UnixFDIndex(1))
	cases["unknown-fixed-array"] = managerDBusReplyFixture(t, []uint64{1})
	for name, frame := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := validateManagerDBusFrame(frame); err != ErrBoundary {
				t.Fatal("malformed frame reached general decoder")
			}
		})
	}
	for end := 0; end < len(valid); end++ {
		if _, err := validateManagerDBusFrame(valid[:end]); err == nil {
			t.Fatal("truncation accepted", end)
		}
	}
}

func TestNumericPublisherManagerDBusWireDuplicateHeaderAndArrayLengths(t *testing.T) {
	valid := managerDBusReplyFixture(t, []string{"marker"})
	headerEnd := 16 + int(binary.LittleEndian.Uint32(valid[12:16]))
	bodyStart := (headerEnd + 7) &^ 7
	duplicate := append([]byte(nil), valid[:bodyStart]...)
	duplicate = append(duplicate, 5, 1, 'u', 0, 7, 0, 0, 0)
	duplicate = append(duplicate, valid[bodyStart:]...)
	binary.LittleEndian.PutUint32(duplicate[12:16], uint32(bodyStart-16+8)) // #nosec G115 -- fixed bounded encoded fixture header is below the 16KiB frame limit.
	if _, err := validateManagerDBusFrame(duplicate); err == nil {
		t.Fatal("duplicate header overwritten by decoder")
	}
	array := append([]byte(nil), valid...)
	position := (bodyStart + 4 + 3) &^ 3
	binary.LittleEndian.PutUint32(array[position:position+4], 0xffffffff)
	if _, err := validateManagerDBusFrame(array); err == nil {
		t.Fatal("oversized inner array accepted")
	}
}

func FuzzNumericPublisherManagerDBusWire(f *testing.F) {
	f.Add([]byte("malformed"))
	f.Fuzz(func(_ *testing.T, frame []byte) {
		if len(frame) > managerDBusLimit {
			return
		}
		_, _ = validateManagerDBusFrame(frame)
	})
}
