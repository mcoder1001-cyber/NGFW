package pppoe

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/netip"
	"reflect"
	"strings"
)

// CarrierSpec is the immutable namespace/transit contract. Logical identifies
// the VPP IP-facing TAP; Parent identifies a different raw Ethernet interface.
// Kernel and VPP addresses are per-session, never a shared same-VRF subnet.
type CarrierSpec struct {
	Owner   string `json:"owner"`
	Logical string `json:"logical"`
	Parent  string `json:"parent"`
	MTU     uint32 `json:"mtu"`
	Host4   string `json:"host4"`
	Peer4   string `json:"peer4"`
	Host6   string `json:"host6"`
	Peer6   string `json:"peer6"`
}

// NewCarrierSpec derives stable internal links. IPv4's finite link-local pool
// can collide; CheckCarrierPrefixes and live VPP admission MUST reject a
// collision before mutation, rather than treating hashes as allocation proof.
func NewCarrierSpec(owner, logical, parent string, mtu uint32) (CarrierSpec, error) {
	valid := func(s string) bool {
		return s != "" && len(s) <= 63 && !strings.ContainsAny(s, "\x00\r\n\t")
	}
	if !valid(owner) || !valid(logical) || !valid(parent) || logical == parent {
		return CarrierSpec{}, fmt.Errorf("%w: kernel PPP requires a distinct logical interface and explicit parent", ErrInput)
	}
	if mtu < 128 || mtu > 1492 {
		return CarrierSpec{}, fmt.Errorf("%w: kernel PPP MTU must be 128..1492", ErrInput)
	}
	digest := sha256.Sum256([]byte(owner + "\x00" + logical))
	block := uint32(binary.BigEndian.Uint16(digest[6:8])) % 16256
	base := uint32(169)<<24 | uint32(254)<<16 | 256
	base += block * 4
	v4 := func(offset uint32) string {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], base+offset)
		return netip.PrefixFrom(netip.AddrFrom4(raw), 30).String()
	}
	var raw6 [16]byte
	raw6[0] = 0xfd
	copy(raw6[1:8], digest[:7])
	v6 := func(offset byte) string {
		raw6[15] = offset
		return netip.PrefixFrom(netip.AddrFrom16(raw6), 126).String()
	}
	peer4 := netip.MustParsePrefix(v4(1)).Addr().String()
	peer6 := netip.MustParsePrefix(v6(1)).Addr().String()
	return CarrierSpec{Owner: owner, Logical: logical, Parent: parent, MTU: mtu,
		Host4: v4(2), Peer4: peer4, Host6: v6(2), Peer6: peer6}, nil
}

// VPP4/VPP6 are the router-side transit addresses, distinct from the ISP address.
func (s CarrierSpec) VPP4() string        { return s.Peer4 + "/30" }
func (s CarrierSpec) VPP6() string        { return s.Peer6 + "/126" }
func (s CarrierSpec) RawHost() string     { return "pw" + strings.TrimPrefix(s.Token(), "ngp-") }
func (s CarrierSpec) TransitHost() string { return "pt" + strings.TrimPrefix(s.Token(), "ngp-") }
func (s CarrierSpec) RawLogical() string  { return "pppr-" + strings.TrimPrefix(s.Token(), "ngp-") }

// TapIDs are stable candidates; live admission refuses any existing ID collision.
func (s CarrierSpec) TapIDs() (raw, transit uint32) {
	digest := sha256.Sum256([]byte(s.Owner + "\x00" + s.Logical))
	raw = binary.BigEndian.Uint32(digest[8:12])&0x3ffffffe | 0x40000000
	return raw, raw + 1
}

// Token is the fixed namespace/instance name; no caller path is accepted.
func (s CarrierSpec) Token() string {
	digest := sha256.Sum256([]byte(s.Owner + "\x00" + s.Logical))
	return "ngp-" + hex.EncodeToString(digest[:6])
}

// Validate refuses an altered/foreign derivation at descriptor and receipt boundaries.
func (s CarrierSpec) Validate() error {
	expected, err := NewCarrierSpec(s.Owner, s.Logical, s.Parent, s.MTU)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(s, expected) {
		return fmt.Errorf("%w: carrier identity/address derivation does not match", ErrInput)
	}
	return nil
}

// CheckCarrierPrefixes validates all requested internal networks against each
// other and externally configured interface addresses. The live admission layer
// must additionally compare VPP's observed addresses, excluding only the exact
// current owned generation. No partial reservation is returned on error.
func CheckCarrierPrefixes(specs []CarrierSpec, reserved []netip.Prefix) error {
	seenNames, seenParents := map[string]bool{}, map[string]bool{}
	var internal []netip.Prefix
	for _, spec := range specs {
		if err := spec.Validate(); err != nil {
			return err
		}
		if seenNames[spec.Logical] || seenParents[spec.Parent] {
			return fmt.Errorf("%w: logical interface or raw parent is used by multiple PPP carriers", ErrInput)
		}
		seenNames[spec.Logical], seenParents[spec.Parent] = true, true
		for _, raw := range []string{spec.Host4, spec.Host6} {
			prefix, err := netip.ParsePrefix(raw)
			if err != nil {
				return err
			}
			prefix = prefix.Masked()
			for _, existing := range append(append([]netip.Prefix(nil), internal...), reserved...) {
				if !existing.IsValid() {
					return fmt.Errorf("%w: invalid reserved carrier prefix", ErrInput)
				}
				if existing.Overlaps(prefix) {
					return fmt.Errorf("%w: internal PPP transit prefix overlaps another interface", ErrInput)
				}
			}
			internal = append(internal, prefix)
		}
	}
	for _, spec := range specs {
		if seenNames[spec.Parent] {
			return fmt.Errorf("%w: a PPP logical interface cannot be another carrier's raw parent", ErrInput)
		}
	}
	return nil
}

// CarrierVLAN binds forwarding readiness to the committed tag classification.
// It is a session input; namespace geometry remains independently immutable.
type CarrierVLAN struct {
	Root                string
	SubID, Outer, Inner uint32
	Dot1AD              bool
}
