package kea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// Read-only DHCP status for the agent's DhcpLeases RPC (F-kea-dhcp-relay): daemon state, per-subnet pool usage and
// lease pages. Nothing here is part of Retrieve (review L3); every read goes over the daemons' own control sockets
// (D-079) and is bounded (lease4/6-get-page, MaxLeases per family).

// SubnetRef is one subnet of a running (or on-disk) configuration with the names the renderer put into its
// user-context.
type SubnetRef struct {
	ID             uint32
	Prefix         string
	Server, Subnet string
}

// SubnetUsage is the pool usage of one subnet.
type SubnetUsage struct {
	SubnetRef
	Total, Assigned, Declined uint64
}

// DaemonStatus is the state of one Kea daemon.
type DaemonStatus struct {
	Family  int
	Running bool
	// Active: the configuration the daemon runs (or loads at start) binds at least one interface.
	Active bool
	// ActionRequired is "start" when Active and not Running (derived on every read: repeated until acted on, D-079).
	ActionRequired string
	// ReloadSec is status-get "reload" (seconds since the last configuration load); 0 when unknown.
	ReloadSec uint64
	Subnets   []SubnetUsage
	// Err is set when the daemon could not be read (the other fields are best effort).
	Err string
}

// Status reads the state of the daemon of family 4 or 6. It never fails: read errors go into Err. A daemon whose
// configuration the agent did not write (no embedded render input) is reported as not configured: only Running.
func (r *Renderer) Status(ctx context.Context, family int) DaemonStatus {
	st := DaemonStatus{Family: family}
	cfg, running, err := r.actualConfig(ctx, family)
	if err != nil {
		st.Err = err.Error()
		return st
	}
	st.Running = running
	if _, ours, _ := EmbeddedInput(cfg); cfg == nil || !ours {
		// Not configured by the agent (review M1): no file, an idle configuration, or a foreign one — such as the
		// packaged, commented /etc/kea file. Nothing is active, nothing must be started, nothing is an error.
		return st
	}
	st.Active = active(cfg)
	if st.Active && !running {
		st.ActionRequired = "start"
	}
	refs, err := SubnetsOf(cfg)
	if err != nil {
		st.Err = err.Error()
		return st
	}
	if !running {
		for _, ref := range refs {
			st.Subnets = append(st.Subnets, SubnetUsage{SubnetRef: ref})
		}
		return st
	}
	if resp, err := r.ctrl.Command(ctx, family, "status-get", nil); err == nil {
		var s struct {
			Reload json.Number `json:"reload"`
		}
		if json.Unmarshal(resp.Arguments, &s) == nil {
			if n, err := strconv.ParseUint(s.Reload.String(), 10, 64); err == nil {
				st.ReloadSec = n
			}
		}
	}
	stats, err := r.Statistics(ctx, family)
	if err != nil {
		st.Err = fmt.Sprintf("statistic-get-all: %v", err)
	}
	total, assigned := "total-addresses", "assigned-addresses"
	if family == 6 {
		total, assigned = "total-nas", "assigned-nas"
	}
	for _, ref := range refs {
		u := SubnetUsage{SubnetRef: ref}
		key := func(s string) string { return fmt.Sprintf("subnet[%d].%s", ref.ID, s) }
		u.Total = statUint(stats[key(total)])
		u.Assigned = statUint(stats[key(assigned)])
		u.Declined = statUint(stats[key("declined-addresses")])
		st.Subnets = append(st.Subnets, u)
	}
	return st
}

// statUint parses a Kea statistic (an integer that may exceed 64 bits for DHCPv6 pools: capped).
func statUint(s string) uint64 {
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		if f >= math.MaxUint64 {
			return math.MaxUint64
		}
		return uint64(f)
	}
	return 0
}

// SubnetsOf lists the subnets of a Dhcp4/Dhcp6 configuration (nil config: none), sorted by id.
func SubnetsOf(cfg []byte) ([]SubnetRef, error) {
	if cfg == nil {
		return nil, nil
	}
	var root map[string]struct {
		Subnet4 []subnet `json:"subnet4"`
		Subnet6 []subnet `json:"subnet6"`
	}
	if err := json.Unmarshal(cfg, &root); err != nil {
		return nil, fmt.Errorf("kea: configuration: %w", err)
	}
	var out []SubnetRef
	for _, k := range []string{"Dhcp4", "Dhcp6"} {
		v, ok := root[k]
		if !ok {
			continue
		}
		for _, s := range append(v.Subnet4, v.Subnet6...) {
			ref := SubnetRef{ID: s.ID, Prefix: s.Subnet}
			if s.UserContext != nil {
				ref.Server, ref.Subnet = s.UserContext.VRX.Server, s.UserContext.VRX.Subnet
			}
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Lease is one Kea lease (lease4-get-page / lease6-get-page element).
type Lease struct {
	Family    int
	Address   string
	HWAddress string
	ClientID  string
	DUID      string
	Hostname  string
	SubnetID  uint32
	ValidLft  uint32
	CLTT      int64
	State     int
	Type      string // DHCPv6: IA_NA / IA_PD
	PrefixLen uint32
}

// Expires is the lease's expiry as Unix seconds (cltt + valid-lft).
func (l Lease) Expires() int64 { return l.CLTT + int64(l.ValidLft) }

// StateName is Kea's lease state as text.
func (l Lease) StateName() string {
	switch l.State {
	case 0:
		return "default"
	case 1:
		return "declined"
	case 2:
		return "expired-reclaimed"
	}
	return "state-" + strconv.Itoa(l.State)
}

type rawLease struct {
	IPAddress string `json:"ip-address"`
	HWAddress string `json:"hw-address"`
	ClientID  string `json:"client-id"`
	DUID      string `json:"duid"`
	Hostname  string `json:"hostname"`
	SubnetID  uint32 `json:"subnet-id"`
	ValidLft  uint32 `json:"valid-lft"`
	CLTT      int64  `json:"cltt"`
	State     int    `json:"state"`
	Type      string `json:"type"`
	PrefixLen uint32 `json:"prefix-len"`
}

// LeaseQuery selects leases.
type LeaseQuery struct {
	// Families to read (4, 6); empty = both.
	Families []int
	// SubnetIDs keeps only leases of these subnets per family (nil = every subnet).
	SubnetIDs map[int]map[uint32]bool
	// Filter is a case-insensitive substring of address, hardware address, client id, DUID or hostname.
	Filter string
	// Offset and Limit select the page of the sorted result (Limit 0 = everything).
	Offset, Limit int
}

// LeasePage reads the leases of the selected families (paged from Kea, at most MaxLeases each), filters and sorts
// them (family, then address) and returns one page, the number of matching leases and whether a family had more
// leases than were read. A family whose daemon is not running contributes nothing (Status reports it).
func (r *Renderer) LeasePage(ctx context.Context, q LeaseQuery) ([]Lease, int, bool, error) {
	fams := q.Families
	if len(fams) == 0 {
		fams = []int{4, 6}
	}
	needle := strings.ToLower(strings.TrimSpace(q.Filter))
	var all []Lease
	truncated := false
	for _, fam := range fams {
		raw, more, err := r.Leases(ctx, fam, MaxLeases)
		switch {
		case errors.Is(err, ErrNotRunning):
			continue
		case err != nil:
			return nil, 0, false, err
		}
		truncated = truncated || more
		keep := q.SubnetIDs[fam]
		for _, b := range raw {
			var rl rawLease
			if err := json.Unmarshal(b, &rl); err != nil {
				return nil, 0, false, fmt.Errorf("kea: lease%d: %w", fam, err)
			}
			if q.SubnetIDs != nil && !keep[rl.SubnetID] {
				continue
			}
			l := Lease{Family: fam, Address: rl.IPAddress, HWAddress: strings.ToLower(rl.HWAddress), ClientID: strings.ToLower(rl.ClientID),
				DUID: strings.ToLower(rl.DUID), Hostname: rl.Hostname, SubnetID: rl.SubnetID, ValidLft: rl.ValidLft, CLTT: rl.CLTT,
				State: rl.State, Type: rl.Type, PrefixLen: rl.PrefixLen}
			if needle != "" && !matches(l, needle) {
				continue
			}
			all = append(all, l)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Family != all[j].Family {
			return all[i].Family < all[j].Family
		}
		a, errA := netip.ParseAddr(all[i].Address)
		b, errB := netip.ParseAddr(all[j].Address)
		if errA != nil || errB != nil {
			return all[i].Address < all[j].Address
		}
		return a.Less(b)
	})
	total := len(all)
	if q.Offset > total {
		q.Offset = total
	}
	end := total
	if q.Limit > 0 && q.Offset+q.Limit < total {
		end = q.Offset + q.Limit
	}
	return all[q.Offset:end], total, truncated, nil
}

func matches(l Lease, needle string) bool {
	for _, s := range []string{l.Address, l.HWAddress, l.ClientID, l.DUID, strings.ToLower(l.Hostname)} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
