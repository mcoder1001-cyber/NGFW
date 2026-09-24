package acl

import (
	vrxv1 "ngfw/agent/gen/vrx/v1"
	descacl "ngfw/agent/internal/descriptors/acl"
)

// Paging limits of AclState (docs/contracts/proto.md §11 F-acl).
const (
	DefaultLimit  = 100
	MaxLimit      = 1000
	MaxSequences  = 1000
	maxRuleCounts = int(^uint32(0) >> 1)
)

// sumRange sums the counters of VPP rules [first, first+count), ignoring slots VPP did not report.
func sumRange(counters []descacl.RuleCounter, first, count uint32) (packets, bytes uint64) {
	end := int(first) + int(count)
	if end > len(counters) {
		end = len(counters)
	}
	for i := int(first); i < end; i++ {
		packets += counters[i].Packets
		bytes += counters[i].Bytes
	}
	return packets, bytes
}

// Sum is the total of the first n counters (n < 0: all).
func Sum(counters []descacl.RuleCounter, n int) (packets, bytes uint64) {
	if n < 0 || n > len(counters) {
		n = len(counters)
	}
	return sumRange(counters, 0, uint32(n)) //nolint:gosec // n ≤ len(counters)
}

// RulePage maps VPP rule counters back to the configuration rules of exp and returns the page
// [offset, offset+limit) of the rules that pass the filter (sequence order), and the filtered total.
// counters may be nil (unavailable): every rule then reports 0, and hits_only selects nothing.
func RulePage(exp *Expansion, counters []descacl.RuleCounter, f *vrxv1.AclStateFilter, offset, limit int) ([]*vrxv1.AclRuleState, int) {
	var want map[uint32]bool
	if seqs := f.GetSequences(); len(seqs) > 0 {
		want = make(map[uint32]bool, len(seqs))
		for _, s := range seqs {
			want[s] = true
		}
	}
	total := 0
	var page []*vrxv1.AclRuleState
	for _, r := range exp.Rules {
		if want != nil && !want[r.Sequence] {
			continue
		}
		var packets, bytes uint64
		if r.Count > 0 && counters != nil {
			packets, bytes = sumRange(counters, r.First, r.Count)
		}
		if f.GetHitsOnly() && packets == 0 {
			continue
		}
		if total >= offset && len(page) < limit {
			page = append(page, &vrxv1.AclRuleState{
				Sequence: r.Sequence, Status: r.Status, VppRules: r.Count, FirstVppRule: r.First,
				Packets: packets, Bytes: bytes,
			})
		}
		total++
	}
	return page, total
}

// ListState is the summary of one tracked ACL (mapping known when exp is not nil).
func ListState(a Applied, exp *Expansion, counters []descacl.RuleCounter) *vrxv1.AclListState {
	st := &vrxv1.AclListState{Name: a.Name, AclIndex: a.Index, VppRules: uint32(min(a.VPPRules, maxRuleCounts))} //nolint:gosec // bounded
	if exp != nil {
		st.MappingKnown = true
		st.ConfigRules = uint32(min(len(exp.Rules), maxRuleCounts)) //nolint:gosec // bounded
	}
	st.Packets, st.Bytes = Sum(counters, a.VPPRules)
	return st
}
