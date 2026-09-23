package dhcp

import (
	"encoding/hex"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/descriptors/dfkit"
)

// Proxy is one DHCP relay server of an rx VRF (dhcp_proxy_config). VPP keeps one source address
// per rx VRF and family and a list of servers; each server is one object. Addresses are
// canonical (netip) and of one family, which selects DHCPv4 or DHCPv6 relay.
type Proxy struct {
	RxVRF     uint32 `json:"rx_vrf"`
	Server    string `json:"server"`
	ServerVRF uint32 `json:"server_vrf"`
	Src       string `json:"src"`
}

// VSS types (dhcp_proxy_set_vss.vss_type).
const (
	VSSASCII   = "ascii"   // option 82 VSS as a VPN ASCII id
	VSSVPNID   = "vpn-id"  // VSS as OUI + VPN index
	VSSDefault = "default" // "default VPN" (type 255)
)

// ProxyVSS is the VSS (RFC 6607 option 82/v6 option 68) of an rx VRF (dhcp_proxy_set_vss).
// Exactly the fields of Type are set: VPNASCIIID for ascii, OUI + VPNIndex for vpn-id.
type ProxyVSS struct {
	Family     string `json:"family"` // "ip4" | "ip6"
	VRF        uint32 `json:"vrf"`
	Type       string `json:"type"`
	VPNASCIIID string `json:"vpn_ascii_id"`
	OUI        uint32 `json:"oui"`
	VPNIndex   uint32 `json:"vpn_index"`
}

// Client is the DHCPv4 client on one interface (dhcp_client_config). WantEvents subscribes this
// API client to dhcp_compl_event (lease state, see WatchLeases).
type Client struct {
	Interface        string `json:"interface"`
	Hostname         string `json:"hostname"`
	ClientID         string `json:"client_id"`
	SetBroadcastFlag bool   `json:"set_broadcast_flag"`
	DSCP             uint8  `json:"dscp"`
	WantEvents       bool   `json:"want_events"`
}

// DHCP6Client is the DHCPv6 IA_NA client on one interface (dhcp6_client_enable_disable).
type DHCP6Client struct {
	Interface string `json:"interface"`
}

// DHCP6PDClient is the DHCPv6 prefix-delegation client on one interface; delegated prefixes go
// into PrefixGroup (dhcp6_pd_client_enable_disable).
type DHCP6PDClient struct {
	Interface   string `json:"interface"`
	PrefixGroup string `json:"prefix_group"`
}

// DHCP6PDAddress is an interface address built from a delegated prefix of PrefixGroup plus the
// host part Address ("::1:0:0:0:1/64" — the prefix bits come from the delegation)
// (ip6_add_del_address_using_prefix).
type DHCP6PDAddress struct {
	Interface   string `json:"interface"`
	PrefixGroup string `json:"prefix_group"`
	Address     string `json:"address"`
}

// DHCP6DUID is the client DUID-LL used by all DHCPv6 clients (dhcp6_duid_ll_set): 10 bytes,
// hex with ":" separators, starting with type 00:03 (DUID-LL).
type DHCP6DUID struct {
	DUIDLL string `json:"duid_ll"`
}

// DUIDID is the object id of the DUID singleton (key dhcp.dhcp6-duid/global).
const DUIDID = "global"

// Proto returns the canonical structpb document.
func (s Proxy) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s ProxyVSS) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s Client) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s DHCP6Client) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s DHCP6PDClient) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s DHCP6PDAddress) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s DHCP6DUID) Proto() *structpb.Struct { return dfkit.Encode(s) }

func decode[T any](msg proto.Message) (T, error) {
	var s T
	err := dfkit.Decode(msg, &s)
	return s, err
}

// Validate checks what VPP would reject: server and src of one family, both non-zero.
func (s Proxy) Validate() error {
	srv, err := dfkit.ParseAddr(s.Server)
	if err != nil {
		return err
	}
	src, err := dfkit.ParseAddr(s.Src)
	if err != nil {
		return err
	}
	if srv.String() != s.Server || src.String() != s.Src {
		return dfkit.Specf("dhcp proxy: addresses must be canonical (%s, %s)", srv, src)
	}
	if srv.Is4() != src.Is4() {
		return dfkit.Specf("dhcp proxy: server %s and src %s are not the same family", srv, src)
	}
	if srv.IsUnspecified() || src.IsUnspecified() {
		return dfkit.Specf("dhcp proxy: server and src must not be unspecified")
	}
	return nil
}

// Validate checks the family, the type and that only the type's fields are set.
func (s ProxyVSS) Validate() error {
	if s.Family != "ip4" && s.Family != "ip6" {
		return dfkit.Specf("dhcp proxy-vss: family %q must be ip4 or ip6", s.Family)
	}
	switch s.Type {
	case VSSASCII:
		if s.VPNASCIIID == "" || len(s.VPNASCIIID) > 128 || s.OUI != 0 || s.VPNIndex != 0 {
			return dfkit.Specf("dhcp proxy-vss: ascii needs vpn_ascii_id (1..128 bytes) and no oui/vpn_index")
		}
		if strings.ContainsRune(s.VPNASCIIID, 0) {
			return dfkit.Specf("dhcp proxy-vss: vpn_ascii_id contains NUL")
		}
	case VSSVPNID:
		if s.VPNASCIIID != "" || s.OUI > 0xffffff {
			return dfkit.Specf("dhcp proxy-vss: vpn-id needs oui (24 bit) + vpn_index and no vpn_ascii_id")
		}
	case VSSDefault:
		if s.VPNASCIIID != "" || s.OUI != 0 || s.VPNIndex != 0 {
			return dfkit.Specf("dhcp proxy-vss: default takes no fields")
		}
	default:
		return dfkit.Specf("dhcp proxy-vss: type %q must be %s, %s or %s", s.Type, VSSASCII, VSSVPNID, VSSDefault)
	}
	return nil
}

func validateText(what, v string, maxLen int, allowEmpty bool) error {
	if v == "" && !allowEmpty {
		return dfkit.Specf("%s is empty", what)
	}
	if len(v) > maxLen {
		return dfkit.Specf("%s %q longer than %d bytes", what, v, maxLen)
	}
	for _, r := range v {
		if r < 0x20 || r > 0x7e {
			return dfkit.Specf("%s %q contains a non-printable character", what, v)
		}
	}
	return nil
}

// Validate checks the interface, hostname (1..63 printable bytes — VPP copies it with strlen, an
// empty one is not supported), client id (0..63) and DSCP (6 bit).
func (s Client) Validate() error {
	if err := validateText("interface", s.Interface, 63, false); err != nil {
		return err
	}
	if err := validateText("hostname", s.Hostname, 63, false); err != nil {
		return err
	}
	if err := validateText("client_id", s.ClientID, 63, true); err != nil {
		return err
	}
	if s.DSCP > 63 {
		return dfkit.Specf("dscp %d > 63", s.DSCP)
	}
	return nil
}

// Validate checks the interface name.
func (s DHCP6Client) Validate() error { return validateText("interface", s.Interface, 63, false) }

// Validate checks the interface and prefix group (1..63 bytes, VPP rejects 64).
func (s DHCP6PDClient) Validate() error {
	if err := validateText("interface", s.Interface, 63, false); err != nil {
		return err
	}
	return validateText("prefix_group", s.PrefixGroup, 63, false)
}

// Validate checks the interface, prefix group and the IPv6 address/length.
func (s DHCP6PDAddress) Validate() error {
	if err := validateText("interface", s.Interface, 63, false); err != nil {
		return err
	}
	if err := validateText("prefix_group", s.PrefixGroup, 63, false); err != nil {
		return err
	}
	a, l, err := parseAddrLen(s.Address)
	if err != nil {
		return err
	}
	if !a.Is6() || a.Is4In6() {
		return dfkit.Specf("dhcp6-pd-address %q is not IPv6", s.Address)
	}
	if a.String()+"/"+strconv.Itoa(l) != s.Address {
		return dfkit.Specf("dhcp6-pd-address %q is not canonical", s.Address)
	}
	return nil
}

// Validate checks the DUID-LL: 10 bytes, type 3.
func (s DHCP6DUID) Validate() error {
	b, err := parseDUID(s.DUIDLL)
	if err != nil {
		return err
	}
	if formatDUID(b) != s.DUIDLL {
		return dfkit.Specf("duid_ll %q is not canonical (want %s)", s.DUIDLL, formatDUID(b))
	}
	return nil
}

func parseDUID(s string) ([]byte, error) {
	b, err := hex.DecodeString(strings.ReplaceAll(s, ":", ""))
	if err != nil || len(b) != 10 {
		return nil, dfkit.Specf("duid_ll %q: want 10 hex bytes", s)
	}
	if b[0] != 0 || b[1] != 3 {
		return nil, dfkit.Specf("duid_ll %q: DUID type must be 00:03 (DUID-LL)", s)
	}
	return b, nil
}

func formatDUID(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = hex.EncodeToString([]byte{x})
	}
	return strings.Join(parts, ":")
}

func uitoa(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

// pidSelf is this process's PID as VPP's u32 pid field (Linux PIDs fit).
func pidSelf() uint32 { return uint32(os.Getpid()) } //nolint:gosec // pid_max ≤ 2^22
