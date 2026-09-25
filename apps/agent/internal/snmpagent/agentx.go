// Package snmpagent is the VRX-MIB AgentX subagent (F-snmp, WBS D7.5): a pure-Go implementation of the
// RFC 2741 subset a read-only subagent needs (Open, Register, Get, GetNext, GetBulk, Response, Ping,
// Close), serving VPP interface counters and agent health under the VRX-MIB subtree. No cgo, no
// net-snmp linking (the GPL/BSD boundary stays at the AgentX socket). SET PDUs are answered notWritable.
// The MIB text is deploy/snmp/VRX-MIB.txt.
package snmpagent

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// PDU types (RFC 2741 §6.1).
const (
	pduOpen          = 1
	pduClose         = 2
	pduRegister      = 3
	pduUnregister    = 4
	pduGet           = 5
	pduGetNext       = 6
	pduGetBulk       = 7
	pduTestSet       = 8
	pduCommitSet     = 9
	pduUndoSet       = 10
	pduCleanupSet    = 11
	pduNotify        = 12
	pduPing          = 13
	pduIndexAllocate = 14
	pduIndexDealloc  = 15
	pduAddAgentCaps  = 16
	pduRemoveAgentCa = 17
	pduResponse      = 18
)

// Header flags.
const (
	flagInstanceRegistration = 0x01
	flagNonDefaultContext    = 0x08
	flagNetworkByteOrder     = 0x10
)

// Varbind types (RFC 2741 §5.4).
const (
	TypeInteger        = 2
	TypeOctetString    = 4
	TypeNull           = 5
	TypeObjectID       = 6
	TypeIPAddress      = 64
	TypeCounter32      = 65
	TypeGauge32        = 66
	TypeTimeTicks      = 67
	TypeCounter64      = 70
	TypeNoSuchObject   = 128
	TypeNoSuchInstance = 129
	TypeEndOfMibView   = 130
)

// Response errors (RFC 2741 §6.2.16).
const (
	errNone           = 0
	errGenErr         = 5
	errNotWritable    = 17
	errParseFailed    = 266
	errProcessingErr  = 268
	errUnsupportedCtx = 262
)

const headerLen = 20

// maxPayload bounds one PDU (a master never sends more than a few KiB to a subagent).
const maxPayload = 1 << 20

// OID is a numeric object identifier.
type OID []uint32

// ParseOID parses ".1.3.6.1" or "1.3.6.1".
func ParseOID(s string) (OID, error) {
	s = strings.TrimPrefix(s, ".")
	if s == "" {
		return nil, errors.New("snmpagent: empty OID")
	}
	parts := strings.Split(s, ".")
	o := make(OID, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("snmpagent: OID %q: %w", s, err)
		}
		o[i] = uint32(v)
	}
	return o, nil
}

// MustOID is ParseOID for constants.
func MustOID(s string) OID {
	o, err := ParseOID(s)
	if err != nil {
		panic(err)
	}
	return o
}

func (o OID) String() string {
	var b strings.Builder
	for _, v := range o {
		b.WriteByte('.')
		b.WriteString(strconv.FormatUint(uint64(v), 10))
	}
	return b.String()
}

// Compare orders OIDs lexicographically (-1, 0, 1).
func (o OID) Compare(p OID) int {
	for i := 0; i < len(o) && i < len(p); i++ {
		switch {
		case o[i] < p[i]:
			return -1
		case o[i] > p[i]:
			return 1
		}
	}
	switch {
	case len(o) < len(p):
		return -1
	case len(o) > len(p):
		return 1
	}
	return 0
}

// HasPrefix reports whether p is a prefix of o.
func (o OID) HasPrefix(p OID) bool {
	return len(o) >= len(p) && o[:len(p)].Compare(p) == 0
}

// Append returns o followed by subs (a new slice).
func (o OID) Append(subs ...uint32) OID {
	out := make(OID, 0, len(o)+len(subs))
	return append(append(out, o...), subs...)
}

// Value is one typed varbind value.
type Value struct {
	Type uint16
	Int  int64  // Integer, Counter32, Gauge32, TimeTicks, Counter64 (as uint64 bits)
	U64  uint64 // Counter64
	Str  []byte // OctetString, IPAddress
	OID  OID    // ObjectID
}

// VarBind is a name and a value.
type VarBind struct {
	Name  OID
	Value Value
}

// header is the fixed PDU header.
type header struct {
	Version       uint8
	Type          uint8
	Flags         uint8
	SessionID     uint32
	TransactionID uint32
	PacketID      uint32
	PayloadLength uint32
}

func (h header) order() binary.ByteOrder {
	if h.Flags&flagNetworkByteOrder != 0 {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

// encoder builds a PDU payload in network byte order (the subagent always sets the flag).
type encoder struct{ b []byte }

func (e *encoder) u8(v uint8)   { e.b = append(e.b, v) }
func (e *encoder) u16(v uint16) { e.b = binary.BigEndian.AppendUint16(e.b, v) }
func (e *encoder) u32(v uint32) { e.b = binary.BigEndian.AppendUint32(e.b, v) }
func (e *encoder) u64(v uint64) { e.b = binary.BigEndian.AppendUint64(e.b, v) }

func (e *encoder) oid(o OID, include bool) {
	prefix := uint8(0)
	subs := o
	if len(o) >= 5 && o[0] == 1 && o[1] == 3 && o[2] == 6 && o[3] == 1 && o[4] > 0 && o[4] < 256 {
		prefix = uint8(o[4])
		subs = o[5:]
	}
	e.u8(uint8(len(subs)))
	e.u8(prefix)
	if include {
		e.u8(1)
	} else {
		e.u8(0)
	}
	e.u8(0)
	for _, s := range subs {
		e.u32(s)
	}
}

func (e *encoder) octets(s []byte) {
	e.u32(uint32(len(s)))
	e.b = append(e.b, s...)
	for len(e.b)%4 != 0 {
		e.b = append(e.b, 0)
	}
}

func (e *encoder) varbind(vb VarBind) {
	e.u16(vb.Value.Type)
	e.u16(0)
	e.oid(vb.Name, false)
	switch vb.Value.Type {
	case TypeInteger, TypeCounter32, TypeGauge32, TypeTimeTicks:
		e.u32(uint32(vb.Value.Int))
	case TypeCounter64:
		e.u64(vb.Value.U64)
	case TypeOctetString, TypeIPAddress:
		e.octets(vb.Value.Str)
	case TypeObjectID:
		e.oid(vb.Value.OID, false)
	}
}

// pdu encodes a whole PDU.
func pdu(typ uint8, flags uint8, session, txn, packet uint32, payload []byte) []byte {
	out := make([]byte, 0, headerLen+len(payload))
	out = append(out, 1, typ, flags|flagNetworkByteOrder, 0)
	out = binary.BigEndian.AppendUint32(out, session)
	out = binary.BigEndian.AppendUint32(out, txn)
	out = binary.BigEndian.AppendUint32(out, packet)
	out = binary.BigEndian.AppendUint32(out, uint32(len(payload)))
	return append(out, payload...)
}

// readPDU reads one PDU.
func readPDU(r io.Reader) (header, []byte, error) {
	var hb [headerLen]byte
	if _, err := io.ReadFull(r, hb[:]); err != nil {
		return header{}, nil, err
	}
	h := header{Version: hb[0], Type: hb[1], Flags: hb[2]}
	o := h.order()
	h.SessionID = o.Uint32(hb[4:])
	h.TransactionID = o.Uint32(hb[8:])
	h.PacketID = o.Uint32(hb[12:])
	h.PayloadLength = o.Uint32(hb[16:])
	if h.Version != 1 {
		return h, nil, fmt.Errorf("snmpagent: AgentX version %d", h.Version)
	}
	if h.PayloadLength > maxPayload || h.PayloadLength%4 != 0 {
		return h, nil, fmt.Errorf("snmpagent: payload length %d", h.PayloadLength)
	}
	p := make([]byte, h.PayloadLength)
	if _, err := io.ReadFull(r, p); err != nil {
		return h, nil, err
	}
	return h, p, nil
}

// decoder reads a payload.
type decoder struct {
	b   []byte
	o   binary.ByteOrder
	err error
}

var errShort = errors.New("snmpagent: short payload")

func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || len(d.b) < n {
		d.err = errShort
		return nil
	}
	v := d.b[:n]
	d.b = d.b[n:]
	return v
}

func (d *decoder) u8() uint8 {
	if v := d.take(1); v != nil {
		return v[0]
	}
	return 0
}

func (d *decoder) u16() uint16 {
	if v := d.take(2); v != nil {
		return d.o.Uint16(v)
	}
	return 0
}

func (d *decoder) u32() uint32 {
	if v := d.take(4); v != nil {
		return d.o.Uint32(v)
	}
	return 0
}

func (d *decoder) u64() uint64 {
	if v := d.take(8); v != nil {
		return d.o.Uint64(v)
	}
	return 0
}

func (d *decoder) oid() (OID, bool) {
	n := int(d.u8())
	prefix := d.u8()
	include := d.u8() != 0
	d.u8()
	var o OID
	if prefix != 0 {
		o = OID{1, 3, 6, 1, uint32(prefix)}
	}
	for range n {
		o = append(o, d.u32())
	}
	return o, include
}

func (d *decoder) octets() []byte {
	n := int(d.u32())
	if n > maxPayload {
		d.err = errShort
		return nil
	}
	v := d.take(n)
	if pad := (4 - n%4) % 4; pad > 0 {
		d.take(pad)
	}
	return append([]byte(nil), v...)
}

func (d *decoder) varbind() VarBind {
	var vb VarBind
	vb.Value.Type = d.u16()
	d.u16()
	vb.Name, _ = d.oid()
	switch vb.Value.Type {
	case TypeInteger:
		vb.Value.Int = int64(int32(d.u32()))
	case TypeCounter32, TypeGauge32, TypeTimeTicks:
		vb.Value.Int = int64(d.u32())
	case TypeCounter64:
		vb.Value.U64 = d.u64()
	case TypeOctetString, TypeIPAddress:
		vb.Value.Str = d.octets()
	case TypeObjectID:
		vb.Value.OID, _ = d.oid()
	}
	return vb
}

// searchRange is one SearchRange of a Get/GetNext/GetBulk.
type searchRange struct {
	Start   OID
	Include bool
	End     OID // empty = unbounded
}

func (d *decoder) ranges() []searchRange {
	var out []searchRange
	for d.err == nil && len(d.b) > 0 {
		s, inc := d.oid()
		e, _ := d.oid()
		out = append(out, searchRange{Start: s, Include: inc, End: e})
	}
	return out
}
