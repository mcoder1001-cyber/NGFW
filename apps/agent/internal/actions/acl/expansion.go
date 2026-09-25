// Package acl is F-acl's runtime-state side of the acl domain: the record of how each configured
// list expanded into VPP rules (so hit counters map back to configuration rules), the applied
// configuration (the agent-local acl.config descriptor: written only by Apply, reverted by rollback),
// the tracker of what the acl.acl / acl.macip-acl descriptors last saw in VPP, the counter reader
// and the AclState mapping (pure functions, tested on the fake client). The projection itself lives
// in internal/desired/acl.go; the wiring in internal/subsystems/acl.go.
//
// Review H1: every projection (DryRun, validate, drift, Apply) records its expansion, keyed by list
// name + VPP content fingerprint + CONFIGURATION hash; readers look up the entry of the APPLIED
// configuration (acl.config), so a DryRun of a candidate never changes what Retrieve, AclState or the
// watcher see.
package acl

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"

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
	// ConfigHash identifies the configuration list that produced it (ConfigHash of the AclList).
	ConfigHash string
	// Rules are the configuration rules in sequence order (also those that rendered nothing).
	Rules []RuleInfo
	// VPPRules is the number of VPP rules (the sum of the Counts).
	VPPRules int
	// Schedules are the definitions of the schedules the rules name (as projected).
	Schedules map[string]*vrxv1.Schedule
}

// MacipExpansion is how one configured MACIP list became one VPP MACIP ACL.
type MacipExpansion struct {
	Name        string
	Fingerprint string
	ConfigHash  string
}

// Attachments says which attachments configuration produced a set of interface bindings.
type Attachments struct {
	Fingerprint string
	ConfigHash  string
}

// ConfigHash identifies a configuration message: SHA-256 over its deterministic protobuf encoding.
func ConfigHash(m proto.Message) string {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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

// Record keeps the expansions of recent projections per list (and binding sets), keyed by VPP
// content fingerprint and configuration hash, so what VPP holds can be attributed to the APPLIED
// configuration (Pin, set by the acl.config descriptor) and its VPP rules mapped to configuration
// rules. A projection that is never applied (DryRun, validate, drift) only adds entries nobody looks
// up. Bounded: per name at most keepPinned entries of the applied configuration (its expansion
// changes with schedules and FQDN answers) and keepOther others, and at most maxRules configuration
// rules over all L3/L4 entries (the newest entry of the applied configuration is never dropped).
type Record struct {
	mu       sync.Mutex
	acls     map[string][]*Expansion // name → newest first
	macips   map[string][]*MacipExpansion
	bindings []*Attachments    // newest first
	pinned   map[string]string // "acl/<name>", "macip/<name>", "attachments" → applied configuration hash
	maxRules int
}

const (
	keepPinned = 4
	keepOther  = 4
	keepSets   = 8
	// DefaultMaxRules bounds the configuration rules kept over all records.
	DefaultMaxRules = 400_000
)

// Pin keys of the applied configurations.
func pinACL(name string) string   { return "acl/" + name }
func pinMacip(name string) string { return "macip/" + name }

const pinAttachments = "attachments"

// NewRecord returns an empty record bounded by maxRules configuration rules (≤ 0: DefaultMaxRules).
func NewRecord(maxRules int) *Record {
	if maxRules <= 0 {
		maxRules = DefaultMaxRules
	}
	return &Record{acls: map[string][]*Expansion{}, macips: map[string][]*MacipExpansion{}, pinned: map[string]string{}, maxRules: maxRules}
}

// Default is the process-wide record the projection writes and the runtime reads.
var Default = NewRecord(0)

// pin records the applied configuration hash of a key ("" removes it).
func (r *Record) pin(key, hash string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if hash == "" {
		delete(r.pinned, key)
		return
	}
	r.pinned[key] = hash
}

// Pinned returns the applied configuration hash of an L3/L4 list ("" when none is applied).
func (r *Record) Pinned(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pinned[pinACL(name)]
}

// PutACL records e (replacing an entry with the same fingerprint and configuration hash).
func (r *Record) PutACL(e *Expansion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	applied := r.pinned[pinACL(e.Name)]
	list := []*Expansion{e}
	nPinned, nOther := 0, 0
	if e.ConfigHash == applied {
		nPinned++
	} else {
		nOther++
	}
	for _, old := range r.acls[e.Name] {
		if old.Fingerprint == e.Fingerprint && old.ConfigHash == e.ConfigHash {
			continue
		}
		switch {
		case old.ConfigHash == applied && nPinned < keepPinned:
			nPinned++
		case old.ConfigHash != applied && nOther < keepOther:
			nOther++
		default:
			continue
		}
		list = append(list, old)
	}
	r.acls[e.Name] = list
	r.trimLocked()
}

// trimLocked drops older entries until the rule budget holds: entries of unapplied configurations
// first (largest first), then older entries of applied ones. A list's newest entry (the projection
// that is being applied right now) and the newest entry of its applied configuration are never dropped.
func (r *Record) trimLocked() {
	total := 0
	for _, l := range r.acls {
		for _, e := range l {
			total += len(e.Rules)
		}
	}
	for total > r.maxRules {
		victim, at, n, applied := "", -1, -1, true
		for name, l := range r.acls {
			hash := r.pinned[pinACL(name)]
			newestApplied := true
			for i, e := range l {
				isApplied := e.ConfigHash == hash
				if isApplied && newestApplied {
					newestApplied = false
					continue
				}
				if i == 0 {
					continue
				}
				better := (!isApplied && applied) || (isApplied == applied && len(e.Rules) > n)
				if better {
					victim, at, n, applied = name, i, len(e.Rules), isApplied
				}
			}
		}
		if victim == "" {
			return
		}
		l := r.acls[victim]
		r.acls[victim] = append(l[:at:at], l[at+1:]...)
		total -= n
	}
}

// ACL returns the entry of list name with this VPP fingerprint and configuration hash.
func (r *Record) ACL(name, fp, configHash string) (*Expansion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.acls[name] {
		if e.Fingerprint == fp && e.ConfigHash == configHash {
			return e, true
		}
	}
	return nil, false
}

// AppliedACL returns the entry that explains VPP content fp by the applied configuration of name.
func (r *Record) AppliedACL(name, fp string) (*Expansion, bool) {
	hash := r.Pinned(name)
	if hash == "" {
		return nil, false
	}
	return r.ACL(name, fp, hash)
}

// PutMacip records e.
func (r *Record) PutMacip(e *MacipExpansion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	applied := r.pinned[pinMacip(e.Name)]
	list := []*MacipExpansion{e}
	n := 0
	for _, old := range r.macips[e.Name] {
		if old.Fingerprint == e.Fingerprint && old.ConfigHash == e.ConfigHash {
			continue
		}
		if old.ConfigHash == applied || n < keepOther {
			if old.ConfigHash != applied {
				n++
			}
			list = append(list, old)
		}
	}
	r.macips[e.Name] = list
}

// Macip returns the MACIP entry with this fingerprint and configuration hash.
func (r *Record) Macip(name, fp, configHash string) (*MacipExpansion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.macips[name] {
		if e.Fingerprint == fp && e.ConfigHash == configHash {
			return e, true
		}
	}
	return nil, false
}

// PutAttachments records a.
func (r *Record) PutAttachments(a *Attachments) {
	r.mu.Lock()
	defer r.mu.Unlock()
	applied := r.pinned[pinAttachments]
	list := []*Attachments{a}
	n := 0
	for _, old := range r.bindings {
		if old.Fingerprint == a.Fingerprint && old.ConfigHash == a.ConfigHash {
			continue
		}
		if old.ConfigHash == applied || n < keepSets {
			if old.ConfigHash != applied {
				n++
			}
			list = append(list, old)
		}
	}
	r.bindings = list
}

// Attachments reports whether the binding set with this fingerprint was produced by the attachments
// configuration with this hash.
func (r *Record) Attachments(fp, configHash string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.bindings {
		if a.Fingerprint == fp && a.ConfigHash == configHash {
			return true
		}
	}
	return false
}
