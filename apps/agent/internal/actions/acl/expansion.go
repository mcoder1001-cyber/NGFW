// Package acl is F-acl's runtime-state side of the acl domain: the record of how each configured
// list expanded into VPP rules (so hit counters map back to configuration rules), the tracker of
// what the acl.acl / acl.macip-acl descriptors last saw in VPP, the counter reader and the
// AclState mapping (pure functions, tested on the fake client). The projection itself lives in
// internal/desired/acl.go; the wiring in internal/subsystems/acl.go.
package acl

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"sync"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	descacl "ngfw/agent/internal/descriptors/acl"
)

// RuleInfo is one configuration rule of an expanded list: its sequence, whether it rendered, and
// the contiguous block [First, First+Count) of VPP rules it expanded to.
type RuleInfo struct {
	Sequence uint32
	Status   vrxv1.AclRuleStatus
	First    uint32
	Count    uint32
	// Schedule is the rule's schedule name ("" = always); FQDN the FQDN address objects its
	// source/destination reference (transitively). Both drive the re-projection watcher.
	Schedule string
	FQDN     []string
}

// Expansion is how one configured L3/L4 list became one VPP ACL.
type Expansion struct {
	Name        string
	Fingerprint string
	// Rules are the configuration rules in sequence order (also those that rendered nothing).
	Rules []RuleInfo
	// VPPRules is the number of VPP rules (the sum of the Counts).
	VPPRules int
	// Config is the configuration list that produced it (Retrieve assembles the domain from it).
	Config *vrxv1.AclList
	// Schedules are the definitions of the schedules the rules name (as projected).
	Schedules map[string]*vrxv1.Schedule
}

// MacipExpansion is how one configured MACIP list became one VPP MACIP ACL.
type MacipExpansion struct {
	Name        string
	Fingerprint string
	Config      *vrxv1.MacipList
}

// Attachments is the configuration that produced a set of interface bindings.
type Attachments struct {
	Fingerprint string
	Attachments []*vrxv1.AclAttachment
	Macip       []*vrxv1.MacipAttachment
}

// Fingerprint identifies the VPP content of an ACL: SHA-256 over its rules in order. Two
// projections with the same fingerprint put exactly the same rules into VPP.
func Fingerprint(rules []descacl.Rule) string {
	h := sha256.New()
	var buf [16]byte
	for _, r := range rules {
		writeString(h, string(r.Action))
		writeString(h, r.Src)
		writeString(h, r.Dst)
		buf[0] = r.Proto
		binary.BigEndian.PutUint16(buf[1:], r.SrcPortFirst)
		binary.BigEndian.PutUint16(buf[3:], r.SrcPortLast)
		binary.BigEndian.PutUint16(buf[5:], r.DstPortFirst)
		binary.BigEndian.PutUint16(buf[7:], r.DstPortLast)
		buf[9], buf[10] = r.TCPFlagsMask, r.TCPFlagsValue
		h.Write(buf[:11])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// MacipFingerprint is Fingerprint for MACIP rules.
func MacipFingerprint(rules []descacl.MacipRule) string {
	h := sha256.New()
	for _, r := range rules {
		writeString(h, string(r.Action))
		writeString(h, r.SrcMac)
		writeString(h, r.SrcMacMask)
		writeString(h, r.SrcPrefix)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// BindingsFingerprint identifies a set of interface bindings and MACIP bindings (any order).
func BindingsFingerprint(bindings []descacl.InterfaceBinding, macip []descacl.MacipBinding) string {
	bs := append([]descacl.InterfaceBinding(nil), bindings...)
	sort.Slice(bs, func(i, j int) bool { return bs[i].Interface < bs[j].Interface })
	ms := append([]descacl.MacipBinding(nil), macip...)
	sort.Slice(ms, func(i, j int) bool { return ms[i].Interface < ms[j].Interface })
	h := sha256.New()
	for _, b := range bs {
		writeString(h, "b:"+b.Interface)
		for _, n := range b.Input {
			writeString(h, "i:"+n)
		}
		for _, n := range b.Output {
			writeString(h, "o:"+n)
		}
	}
	for _, m := range ms {
		writeString(h, "m:"+m.Interface+"="+m.ACL)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type hashWriter interface{ Write([]byte) (int, error) }

func writeString(h hashWriter, s string) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s))) //nolint:gosec // strings here are short
	_, _ = h.Write(n[:])
	_, _ = h.Write([]byte(s))
}

// ---- the expansion record ------------------------------------------------------------------------

// Record keeps the most recent expansions per list (and binding sets), keyed by content
// fingerprint, so what VPP holds (seen by the descriptors) can be attributed to the configuration
// that produced it. A projection that is never applied (DryRun) only adds a record nobody finds.
// Bounded: per name the last keepPerName fingerprints, and at most maxRules configuration rules
// over all L3/L4 records (a 100 000-rule list keeps about four versions).
type Record struct {
	mu       sync.Mutex
	acls     map[string][]*Expansion // name → newest first
	macips   map[string][]*MacipExpansion
	bindings []*Attachments // newest first
	maxRules int
}

const (
	keepPerName = 3
	keepSets    = 4
	// DefaultMaxRules bounds the configuration rules kept over all records.
	DefaultMaxRules = 400_000
)

// NewRecord returns an empty record bounded by maxRules configuration rules (≤ 0: DefaultMaxRules).
func NewRecord(maxRules int) *Record {
	if maxRules <= 0 {
		maxRules = DefaultMaxRules
	}
	return &Record{acls: map[string][]*Expansion{}, macips: map[string][]*MacipExpansion{}, maxRules: maxRules}
}

// Default is the process-wide record the projection writes and the runtime reads.
var Default = NewRecord(0)

// PutACL records e (replacing an older record with the same fingerprint).
func (r *Record) PutACL(e *Expansion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := []*Expansion{e}
	for _, old := range r.acls[e.Name] {
		if old.Fingerprint != e.Fingerprint && len(list) < keepPerName {
			list = append(list, old)
		}
	}
	r.acls[e.Name] = list
	r.trimLocked()
}

// trimLocked drops older records (never a list's newest) until the rule budget holds, the largest
// old record first.
func (r *Record) trimLocked() {
	total := 0
	for _, l := range r.acls {
		for _, e := range l {
			total += len(e.Rules)
		}
	}
	for total > r.maxRules {
		victim, n := "", -1
		for name, l := range r.acls {
			if len(l) > 1 && len(l[len(l)-1].Rules) > n {
				victim, n = name, len(l[len(l)-1].Rules)
			}
		}
		if victim == "" {
			return // only the newest record of each list is left
		}
		r.acls[victim] = r.acls[victim][:len(r.acls[victim])-1]
		total -= n
	}
}

// ACL returns the record of list name with this fingerprint.
func (r *Record) ACL(name, fp string) (*Expansion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.acls[name] {
		if e.Fingerprint == fp {
			return e, true
		}
	}
	return nil, false
}

// PutMacip records e.
func (r *Record) PutMacip(e *MacipExpansion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := []*MacipExpansion{e}
	for _, old := range r.macips[e.Name] {
		if old.Fingerprint != e.Fingerprint && len(list) < keepPerName {
			list = append(list, old)
		}
	}
	r.macips[e.Name] = list
}

// Macip returns the record of MACIP list name with this fingerprint.
func (r *Record) Macip(name, fp string) (*MacipExpansion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.macips[name] {
		if e.Fingerprint == fp {
			return e, true
		}
	}
	return nil, false
}

// PutAttachments records a.
func (r *Record) PutAttachments(a *Attachments) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := []*Attachments{a}
	for _, old := range r.bindings {
		if old.Fingerprint != a.Fingerprint && len(list) < keepSets {
			list = append(list, old)
		}
	}
	r.bindings = list
}

// Attachments returns the attachments that produced the binding set with this fingerprint.
func (r *Record) Attachments(fp string) (*Attachments, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.bindings {
		if a.Fingerprint == fp {
			return a, true
		}
	}
	return nil, false
}
