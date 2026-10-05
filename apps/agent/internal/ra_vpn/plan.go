// Package ravpn owns the independent remote-access namespace/runtime boundary.
// It does not import the retired kernel-vpp implementation.
package ravpn

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

const InstanceRoot = "/run/ngfw/ra"

var ErrPlan = errors.New("remote-access: invalid isolated network plan")
var instanceName = regexp.MustCompile(`^[a-f0-9]{64}$`)
var ownerName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func safeOwnerName(name string) bool { return ownerName.MatchString(name) }

// Link is an explicit point-to-point namespace/VPP transit pair.
type Link struct {
	VPP       string `json:"vpp"`
	Namespace string `json:"namespace"`
}

type RadiusEndpoint struct {
	Address string `json:"address"`
	Port    uint32 `json:"port"`
}
type KernelLink struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Index uint32 `json:"index"`
}

// NetworkPlan is public, bounded root-owned helper input; it has no credential field.
type NetworkPlan struct {
	Format             uint32           `json:"format"`
	Owner              string           `json:"owner"`
	Profile            string           `json:"profile"`
	Instance           string           `json:"instance"`
	NamespaceInode     uint64           `json:"namespaceInode"`
	HostNamespaceInode uint64           `json:"hostNamespaceInode"`
	KernelLinks        []KernelLink     `json:"kernelLinks,omitempty"`
	LocalAddress       string           `json:"localAddress"`
	Outer              Link             `json:"outer"`
	Inner              Link             `json:"inner"`
	InnerIPv6          *Link            `json:"innerIpv6,omitempty"`
	Pools              []string         `json:"pools"`
	Split              []string         `json:"split"`
	Radius             []RadiusEndpoint `json:"radius"`
}

// InstanceID is deterministic per agent owner/profile, never an operator-supplied path.
func InstanceID(owner, profile string) string {
	sum := sha256.Sum256([]byte(owner + "\x00" + profile))
	return hex.EncodeToString(sum[:])
}
func ValidInstance(instance string) bool { return instanceName.MatchString(instance) }

// BuildNetworkPlan validates the exact addresses before namespace creation.
func BuildNetworkPlan(owner, name string, profile *ngfwv1.RemoteAccessProfile) (*NetworkPlan, error) {
	if owner == "" || name == "" || profile == nil || profile.GetTransport() == nil {
		return nil, ErrPlan
	}
	source := profile.GetTransport()
	link := func(value *ngfwv1.RemoteAccessTransitLink) Link {
		return Link{VPP: value.GetVpp(), Namespace: value.GetNamespace()}
	}
	plan := &NetworkPlan{Format: 1, Owner: owner, Profile: name, Instance: InstanceID(owner, name), LocalAddress: profile.GetLocalAddr(), Outer: link(source.GetOuter()), Inner: link(source.GetInner())}
	if source.GetInnerIpv6() != nil {
		v6 := link(source.GetInnerIpv6())
		plan.InnerIPv6 = &v6
	}
	for _, pool := range profile.GetPools() {
		plan.Pools = append(plan.Pools, pool.GetPrefix())
	}
	plan.Split = slices.Clone(profile.GetSplitTunnel())
	for _, server := range profile.GetRadius().GetServers() {
		port := server.GetPort()
		if port == 0 {
			port = 1812
		}
		plan.Radius = append(plan.Radius, RadiusEndpoint{Address: server.GetAddress(), Port: port})
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return plan, nil
}

// Validate is repeated by the helper; a valid instance name never bypasses bounds.
func (plan *NetworkPlan) Validate() error {
	if plan == nil || plan.Format != 1 || !ValidInstance(plan.Instance) || len(plan.Pools) == 0 || len(plan.Pools) > 16 || len(plan.Split) > 64 || len(plan.Radius) > 8 {
		return ErrPlan
	}
	if !safeOwnerName(plan.Owner) || !safeOwnerName(plan.Profile) || InstanceID(plan.Owner, plan.Profile) != plan.Instance {
		return ErrPlan
	}
	if len(plan.KernelLinks) > 16 {
		return ErrPlan
	}
	seenKernel := map[string]bool{}
	for _, link := range plan.KernelLinks {
		if plan.NamespaceInode == 0 || plan.HostNamespaceInode == 0 || plan.NamespaceInode == plan.HostNamespaceInode || kernelFallbackKinds[link.Name] != link.Kind || link.Kind == "" || link.Index == 0 || seenKernel[link.Name] {
			return ErrPlan
		}
		seenKernel[link.Name] = true
	}
	endpoint, err := netip.ParseAddr(plan.LocalAddress)
	if err != nil || endpoint.IsUnspecified() || endpoint.IsMulticast() || endpoint.IsLoopback() || endpoint.IsLinkLocalUnicast() {
		return ErrPlan
	}
	type pair struct{ vpp, namespace netip.Prefix }
	pairs := []pair{}
	check := func(link Link, family4 bool) error {
		vpp, err := netip.ParsePrefix(link.VPP)
		if err != nil {
			return ErrPlan
		}
		namespace, err := netip.ParsePrefix(link.Namespace)
		if err != nil {
			return ErrPlan
		}
		bits := 127
		if family4 {
			bits = 31
		}
		if vpp.Addr().Is4() != family4 || namespace.Addr().Is4() != family4 || vpp.Bits() != bits || namespace.Bits() != bits || vpp.Masked() != namespace.Masked() || vpp.Addr() == namespace.Addr() || vpp.Contains(endpoint) || vpp.Addr().IsMulticast() || namespace.Addr().IsMulticast() {
			return ErrPlan
		}
		for _, other := range pairs {
			if vpp.Overlaps(other.vpp) {
				return ErrPlan
			}
		}
		pairs = append(pairs, pair{vpp, namespace})
		return nil
	}
	if err := check(plan.Outer, endpoint.Is4()); err != nil {
		return err
	}
	if err := check(plan.Inner, true); err != nil {
		return err
	}
	if plan.InnerIPv6 != nil {
		if err := check(*plan.InnerIPv6, false); err != nil {
			return err
		}
	}
	pools := []netip.Prefix{}
	for _, text := range plan.Pools {
		prefix, err := netip.ParsePrefix(text)
		if err != nil || prefix.Masked().String() != text || prefix.Contains(endpoint) || prefix.Addr().IsMulticast() || (!prefix.Addr().Is4() && plan.InnerIPv6 == nil) {
			return ErrPlan
		}
		for _, link := range pairs {
			if prefix.Overlaps(link.vpp) {
				return ErrPlan
			}
		}
		for _, other := range pools {
			if prefix.Overlaps(other) {
				return ErrPlan
			}
		}
		pools = append(pools, prefix)
	}
	for _, text := range plan.Split {
		prefix, err := netip.ParsePrefix(text)
		if err != nil || prefix.Masked().String() != text || (!prefix.Addr().Is4() && plan.InnerIPv6 == nil) {
			return ErrPlan
		}
	}
	for _, server := range plan.Radius {
		if server.Port == 0 || server.Port > 65535 {
			return ErrPlan
		}
		address, err := netip.ParseAddr(server.Address)
		if err != nil || address.Is4() != endpoint.Is4() || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() {
			return ErrPlan
		}
		for _, pool := range pools {
			if pool.Contains(address) {
				return ErrPlan
			}
		}
	}
	return nil
}

// Commands returns fixed iproute2 arguments, executed only AFTER the helper
// proves it is in the expected owned namespace and has no CAP_SYS_ADMIN.
// addXFRM/addRules are determined from observed existing namespace state;
// existing foreign link/rule collisions are refused by the caller.
func (plan *NetworkPlan) Commands(addXFRM, addRules bool) ([][]string, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	endpoint, _ := netip.ParseAddr(plan.LocalAddress)
	family := "-6"
	bits := 128
	if endpoint.Is4() {
		family = "-4"
		bits = 32
	}
	outer, _ := netip.ParsePrefix(plan.Outer.VPP)
	inner, _ := netip.ParsePrefix(plan.Inner.VPP)
	commands := [][]string{{"link", "set", "dev", "lo", "up"}, {family, "address", "replace", endpoint.String() + "/" + strconv.Itoa(bits), "dev", "lo"}, {"link", "set", "dev", "outer0", "up"}, {"link", "set", "dev", "inner0", "up"}}
	if addXFRM {
		commands = append(commands, []string{"link", "add", "xfrm0", "type", "xfrm", "dev", "outer0", "if_id", "1"})
	}
	commands = append(commands, []string{"link", "set", "dev", "xfrm0", "up"})
	if addRules {
		commands = append(commands, []string{family, "rule", "add", "priority", "100", "fwmark", "1", "lookup", "100"})
	}
	commands = append(commands, []string{family, "route", "replace", "table", "100", "default", "via", outer.Addr().String(), "dev", "outer0"})
	split := slices.Clone(plan.Split)
	if len(split) == 0 {
		split = append(split, "0.0.0.0/0")
		if plan.InnerIPv6 != nil {
			split = append(split, "::/0")
		}
	}
	for _, text := range split {
		prefix, _ := netip.ParsePrefix(text)
		f, gateway := "-4", inner.Addr().String()
		if !prefix.Addr().Is4() {
			f = "-6"
			v6, _ := netip.ParsePrefix(plan.InnerIPv6.VPP)
			gateway = v6.Addr().String()
		}
		commands = append(commands, []string{f, "route", "replace", text, "via", gateway, "dev", "inner0"})
	}
	for _, text := range plan.Pools {
		f := "-4"
		if strings.Contains(text, ":") {
			f = "-6"
		}
		commands = append(commands, []string{f, "route", "replace", text, "dev", "xfrm0"})
	}
	for _, server := range plan.Radius {
		commands = append(commands, []string{family, "route", "replace", server.Address + "/" + strconv.Itoa(bits), "via", outer.Addr().String(), "dev", "outer0", "src", endpoint.String()})
	}
	return commands, nil
}
