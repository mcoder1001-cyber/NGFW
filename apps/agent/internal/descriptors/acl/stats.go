package acl

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"go.fd.io/govpp/adapter"

	"ngfw/agent/internal/vpp"
)

// StatsSource is the subset of go.fd.io/govpp/adapter.StatsAPI the counter reader needs.
// *statsclient.StatsClient (connected to /run/vpp/stats.sock) satisfies it; unit tests use a map.
type StatsSource interface {
	ListStats(patterns ...string) ([]adapter.StatIdentifier, error)
	DumpStats(patterns ...string) ([]adapter.StatEntry, error)
}

// ErrNoCounters is returned (wrapped) when the stats segment has no counter vector for an ACL.
var ErrNoCounters = errors.New("acl: no counters in the stats segment")

// StatsPath is the stats-segment name of the combined (packets, bytes) counter vector of one
// ACL, one entry per rule in rule order: "/acl/<acl_index>/matches" (acl.c
// validate_and_reset_acl_counters). VPP registers it when the ACL is created and allocates one
// slot more than the rule count; the data plane increments it only while acl.stats-enable is on.
func StatsPath(aclIndex uint32) string { return fmt.Sprintf("/acl/%d/matches", aclIndex) }

// StatsPathPattern is the regexp (govpp patterns are regexps) matching every ACL counter vector.
const StatsPathPattern = `^/acl/[0-9]+/matches$`

// RuleCounter is the hit counter of one rule, summed over all VPP workers.
type RuleCounter struct {
	Packets uint64
	Bytes   uint64
}

// Counters is the per-rule hit counters of one owned ACL.
type Counters struct {
	Name     string
	ACLIndex uint32
	Rules    []RuleCounter // len == number of rules of the ACL, in rule order
}

// StatsReader reads ACL hit counters from the stats segment. It is what the F-* features and
// StreamStats call (at 1 Hz); it is not a descriptor: counters are read-only state.
type StatsReader struct {
	stats  StatsSource
	client vpp.Client
	owner  string
}

// NewStatsReader returns a reader over stats (the stats segment) for the ACLs owned by owner
// (resolved through client with acl_dump).
func NewStatsReader(stats StatsSource, client vpp.Client, owner string) *StatsReader {
	return &StatsReader{stats: stats, client: client, owner: owner}
}

// ReadIndex returns every counter slot of the vector of aclIndex (rule order; VPP allocates one
// slot more than the rule count, so callers with the rule count truncate — ReadOwned does).
func (r *StatsReader) ReadIndex(aclIndex uint32) ([]RuleCounter, error) {
	path := StatsPath(aclIndex)
	entries, err := r.stats.DumpStats("^" + regexp.QuoteMeta(path) + "$")
	if err != nil {
		return nil, fmt.Errorf("stats dump %s: %w", path, err)
	}
	for _, e := range entries {
		if string(e.Name) != path {
			continue
		}
		return reduceCombined(e)
	}
	return nil, fmt.Errorf("%w: %s", ErrNoCounters, path)
}

// reduceCombined sums a per-worker combined counter vector into one RuleCounter per index.
func reduceCombined(e adapter.StatEntry) ([]RuleCounter, error) {
	cc, ok := e.Data.(adapter.CombinedCounterStat)
	if !ok {
		return nil, fmt.Errorf("acl: %s is %T (type %v), want a combined counter vector", e.Name, e.Data, e.Type)
	}
	var out []RuleCounter
	for _, worker := range cc {
		for i, c := range worker {
			for len(out) <= i {
				out = append(out, RuleCounter{})
			}
			out[i].Packets += c.Packets()
			out[i].Bytes += c.Bytes()
		}
	}
	return out, nil
}

// ReadOwned returns the counters of every ACL of this owner, in acl_index order, truncated to
// each ACL's rule count. An ACL without a counter vector yet is reported with all-zero rules.
func (r *StatsReader) ReadOwned(ctx context.Context) ([]Counters, error) {
	owned, err := dumpOwnedACLs(ctx, r.client, r.owner)
	if err != nil {
		return nil, err
	}
	out := make([]Counters, 0, len(owned))
	for _, a := range owned {
		slots, err := r.ReadIndex(a.Index)
		if err != nil && !errors.Is(err, ErrNoCounters) {
			return nil, err
		}
		rules := make([]RuleCounter, len(a.Rules))
		copy(rules, slots) // copies min(len) — drops VPP's extra slot, zero-fills missing ones
		out = append(out, Counters{Name: a.Name, ACLIndex: a.Index, Rules: rules})
	}
	return out, nil
}

// ListPaths returns the names of all ACL counter vectors in the stats segment (all owners),
// for discovery and diagnostics.
func (r *StatsReader) ListPaths() ([]string, error) {
	ids, err := r.stats.ListStats(StatsPathPattern)
	if err != nil {
		return nil, fmt.Errorf("stats list: %w", err)
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id.Name))
	}
	return out, nil
}
