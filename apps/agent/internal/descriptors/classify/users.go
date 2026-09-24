package classify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"

	"go.fd.io/govpp/adapter"

	classifyapi "ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	isr "ngfw/agent/binapi/ip_session_redirect"
	ipfixapi "ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/vpp"
)

// ErrTableInUse is returned by TableDescriptor.Delete while something still refers to the
// table: deleting it would leave that binding pointing at a freed index, and the first packet
// classified through it crashes VPP (vnet_classify_find_entry, VPP V19 / D-095).
var ErrTableInUse = errors.New("classify table still in use")

// BindingRecord is a write-only binding (interface-ip-table, interface-l2-tables) this owner
// applied: VPP cannot read these back, so the table's Delete learns about them from here.
type BindingRecord struct {
	Key       string   `json:"key"`
	Interface string   `json:"interface"`
	SwIfIndex uint32   `json:"sw_if_index"`
	Tables    []uint32 `json:"tables"`
}

func unknownMsg(err error) bool {
	var unknown *adapter.UnknownMsgError
	return errors.As(err, &unknown)
}

// TableUsers lists what refers to classify table index, re-read from VPP wherever VPP offers a
// readback: another table chaining to it (classify_table_info.next_table_index), an input ACL
// on any interface (classify_table_by_interface over sw_interface_dump), the punt ACL
// (punt_acl_get), the ipfix classify tables (ipfix_classify_table_dump) and ip_session_redirect
// sessions (ip_session_redirect_dump). Bindings VPP cannot report — output ACL,
// interface-ip-table and interface-l2-tables — come from st's records, and only count while
// their interface still exists (VPP refuses every call on a deleted interface; what is left on
// its index is cleared by the next creator's ifsanitize.Sanitize). Policer and flow classify
// bindings have no usable readback in VPP 26.06 (the dumps read out of bounds): their
// descriptors' dependency on the table orders their deletion first.
func TableUsers(ctx context.Context, c vpp.Client, st Store, index uint32) ([]string, error) {
	var users []string
	cl := classifyapi.NewServiceClient(c)
	ids, err := tableIDs(ctx, c)
	if err != nil {
		return nil, err
	}
	for id := range ids {
		if id == index {
			continue
		}
		info, err := cl.ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: id})
		if err != nil {
			continue // deleted since classify_table_ids
		}
		if info.NextTableIndex == index {
			users = append(users, fmt.Sprintf("classify table %d chains to it", id))
		}
	}
	live := map[uint32]string{}
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_dump: %w", err)
		}
		live[uint32(d.SwIfIndex)] = d.InterfaceName
	}
	for idx, name := range live {
		rep, err := cl.ClassifyTableByInterface(ctx, &classifyapi.ClassifyTableByInterface{SwIfIndex: interface_types.InterfaceIndex(idx)})
		if df2.InterfaceVanished(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("classify_table_by_interface %d: %w", idx, err)
		}
		for kind, t := range map[string]uint32{"ip4": rep.IP4TableID, "ip6": rep.IP6TableID, "l2": rep.L2TableID} {
			if t == index {
				users = append(users, fmt.Sprintf("input-acl %s on %s (sw_if_index %d)", kind, name, idx))
			}
		}
	}
	if punt, err := cl.PuntACLGet(ctx, &classifyapi.PuntACLGet{}); err == nil {
		if punt.IP4TableIndex == index || punt.IP6TableIndex == index {
			users = append(users, "punt-acl")
		}
	} else if !unknownMsg(err) {
		return nil, fmt.Errorf("punt_acl_get: %w", err)
	}
	if s, err := ipfixapi.NewServiceClient(c).IpfixClassifyTableDump(ctx, &ipfixapi.IpfixClassifyTableDump{}); err == nil {
		for {
			d, err := s.Recv()
			if errors.Is(err, io.EOF) || unknownMsg(err) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("ipfix_classify_table_dump: %w", err)
			}
			if d.TableID == index {
				users = append(users, "ipfix classify table")
			}
		}
	} else if !unknownMsg(err) {
		return nil, fmt.Errorf("ipfix_classify_table_dump: %w", err)
	}
	if s, err := isr.NewServiceClient(c).IPSessionRedirectDump(ctx, &isr.IPSessionRedirectDump{TableIndex: index}); err == nil {
		n := 0
		for {
			d, err := s.Recv()
			if errors.Is(err, io.EOF) || unknownMsg(err) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("ip_session_redirect_dump: %w", err)
			}
			if d.TableIndex == index {
				n++
			}
		}
		if n > 0 {
			users = append(users, fmt.Sprintf("%d ip-session-redirect session(s)", n))
		}
	} else if !unknownMsg(err) {
		return nil, fmt.Errorf("ip_session_redirect_dump: %w", err)
	}
	for _, o := range st.Outputs() {
		if (o.IP4Table != "" && o.IP4Index == index) || (o.IP6Table != "" && o.IP6Index == index) {
			users = append(users, "output-acl on "+o.Interface+" (record)")
		}
	}
	for _, b := range st.Bindings() {
		for _, t := range b.Tables {
			if t != index {
				continue
			}
			if _, ok := live[b.SwIfIndex]; !ok {
				slog.Default().Warn("classify: write-only binding record of a deleted interface ignored (its index is sanitized on reuse, VPP V19)",
					"binding", b.Key, "sw_if_index", b.SwIfIndex, "table", index)
				continue
			}
			users = append(users, b.Key+" (record)")
		}
	}
	sort.Strings(users)
	return users, nil
}
