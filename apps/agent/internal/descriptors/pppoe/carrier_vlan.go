package pppoe

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"ngfw/agent/binapi/interface_types"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/l2"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// CarrierParent resolves exactly one configured raw interface or VLAN child.
// Config contains only the selected leaf's common interface configuration. Root
// can have unrelated VLAN children; they never become part of PPP transport.
type CarrierParent struct {
	Name, RootName, SubID string
	Root                  *ngfwv1.Interface
	Sub                   *ngfwv1.Subinterface
	Config                *ngfwv1.Interface
	Rewrite               *l2.VlanTagRewrite
}

func ResolveCarrierParent(ifs map[string]*ngfwv1.Interface, name string) (CarrierParent, error) {
	out := CarrierParent{Name: name, RootName: name, Root: ifs[name], Config: ifs[name]}
	if out.Root == nil {
		cut := strings.LastIndexByte(name, '.')
		root, id, ok := "", "", cut > 0
		if ok {
			root, id = name[:cut], name[cut+1:]
		}
		numeric, err := strconv.ParseUint(id, 10, 32)
		if !ok || err != nil || strconv.FormatUint(numeric, 10) != id || ifs[root] == nil || ifs[root].GetSubinterfaces()[id] == nil {
			return out, errors.New("PPP raw parent must be an existing interface or explicit VLAN child")
		}
		out.RootName, out.SubID, out.Root = root, id, ifs[root]
		out.Sub = out.Root.GetSubinterfaces()[id]
		sub := out.Sub
		if sub.GetVlanId() < 1 || sub.GetVlanId() > 4094 || (sub.InnerVlanId != nil && (sub.GetInnerVlanId() < 1 || sub.GetInnerVlanId() > 4094)) {
			return out, errors.New("PPP VLAN tags must be explicit IDs 1..4094")
		}
		out.Config = &ngfwv1.Interface{Enabled: sub.Enabled, Mtu: sub.Mtu, Ipv4: sub.Ipv4, Ipv6: sub.Ipv6, Vrf: sub.Vrf, Unnumbered: sub.Unnumbered, DhcpClient: sub.DhcpClient, L2: sub.L2, Ipv6Ra: sub.Ipv6Ra, ProxyArp: sub.ProxyArp, ProxyNd: sub.ProxyNd}
		op := l2.VtrOp_VTR_OP_POP_1
		if sub.InnerVlanId != nil {
			op = l2.VtrOp_VTR_OP_POP_2
		}
		out.Rewrite = &l2.VlanTagRewrite{Interface: string(iface.AliasKey(name)), Op: op, Xconnect: true}
	}
	if !out.Root.GetEnabled() || !out.Config.GetEnabled() {
		return out, errors.New("PPP raw parent and its physical root must be enabled")
	}
	p := out.Config
	if p.GetPppoe() != nil || p.GetBond() != nil || p.GetLcp() != nil || p.GetL2() != nil || p.Unnumbered != nil || len(p.GetIpv4()) != 0 || len(p.GetIpv6()) != 0 || p.GetDhcpClient() != nil || p.GetIpv6Ra() != nil || p.GetProxyArp() || len(p.GetProxyNd()) != 0 {
		return out, errors.New("PPP raw leaf must be exclusively unaddressed Ethernet without LCP, L2, IP, DHCP, RA or proxy configuration")
	}
	if out.Sub == nil && len(p.GetSubinterfaces()) != 0 {
		return out, errors.New("whole-port PPP parent cannot have VLAN children; select an explicit child")
	}
	if out.Root.GetPppoe() != nil || out.Root.GetBond() != nil || out.Root.GetLcp() != nil || out.Root.GetL2() != nil || out.Root.Unnumbered != nil {
		return out, errors.New("PPP physical root cannot be a PPP, bond, LCP, L2 or unnumbered attachment")
	}
	for _, other := range ifs {
		if _, member := other.GetBond().GetMembers()[out.RootName]; member {
			return out, errors.New("PPP physical root is a bond member")
		}
	}
	return out, nil
}

// CopyCarrierParentReference preserves explicit VLAN classification in the
// reference-only daemon manifest. Unrelated siblings do not trigger a PPP restart.
func CopyCarrierParentReference(target *ngfwv1.DesiredState, ifs map[string]*ngfwv1.Interface, name string) error {
	parent, err := ResolveCarrierParent(ifs, name)
	if err != nil {
		return err
	}
	root := proto.Clone(parent.Root).(*ngfwv1.Interface)
	if parent.Sub != nil {
		root.Subinterfaces = map[string]*ngfwv1.Subinterface{parent.SubID: proto.Clone(parent.Sub).(*ngfwv1.Subinterface)}
	}
	if target.Interfaces == nil {
		target.Interfaces = map[string]*ngfwv1.Interface{}
	}
	if existing := target.Interfaces[parent.RootName]; existing != nil && parent.Sub != nil {
		for id, sub := range existing.Subinterfaces {
			if id != parent.SubID {
				root.Subinterfaces[id] = sub
			}
		}
	}
	target.Interfaces[parent.RootName] = root
	return nil
}

// CarrierVLANDependency uses preserved configuration, never a dot-name guess.
func CarrierVLANDependency(doc *ngfwv1.DesiredState, parent string) (scheduler.Key, bool, error) {
	resolved, err := ResolveCarrierParent(doc.GetInterfaces(), parent)
	if err != nil {
		return "", false, err
	}
	if resolved.Rewrite == nil {
		return "", false, nil
	}
	return scheduler.Join(l2.VlanTagRewriteName, resolved.Name), true, nil
}

// carrierLiveVLAN proves selected subinterface ownership and exact classification.
// The output push is derived by VPP from these exact tags when POP_1/POP_2 is set.
func carrierLiveVLAN(table *iface.Table, parent uint32) (*l2.VlanTagRewrite, error) {
	row, ok := table.Details(parent)
	if !ok {
		return nil, errors.New("PPP raw parent disappeared")
	}
	if uint32(row.SupSwIfIndex) == parent && row.SubNumberOfTags == 0 {
		return nil, nil
	}
	name, owned := table.OwnedID(parent)
	if !owned {
		return nil, errors.New("PPP VLAN child must be owned by this agent")
	}
	flags := row.SubIfFlags
	required := interface_types.SUB_IF_API_FLAG_EXACT_MATCH
	switch row.SubNumberOfTags {
	case 1:
		required |= interface_types.SUB_IF_API_FLAG_ONE_TAG
	case 2:
		required |= interface_types.SUB_IF_API_FLAG_TWO_TAGS
	default:
		return nil, errors.New("PPP VLAN child must match one or two explicit tags")
	}
	if flags&^interface_types.SUB_IF_API_FLAG_DOT1AD != required || row.SubOuterVlanID < 1 || row.SubOuterVlanID > 4094 ||
		(row.SubNumberOfTags == 1 && row.SubInnerVlanID != 0) || (row.SubNumberOfTags == 2 && (row.SubInnerVlanID < 1 || row.SubInnerVlanID > 4094)) {
		return nil, errors.New("PPP VLAN child has wildcard, default or inconsistent tag classification")
	}
	root, ok := table.Details(uint32(row.SupSwIfIndex))
	if !ok || root.SubNumberOfTags != 0 || root.SupSwIfIndex != uint32(root.SwIfIndex) {
		return nil, errors.New("PPP VLAN root is not a physical parent")
	}
	expectedName := strings.TrimSpace(strings.TrimRight(root.InterfaceName, "\x00")) + "." + strconv.FormatUint(uint64(row.SubID), 10)
	// OwnedID is logical naming; a managed root can have a different live VPP name.
	if rootID, isOwned := table.OwnedID(uint32(root.SwIfIndex)); isOwned {
		expectedName = rootID + "." + strconv.FormatUint(uint64(row.SubID), 10)
	}
	if name != expectedName {
		return nil, errors.New("PPP VLAN ownership name does not match its physical parent and sub-ID")
	}
	op := l2.VtrOp_VTR_OP_POP_1
	if row.SubNumberOfTags == 2 {
		op = l2.VtrOp_VTR_OP_POP_2
	}
	return &l2.VlanTagRewrite{Interface: string(iface.AliasKey(name)), Op: op, Xconnect: true}, nil
}

// AdmitCarrierVLAN rejects an existing rewrite before acquiring the raw leaf.
// Existing L2/bridge/LCP/bond/address admission remains mandatory in AdmitCarrier.
func AdmitCarrierVLAN(ctx context.Context, c vpp.Client, owner string, spec ren.CarrierSpec) error {
	table, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return err
	}
	parent, err := table.IndexByName(spec.Parent)
	if err != nil {
		return err
	}
	if _, err = carrierLiveVLAN(table, parent); err != nil {
		return err
	}
	row, _ := table.Details(parent)
	if row.VtrOp != 0 {
		return errors.New("PPP raw parent already has a VLAN rewrite")
	}
	return nil
}

// VerifyCarrierVLANReadiness checks live VTR and classification, plus an untagged
// raw TAP. Configure POP on the VLAN child only: VPP creates the symmetric egress
// push; adding PUSH to the TAP would double-tag traffic.
func VerifyCarrierVLANReadiness(ctx context.Context, c vpp.Client, owner string, spec ren.CarrierSpec) error {
	table, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return err
	}
	parent, err := table.IndexByName(spec.Parent)
	if err != nil {
		return err
	}
	rewrite, err := carrierLiveVLAN(table, parent)
	if err != nil {
		return err
	}
	row, _ := table.Details(parent)
	expected := uint32(0)
	if rewrite != nil {
		expected = uint32(rewrite.Op)
	}
	if row.VtrOp != expected || row.VtrPushDot1q != 0 || row.VtrTag1 != 0 || row.VtrTag2 != 0 {
		return fmt.Errorf("PPP raw VLAN rewrite differs from required pop operation")
	}
	raw, err := table.IndexByName(spec.RawLogical())
	if err != nil {
		return err
	}
	if id, owned := table.OwnedID(raw); !owned || id != spec.RawLogical() {
		return errors.New("PPP raw TAP ownership is unavailable")
	}
	tap, _ := table.Details(raw)
	if tap.VtrOp != 0 || tap.VtrPushDot1q != 0 || tap.VtrTag1 != 0 || tap.VtrTag2 != 0 || tap.SubNumberOfTags != 0 {
		return errors.New("PPP raw TAP must transport untagged frames")
	}
	return nil
}

// VerifyCarrierVLANConfiguration compares valid live classification with the
// exact committed tag IDs; merely being some valid VLAN is insufficient.
func VerifyCarrierVLANConfiguration(ctx context.Context, c vpp.Client, owner string, spec ren.CarrierSpec, expected *ren.CarrierVLAN) error {
	table, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return err
	}
	index, err := table.IndexByName(spec.Parent)
	if err != nil {
		return err
	}
	row, _ := table.Details(index)
	if expected == nil {
		if row.SubNumberOfTags != 0 || uint32(row.SupSwIfIndex) != index {
			return errors.New("raw PPP parent unexpectedly became a VLAN")
		}
		return nil
	}
	root, err := table.IndexByName(expected.Root)
	if err != nil {
		return err
	}
	dot1ad := row.SubIfFlags&interface_types.SUB_IF_API_FLAG_DOT1AD != 0
	tags := uint8(1)
	if expected.Inner != 0 {
		tags = 2
	}
	if uint32(row.SupSwIfIndex) != root || row.SubID != expected.SubID || uint32(row.SubOuterVlanID) != expected.Outer || uint32(row.SubInnerVlanID) != expected.Inner || row.SubNumberOfTags != tags || dot1ad != expected.Dot1AD {
		return errors.New("live PPP VLAN tags differ from committed classification")
	}
	return nil
}
