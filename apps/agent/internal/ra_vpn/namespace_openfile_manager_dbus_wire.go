package ravpn

import (
	"encoding/binary"
	"strings"
	"unicode/utf8"
)

const managerDBusLimit = 16384

// managerDBusCursor validates a closed wire subset before the general decoder
// sees any attacker-controlled inner allocation length.
type managerDBusCursor struct {
	data  []byte
	pos   int
	order binary.ByteOrder
}

func (c *managerDBusCursor) align(n int) bool {
	end := (c.pos + n - 1) &^ (n - 1)
	if end > len(c.data) {
		return false
	}
	for _, b := range c.data[c.pos:end] {
		if b != 0 {
			return false
		}
	}
	c.pos = end
	return true
}

func (c *managerDBusCursor) take(n int) ([]byte, bool) {
	if n < 0 || n > len(c.data)-c.pos {
		return nil, false
	}
	v := c.data[c.pos : c.pos+n]
	c.pos += n
	return v, true
}

func (c *managerDBusCursor) number(n int) (uint64, bool) {
	if !c.align(n) {
		return 0, false
	}
	b, ok := c.take(n)
	if !ok {
		return 0, false
	}
	switch n {
	case 1:
		return uint64(b[0]), true
	case 4:
		return uint64(c.order.Uint32(b)), true
	case 8:
		return c.order.Uint64(b), true
	}
	return 0, false
}

func (c *managerDBusCursor) text(signature bool) (string, bool) {
	n := 4
	if signature {
		n = 1
	}
	length, ok := c.number(n)
	if !ok || length > managerDBusLimit || c.pos > len(c.data) || int(length) > len(c.data)-c.pos {
		return "", false
	}
	b, ok := c.take(int(length) + 1)
	if !ok || b[len(b)-1] != 0 || !utf8.Valid(b[:len(b)-1]) || strings.ContainsRune(string(b[:len(b)-1]), 0) {
		return "", false
	}
	return string(b[:len(b)-1]), true
}

func (c *managerDBusCursor) array(element func(*managerDBusCursor) bool, alignment int) bool {
	length, ok := c.number(4)
	if !ok || length > managerDBusLimit || !c.align(alignment) || c.pos > len(c.data) || int(length) > len(c.data)-c.pos {
		return false
	}
	end := c.pos + int(length)
	count := 0
	for c.pos < end {
		before := c.pos
		if count >= 64 || !element(c) || c.pos <= before || c.pos > end {
			return false
		}
		count++
	}
	return c.pos == end
}

func (c *managerDBusCursor) value(sig string) bool {
	switch sig {
	case "s":
		_, ok := c.text(false)
		return ok
	case "u":
		_, ok := c.number(4)
		return ok
	case "t":
		_, ok := c.number(8)
		return ok
	case "b":
		v, ok := c.number(4)
		return ok && v <= 1
	case "as":
		return c.array(func(c *managerDBusCursor) bool { return c.value("s") }, 4)
	case "a(ss)":
		return c.array(func(c *managerDBusCursor) bool { return c.align(8) && c.value("s") && c.value("s") }, 8)
	case "a(sasbttttuii)":
		return c.array(func(c *managerDBusCursor) bool {
			if !c.align(8) || !c.value("s") || !c.value("as") || !c.value("b") {
				return false
			}
			for i := 0; i < 4; i++ {
				if !c.value("t") {
					return false
				}
			}
			for i := 0; i < 3; i++ {
				if !c.value("u") {
					return false
				}
			}
			return true
		}, 8)
	}
	return false
}

func managerDBusFrameSize(header []byte) (int, binary.ByteOrder, bool) {
	if len(header) != 16 || header[3] != 1 || header[1] < 2 || header[1] > 3 || header[2]&^byte(1) != 0 {
		return 0, nil, false
	}
	var order binary.ByteOrder
	switch header[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return 0, nil, false
	}
	body := uint64(order.Uint32(header[4:8]))
	fields := uint64(order.Uint32(header[12:16]))
	total := uint64(16) + (fields+7)&^uint64(7) + body
	if order.Uint32(header[8:12]) == 0 || total > managerDBusLimit || total < 16 {
		return 0, nil, false
	}
	return int(total), order, true
}

// validateManagerDBusFrame rejects maps, rights, unexpected messages and every
// oversized nested length before godbus can allocate or overwrite duplicate keys.
func validateManagerDBusFrame(frame []byte) (uint32, error) {
	if len(frame) < 16 {
		return 0, ErrBoundary
	}
	size, order, ok := managerDBusFrameSize(frame[:16])
	if !ok || size != len(frame) {
		return 0, ErrBoundary
	}
	headerEnd := 16 + int(order.Uint32(frame[12:16]))
	c := managerDBusCursor{data: frame[:headerEnd], pos: 16, order: order}
	seen := map[byte]bool{}
	reply := uint32(0)
	bodySig := ""
	for c.pos < headerEnd {
		if !c.align(8) {
			return 0, ErrBoundary
		}
		raw, ok := c.take(1)
		if !ok {
			return 0, ErrBoundary
		}
		key := raw[0]
		if seen[key] {
			return 0, ErrBoundary
		}
		seen[key] = true
		sig, ok := c.text(true)
		if !ok {
			return 0, ErrBoundary
		}
		switch key {
		case 4, 6, 7:
			if sig != "s" || key == 4 && frame[1] != 3 {
				return 0, ErrBoundary
			}
			if _, ok = c.text(false); !ok {
				return 0, ErrBoundary
			}
		case 5:
			if sig != "u" {
				return 0, ErrBoundary
			}
			v, ok := c.number(4)
			if !ok || v == 0 {
				return 0, ErrBoundary
			}
			reply = uint32(v) // #nosec G115 -- number(4) reads exactly a uint32, widened to uint64.
		case 8:
			if sig != "g" {
				return 0, ErrBoundary
			}
			bodySig, ok = c.text(true)
			if !ok {
				return 0, ErrBoundary
			}
		default:
			return 0, ErrBoundary
		}
	}
	if c.pos != headerEnd || reply == 0 || !seen[8] || frame[1] == 3 && !seen[4] {
		return 0, ErrBoundary
	}
	c.data = frame
	if !c.align(8) {
		return 0, ErrBoundary
	}
	if frame[1] == 3 {
		if bodySig != "s" || !c.value("s") {
			return 0, ErrBoundary
		}
	} else {
		if bodySig != "v" {
			return 0, ErrBoundary
		}
		sig, ok := c.text(true)
		if !ok || !c.value(sig) {
			return 0, ErrBoundary
		}
	}
	if c.pos != len(frame) {
		return 0, ErrBoundary
	}
	return reply, nil
}

// managerDBusSignalFrameSize only recognizes bounded type4 framing. Callers
// must validate the complete closed signal headers before discarding the body.
// It does not admit signals to the existing strict reply decoder.
func managerDBusSignalFrameSize(header []byte) (int, bool) {
	if len(header) != 16 || header[1] != 4 {
		return 0, false
	}
	var replyHeader [16]byte
	copy(replyHeader[:], header)
	replyHeader[1] = 2
	size, _, ok := managerDBusFrameSize(replyHeader[:])
	return size, ok
}

// validateManagerDBusSignalFrame admits only public systemd broadcasts from
// the already authenticated PID1 peer. Bodies stay opaque: no generic decoder,
// property interpretation, pending-serial removal or authority is possible.
// Actual descriptor-bearing control messages are refused by rawRead as before.
func validateManagerDBusSignalFrame(frame []byte) error {
	if len(frame) < 16 {
		return ErrBoundary
	}
	size, ok := managerDBusSignalFrameSize(frame[:16])
	if !ok || size != len(frame) {
		return ErrBoundary
	}
	var byteOrder binary.ByteOrder = binary.LittleEndian
	if frame[0] == 'B' {
		byteOrder = binary.BigEndian
	}
	headerEnd := 16 + int(byteOrder.Uint32(frame[12:16]))
	c := managerDBusCursor{data: frame[:headerEnd], pos: 16, order: byteOrder}
	seen := map[byte]bool{}
	values := map[byte]string{}
	for c.pos < headerEnd {
		if !c.align(8) {
			return ErrBoundary
		}
		raw, ok := c.take(1)
		if !ok || seen[raw[0]] {
			return ErrBoundary
		}
		key := raw[0]
		seen[key] = true
		sig, ok := c.text(true)
		if !ok {
			return ErrBoundary
		}
		switch key {
		case 1:
			if sig != "o" {
				return ErrBoundary
			}
		case 2, 3, 6, 7:
			if sig != "s" {
				return ErrBoundary
			}
		case 8:
			if sig != "g" {
				return ErrBoundary
			}
		default:
			return ErrBoundary // Includes reply serial and UNIX_FDS metadata.
		}
		value, ok := c.text(key == 8)
		if !ok || key != 8 && (value == "" || key != 1 && len(value) > 255) {
			return ErrBoundary
		}
		values[key] = value
	}
	if c.pos != headerEnd || !seen[1] || !seen[2] || !seen[3] {
		return ErrBoundary
	}
	c.data = frame
	if !c.align(8) {
		return ErrBoundary
	}
	path, iface, member, sig := values[1], values[2], values[3], values[8]
	const managerPath = "/org/freedesktop/systemd1"
	if iface == "org.freedesktop.DBus.Properties" && member == "PropertiesChanged" {
		if sig != "sa{sv}as" || !managerDBusSignalPath(path) {
			return ErrBoundary
		}
		return nil
	}
	if path != managerPath || iface != "org.freedesktop.systemd1.Manager" {
		return ErrBoundary
	}
	expected := ""
	switch member {
	case "UnitNew", "UnitRemoved":
		expected = "so"
	case "JobNew":
		expected = "uos"
	case "JobRemoved":
		expected = "uoss"
	case "Reloading":
		expected = "b"
	case "StartupFinished":
		expected = "tttttt"
	case "UnitFilesChanged":
		if sig != "" || byteOrder.Uint32(frame[4:8]) != 0 {
			return ErrBoundary
		}
		return nil
	default:
		return ErrBoundary
	}
	if !seen[8] || sig != expected {
		return ErrBoundary
	}
	return nil
}

func managerDBusSignalPath(path string) bool {
	const managerPath = "/org/freedesktop/systemd1"
	if path == managerPath {
		return true
	}
	const jobPrefix = managerPath + "/job/"
	if strings.HasPrefix(path, jobPrefix) {
		id := path[len(jobPrefix):]
		if id == "" {
			return false
		}
		for _, b := range []byte(id) {
			if b < '0' || b > '9' {
				return false
			}
		}
		return true
	}
	const unitPrefix = managerPath + "/unit/"
	if !strings.HasPrefix(path, unitPrefix) || len(path) == len(unitPrefix) {
		return false
	}
	for _, b := range []byte(path[len(unitPrefix):]) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_') {
			return false
		}
	}
	return true
}
