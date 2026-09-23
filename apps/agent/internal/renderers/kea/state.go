package kea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// LeasePageSize is the lease4/6-get-page limit: leases are always read in pages, never with
// one lease4-get-all message (Kea builds the whole answer in memory).
const LeasePageSize = 1000

// MaxLeases bounds one Retrieve (DaemonState.LeasesTruncated reports the cut).
const MaxLeases = 100_000

// State is the actual Kea state.
type State struct {
	Dhcp4 DaemonState `json:"dhcp4"`
	Dhcp6 DaemonState `json:"dhcp6"`
}

// DaemonState is one server's state as Kea reports it (JSON passed through).
type DaemonState struct {
	Running         bool              `json:"running"`
	Status          json.RawMessage   `json:"status,omitempty"`
	Config          json.RawMessage   `json:"config,omitempty"`
	Statistics      json.RawMessage   `json:"statistics,omitempty"`
	Leases          []json.RawMessage `json:"leases"`
	LeasesTruncated bool              `json:"leasesTruncated,omitempty"`
	// LeasesUnsupported: the lease_cmds hook is not loaded.
	LeasesUnsupported bool `json:"leasesUnsupported,omitempty"`
}

// State reads both servers. A server that is not running is reported with Running false.
func (r *Renderer) State(ctx context.Context) (State, error) {
	var st State
	var err error
	if st.Dhcp4, err = r.daemonState(ctx, 4); err != nil {
		return State{}, err
	}
	if st.Dhcp6, err = r.daemonState(ctx, 6); err != nil {
		return State{}, err
	}
	return st, nil
}

func (r *Renderer) daemonState(ctx context.Context, fam int) (DaemonState, error) {
	ds := DaemonState{Leases: []json.RawMessage{}}
	status, err := r.ctrl.Command(ctx, fam, "status-get", nil)
	if errors.Is(err, ErrNotRunning) {
		return ds, nil
	}
	if err != nil {
		return ds, fmt.Errorf("kea: dhcp%d status-get: %w", fam, err)
	}
	ds.Running, ds.Status = true, status.Arguments
	cfg, err := r.ConfigGet(ctx, fam)
	if err != nil {
		return ds, err
	}
	ds.Config = cfg
	stats, err := r.ctrl.Command(ctx, fam, "statistic-get-all", nil)
	if err != nil {
		return ds, fmt.Errorf("kea: dhcp%d statistic-get-all: %w", fam, err)
	}
	ds.Statistics = stats.Arguments
	leases, truncated, err := r.Leases(ctx, fam, MaxLeases)
	switch {
	case errors.Is(err, errUnsupported):
		ds.LeasesUnsupported = true
	case err != nil:
		return ds, err
	default:
		ds.Leases, ds.LeasesTruncated = leases, truncated
	}
	return ds, nil
}

// ConfigGet returns the running configuration of family 4 or 6 (`config-get`), with the
// "hash" Kea adds removed.
func (r *Renderer) ConfigGet(ctx context.Context, fam int) (json.RawMessage, error) {
	resp, err := r.ctrl.Command(ctx, fam, "config-get", nil)
	if err != nil {
		return nil, fmt.Errorf("kea: dhcp%d config-get: %w", fam, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(resp.Arguments, &m); err != nil {
		return nil, fmt.Errorf("kea: dhcp%d config-get: %w", fam, err)
	}
	delete(m, "hash")
	return json.Marshal(m)
}

var errUnsupported = errors.New("kea: command unsupported (hook not loaded)")

// Leases reads every lease of family 4 or 6 with lease4/6-get-page (LeasePageSize per
// message), up to limit leases.
func (r *Renderer) Leases(ctx context.Context, fam, limit int) ([]json.RawMessage, bool, error) {
	cmd := fmt.Sprintf("lease%d-get-page", fam)
	from := "start"
	out := []json.RawMessage{}
	for {
		resp, err := r.ctrl.Command(ctx, fam, cmd, map[string]any{"from": from, "limit": LeasePageSize})
		if err != nil {
			if resp.Result == ResultUnsupported {
				return nil, false, errUnsupported
			}
			return nil, false, fmt.Errorf("kea: %s: %w", cmd, err)
		}
		if resp.Result == ResultEmpty || len(resp.Arguments) == 0 {
			return out, false, nil
		}
		var page struct {
			Leases []json.RawMessage `json:"leases"`
			Count  int               `json:"count"`
		}
		if err := json.Unmarshal(resp.Arguments, &page); err != nil {
			return nil, false, fmt.Errorf("kea: %s: %w", cmd, err)
		}
		out = append(out, page.Leases...)
		if len(out) >= limit {
			return out[:limit], true, nil
		}
		if len(page.Leases) < LeasePageSize {
			return out, false, nil
		}
		var last struct {
			IPAddress string `json:"ip-address"`
		}
		if err := json.Unmarshal(page.Leases[len(page.Leases)-1], &last); err != nil || last.IPAddress == "" {
			return nil, false, fmt.Errorf("kea: %s: page without ip-address", cmd)
		}
		from = last.IPAddress
	}
}

// Statistics returns a flat name → value map of the latest sample of every statistic
// (`statistic-get-all`), e.g. "pkt4-received" → "12", "subnet[1].assigned-addresses" → "0".
func (r *Renderer) Statistics(ctx context.Context, fam int) (map[string]string, error) {
	resp, err := r.ctrl.Command(ctx, fam, "statistic-get-all", nil)
	if err != nil {
		return nil, err
	}
	return flattenStats(resp.Arguments)
}

// flattenStats turns {"name": [[value, "timestamp"], ...]} into name → latest value.
func flattenStats(raw json.RawMessage) (map[string]string, error) {
	var m map[string][][]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("kea: statistics: %w", err)
	}
	out := make(map[string]string, len(m))
	for name, samples := range m {
		if len(samples) == 0 || len(samples[0]) == 0 {
			continue
		}
		v := string(samples[0][0])
		if s, err := strconv.Unquote(v); err == nil {
			v = s
		}
		out[name] = v
	}
	return out, nil
}

// ConfigDrift compares a rendered Dhcp4/Dhcp6 configuration with the one Kea reports
// (config-get) and lists every rendered value that is missing or different. Kea adds
// defaults, so the comparison is a subset match: objects need every rendered key, arrays the
// same length with element-wise matches, scalars equal values. An empty result means the
// daemon runs exactly the rendered configuration.
func ConfigDrift(rendered, running []byte) ([]string, error) {
	var want, got any
	if err := json.Unmarshal(rendered, &want); err != nil {
		return nil, fmt.Errorf("kea: drift: rendered: %w", err)
	}
	if err := json.Unmarshal(running, &got); err != nil {
		return nil, fmt.Errorf("kea: drift: running: %w", err)
	}
	var diffs []string
	subset("", want, got, &diffs)
	sort.Strings(diffs)
	return diffs, nil
}

// normPool turns a Kea pool ("a-b" or the "p/len" form Kea reports for aligned ranges) into
// "first-last".
func normPool(s string) string {
	if p, err := netip.ParsePrefix(strings.TrimSpace(s)); err == nil {
		p = p.Masked()
		return p.Addr().String() + "-" + lastAddr(p).String()
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return s
	}
	x, err1 := netip.ParseAddr(strings.TrimSpace(a))
	y, err2 := netip.ParseAddr(strings.TrimSpace(b))
	if err1 != nil || err2 != nil {
		return s
	}
	return x.String() + "-" + y.String()
}

// lastAddr is the highest address of p.
func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().AsSlice()
	for i := p.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 1 << (7 - i%8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

func subset(path string, want, got any, diffs *[]string) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			*diffs = append(*diffs, fmt.Sprintf("%s: want object, got %T", path, got))
			return
		}
		for k, wv := range w {
			gv, ok := g[k]
			if !ok {
				*diffs = append(*diffs, fmt.Sprintf("%s/%s: missing", path, k))
				continue
			}
			subset(path+"/"+k, wv, gv, diffs)
		}
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			*diffs = append(*diffs, fmt.Sprintf("%s: want %d elements, got %v", path, len(w), got))
			return
		}
		for i := range w {
			subset(fmt.Sprintf("%s/%d", path, i), w[i], g[i], diffs)
		}
	default:
		if strings.HasSuffix(path, "/pool") {
			want, got = normPool(fmt.Sprint(want)), normPool(fmt.Sprint(got))
		}
		if fmt.Sprint(want) != fmt.Sprint(got) {
			*diffs = append(*diffs, fmt.Sprintf("%s: want %v, got %v", path, want, got))
		}
	}
}
