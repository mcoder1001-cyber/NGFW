package nftables

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"

	"ngfw/agent/internal/objects"
)

// KernelTable is `nft -j list table inet <table>` decoded: what the kernel holds, with counters.
type KernelTable struct {
	Sets   []*Set
	Chains []*KernelChain
	// Dormant: the table carries `flags dormant` (it exists but filters nothing).
	Dormant bool
}

// KernelChain is one chain of the kernel table.
type KernelChain struct {
	Name, Type, Hook, Policy string
	Priority                 int32
	Rules                    []KernelRule
}

// KernelRule is one rule: its comment (identity), verdict, counter and the hash of its body.
type KernelRule struct {
	Comment        string
	Verdict        string
	Packets, Bytes uint64
	// ExprHash is sha256 of the rule's `expr` JSON (whitespace-normalised, counter values stripped): it
	// changes with any edit of the rule body, whatever nft's re-printing looks like (fix round 1, M1).
	ExprHash string
}

// Hashes are the rule-body hashes by "<chain>/<comment>" (the store's kernel_hashes).
func (k *KernelTable) Hashes() map[string]string {
	out := map[string]string{}
	for _, c := range k.Chains {
		for _, r := range c.Rules {
			if r.Comment != "" {
				out[c.Name+"/"+r.Comment] = r.ExprHash
			}
		}
	}
	return out
}

// exprHash hashes a rule's statements; a counter statement counts as the word "counter".
func exprHash(expr []map[string]json.RawMessage) string {
	h := sha256.New()
	for _, st := range expr {
		for _, name := range sortedKeys(st) {
			h.Write([]byte(name))
			h.Write([]byte{0})
			if name != "counter" {
				var buf bytes.Buffer
				if json.Compact(&buf, st[name]) == nil {
					h.Write(buf.Bytes())
				} else {
					h.Write(st[name])
				}
			}
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ParseKernel decodes the JSON of `nft -j list table inet <table>` (libnftables JSON schema 1). Set
// elements are normalised to canonical prefixes (a single address → /32 or /128, a range → its CIDR
// set), sorted; chains keep the kernel's rule order.
func ParseKernel(raw []byte, table string) (*KernelTable, error) {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("nftables: decode nft -j: %w", err)
	}
	k := &KernelTable{}
	chains := map[string]*KernelChain{}
	var order []string
	for _, item := range doc.Nftables {
		for kind, body := range item {
			switch kind {
			case "table":
				var tb struct {
					Name  string
					Flags []string
				}
				if err := json.Unmarshal(body, &tb); err != nil {
					return nil, fmt.Errorf("nftables: decode table: %w", err)
				}
				if tb.Name == table && slices.Contains(tb.Flags, "dormant") {
					k.Dormant = true
				}
			case "chain":
				var c struct {
					Table, Name, Type, Hook, Policy string
					Prio                            int32
				}
				if err := json.Unmarshal(body, &c); err != nil {
					return nil, fmt.Errorf("nftables: decode chain: %w", err)
				}
				if c.Table != table {
					continue
				}
				kc := &KernelChain{Name: c.Name, Type: c.Type, Hook: c.Hook, Policy: c.Policy, Priority: c.Prio}
				chains[c.Name] = kc
				order = append(order, c.Name)
			case "set":
				var s struct {
					Table, Name, Type string
					Elem              []json.RawMessage
				}
				if err := json.Unmarshal(body, &s); err != nil {
					return nil, fmt.Errorf("nftables: decode set: %w", err)
				}
				if s.Table != table {
					continue
				}
				elems, err := setElements(s.Elem)
				if err != nil {
					return nil, fmt.Errorf("nftables: set %s: %w", s.Name, err)
				}
				k.Sets = append(k.Sets, &Set{Name: s.Name, Type: s.Type, Elements: elems})
			case "rule":
				var r struct {
					Table, Chain, Comment string
					Expr                  []map[string]json.RawMessage
				}
				if err := json.Unmarshal(body, &r); err != nil {
					return nil, fmt.Errorf("nftables: decode rule: %w", err)
				}
				c, ok := chains[r.Chain]
				if r.Table != table || !ok {
					continue
				}
				kr := KernelRule{Comment: r.Comment, ExprHash: exprHash(r.Expr)}
				for _, st := range r.Expr {
					for name, v := range st {
						switch name {
						case "counter":
							var cnt struct{ Packets, Bytes uint64 }
							if json.Unmarshal(v, &cnt) == nil {
								kr.Packets, kr.Bytes = cnt.Packets, cnt.Bytes
							}
						case "accept", "drop", "reject":
							kr.Verdict = name
						}
					}
				}
				c.Rules = append(c.Rules, kr)
			}
		}
	}
	for _, n := range order {
		k.Chains = append(k.Chains, chains[n])
	}
	sort.SliceStable(k.Chains, func(i, j int) bool {
		a, b := k.Chains[i], k.Chains[j]
		oa, ok1 := hookOrder[a.Hook]
		ob, ok2 := hookOrder[b.Hook]
		if !ok1 {
			oa = 3
		}
		if !ok2 {
			ob = 3
		}
		if oa != ob {
			return oa < ob
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.Name < b.Name
	})
	sort.Slice(k.Sets, func(i, j int) bool { return k.Sets[i].Name < k.Sets[j].Name })
	return k, nil
}

// setElements normalises the elements of an interval set of addresses.
func setElements(raw []json.RawMessage) ([]string, error) {
	var ps []netip.Prefix
	var add func(json.RawMessage) error
	add = func(e json.RawMessage) error {
		var s string
		if json.Unmarshal(e, &s) == nil {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return fmt.Errorf("element %q", s)
			}
			ps = append(ps, netip.PrefixFrom(a, a.BitLen()))
			return nil
		}
		var obj struct {
			Prefix *struct {
				Addr string
				Len  int
			}
			Range []string
			Elem  *struct {
				Val json.RawMessage
			}
		}
		if err := json.Unmarshal(e, &obj); err != nil {
			return fmt.Errorf("element %s: %w", e, err)
		}
		switch {
		case obj.Prefix != nil:
			p, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", obj.Prefix.Addr, obj.Prefix.Len))
			if err != nil {
				return fmt.Errorf("element prefix %v", *obj.Prefix)
			}
			ps = append(ps, p.Masked())
		case len(obj.Range) == 2:
			a, err1 := netip.ParseAddr(obj.Range[0])
			b, err2 := netip.ParseAddr(obj.Range[1])
			if err1 != nil || err2 != nil {
				return fmt.Errorf("element range %v", obj.Range)
			}
			rs, err := objects.RangeToPrefixes(a, b)
			if err != nil {
				return err
			}
			ps = append(ps, rs...)
		case obj.Elem != nil:
			return add(obj.Elem.Val)
		default:
			return fmt.Errorf("element %s", e)
		}
		return nil
	}
	for _, e := range raw {
		if err := add(e); err != nil {
			return nil, err
		}
	}
	sort.Slice(ps, func(i, j int) bool { return prefixLess(ps[i], ps[j]) })
	return prefixStrings(uniqPrefixes(ps)), nil
}

// Table is the kernel table as a descriptor value: sets, and chains with rules identified by comment and
// verdict only (the descriptor adds the configuration and the rules' annotations from its store).
// Non-filter or non-base chains keep their name with an empty hook, so they never equal a rendered one.
func (k *KernelTable) Table() *HostTable {
	v := &HostTable{Sets: k.Sets, Dormant: k.Dormant}
	for _, c := range k.Chains {
		ch := &Chain{Name: c.Name, Hook: c.Hook, Priority: c.Priority, Policy: c.Policy}
		if c.Type != "filter" {
			ch.Hook = ""
		}
		for _, r := range c.Rules {
			ch.Rules = append(ch.Rules, &Rule{Comment: r.Comment, Kind: kindOf(r.Comment), Verdict: r.Verdict})
		}
		v.Chains = append(v.Chains, ch)
	}
	return v
}

// kindOf derives the kind of a rule from its comment ("unknown" when the agent did not write it).
func kindOf(comment string) string {
	m := commentRe.FindStringSubmatch(comment)
	if m == nil {
		return KindUnknown
	}
	if id := m[1]; strings.HasPrefix(id, "@") {
		switch k := id[1:]; k {
		case KindEstablished, KindLoopback, KindICMP, KindAntiLockout:
			return k
		}
		return KindUnknown
	}
	return KindRule
}
