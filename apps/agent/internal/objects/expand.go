package objects

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Kind names one record of the objects document by its configuration (JSON) name; it is also the
// JSON pointer segment below /objects.
type Kind string

// The seven kinds of the objects domain (packages/schema/src/domains/objects.ts).
const (
	KindAddresses     Kind = "addresses"
	KindAddressGroups Kind = "addressGroups"
	KindServices      Kind = "services"
	KindServiceGroups Kind = "serviceGroups"
	KindSchedules     Kind = "schedules"
	KindZones         Kind = "zones"
	KindTags          Kind = "tags"
)

// Kinds lists every kind in dependency-friendly order (tags first, groups after their leaves).
var Kinds = []Kind{KindTags, KindAddresses, KindAddressGroups, KindServices, KindServiceGroups, KindSchedules, KindZones}

// MaxEntries is the expansion cap (WBS D5.1): an expansion — or an ACL rule built from several,
// see CheckLimit — with more entries is an error, which the consumer reports as a DryRun issue at
// the pointer of the rule that uses it.
const MaxEntries = 10000

// Errors of the expansion library (match with errors.Is; a cap violation is a *LimitError).
var (
	// ErrUnknownObject: the reference names no object of the expected kinds.
	ErrUnknownObject = errors.New("objects: unknown object")
	// ErrCycle: group membership loops (defence in depth — the schema rules reject cycles).
	ErrCycle = errors.New("objects: group membership cycle")
	// ErrInvalid: an object the schema would have rejected (bad address text, unknown type, …).
	ErrInvalid = errors.New("objects: invalid object")
)

// LimitError is returned when an expansion exceeds its cap.
type LimitError struct {
	Ref   string // the object (or rule) whose expansion is too large
	Count int    // entries it expands to (at the point the cap was hit)
	Limit int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("objects: %q expands to %d entries, more than the limit of %d", e.Ref, e.Count, e.Limit)
}

// CheckLimit returns a *LimitError when n entries exceed MaxEntries — for consumers that multiply
// expansions into rules (sources × destinations × services).
func CheckLimit(ref string, n int) error {
	if n > MaxEntries {
		return &LimitError{Ref: ref, Count: n, Limit: MaxEntries}
	}
	return nil
}

// FQDNLookup returns the resolved addresses of an FQDN address object by object name; false (or
// no addresses) = not resolved (yet). (*Runtime).FQDN is the agent's.
type FQDNLookup func(object string) ([]netip.Addr, bool)

// Option tunes an expansion.
type Option func(*options)

type options struct {
	fqdn  FQDNLookup
	limit int
}

// WithFQDN resolves `type: fqdn` address objects through f. Without it they expand to nothing and
// are listed in Addresses.Unresolved.
func WithFQDN(f FQDNLookup) Option { return func(o *options) { o.fqdn = f } }

// WithLimit replaces MaxEntries as the cap of one expansion (n ≤ 0 keeps MaxEntries).
func WithLimit(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.limit = n
		}
	}
}

func optionsOf(opts []Option) options {
	o := options{limit: MaxEntries}
	for _, f := range opts {
		f(&o)
	}
	return o
}

// Addresses is the expansion of an address object or group: canonical (masked), aggregated
// (covered prefixes dropped, sibling prefixes merged), sorted prefixes split by family.
type Addresses struct {
	V4 []netip.Prefix
	V6 []netip.Prefix
	// Unresolved lists the FQDN address objects (by name, sorted) that contributed nothing because
	// they have no resolved address — a warning for the consumer, never an error.
	Unresolved []string
}

// Len is the number of prefixes of both families.
func (a Addresses) Len() int { return len(a.V4) + len(a.V6) }

// All returns the IPv4 prefixes followed by the IPv6 ones.
func (a Addresses) All() []netip.Prefix {
	out := make([]netip.Prefix, 0, a.Len())
	return append(append(out, a.V4...), a.V6...)
}

// Expand turns the address object or address group ref of doc into prefixes: host → /32 or /128,
// network → its prefix, range → the minimal CIDR set, fqdn → the resolved addresses (WithFQDN;
// unresolved → nothing, listed in Unresolved), group → the union of its members, recursively. The
// result is deterministic (aggregated, sorted, v4/v6 split); an empty group expands to nothing.
// Errors: ErrUnknownObject, ErrCycle, ErrInvalid, *LimitError (more than MaxEntries prefixes).
func Expand(doc *vrxv1.ObjectsConfig, ref string, opts ...Option) (Addresses, error) {
	e := &addrExpander{doc: doc, o: optionsOf(opts), memo: map[string][]netip.Prefix{}, visiting: map[string]bool{}, unresolved: map[string]bool{}}
	ps, err := e.expand(ref)
	if err != nil {
		return Addresses{}, err
	}
	if len(ps) > e.o.limit {
		return Addresses{}, &LimitError{Ref: ref, Count: len(ps), Limit: e.o.limit}
	}
	out := Addresses{}
	for _, p := range ps {
		if p.Addr().Is4() {
			out.V4 = append(out.V4, p)
		} else {
			out.V6 = append(out.V6, p)
		}
	}
	for n := range e.unresolved {
		out.Unresolved = append(out.Unresolved, n)
	}
	sort.Strings(out.Unresolved)
	return out, nil
}

type addrExpander struct {
	doc        *vrxv1.ObjectsConfig
	o          options
	memo       map[string][]netip.Prefix
	visiting   map[string]bool
	stack      []string
	unresolved map[string]bool
}

func (e *addrExpander) expand(ref string) ([]netip.Prefix, error) {
	if a, ok := e.doc.GetAddresses()[ref]; ok {
		return e.leaf(ref, a)
	}
	g, ok := e.doc.GetAddressGroups()[ref]
	if !ok {
		return nil, fmt.Errorf("%w: %q is neither an address nor an address group", ErrUnknownObject, ref)
	}
	if m, ok := e.memo[ref]; ok {
		return m, nil
	}
	if e.visiting[ref] {
		return nil, fmt.Errorf("%w: %s", ErrCycle, cyclePath(e.stack, ref))
	}
	e.visiting[ref] = true
	e.stack = append(e.stack, ref)
	var out []netip.Prefix
	for _, m := range g.GetMembers() {
		ps, err := e.expand(m)
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	out = aggregate(out)
	if len(out) > e.o.limit {
		return nil, &LimitError{Ref: ref, Count: len(out), Limit: e.o.limit}
	}
	e.stack = e.stack[:len(e.stack)-1]
	delete(e.visiting, ref)
	e.memo[ref] = out
	return out, nil
}

func cyclePath(stack []string, ref string) string {
	for i, n := range stack {
		if n == ref {
			return strings.Join(append(append([]string{}, stack[i:]...), ref), " → ")
		}
	}
	return ref
}

func (e *addrExpander) leaf(name string, a *vrxv1.AddressObject) ([]netip.Prefix, error) {
	switch a.GetType() {
	case "host":
		addr, err := parseAddr(a.GetAddress())
		if err != nil {
			return nil, fmt.Errorf("%w: address %q: %v", ErrInvalid, name, err)
		}
		return []netip.Prefix{netip.PrefixFrom(addr, addr.BitLen())}, nil
	case "network":
		p, err := netip.ParsePrefix(a.GetPrefix())
		if err != nil {
			return nil, fmt.Errorf("%w: address %q: %v", ErrInvalid, name, err)
		}
		return []netip.Prefix{p.Masked()}, nil
	case "range":
		start, err1 := parseAddr(a.GetStart())
		end, err2 := parseAddr(a.GetEnd())
		if err := errors.Join(err1, err2); err != nil {
			return nil, fmt.Errorf("%w: address %q: %v", ErrInvalid, name, err)
		}
		ps, err := RangeToPrefixes(start, end)
		if err != nil {
			return nil, fmt.Errorf("%w: address %q: %v", ErrInvalid, name, err)
		}
		return ps, nil
	case "fqdn":
		var addrs []netip.Addr
		if e.o.fqdn != nil {
			if got, ok := e.o.fqdn(name); ok {
				addrs = got
			}
		}
		if len(addrs) == 0 {
			e.unresolved[name] = true
			return nil, nil
		}
		out := make([]netip.Prefix, 0, len(addrs))
		for _, ad := range addrs {
			if ad.IsValid() && ad.Zone() == "" {
				out = append(out, netip.PrefixFrom(ad, ad.BitLen()))
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: address %q has unknown type %q", ErrInvalid, name, a.GetType())
}

func parseAddr(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, err
	}
	if a.Zone() != "" {
		return netip.Addr{}, fmt.Errorf("%q: zones are not allowed", s)
	}
	return a, nil
}

// RangeToPrefixes returns the minimal set of CIDR prefixes that covers exactly the inclusive
// address range [start, end], in ascending order. Both ends must be of one family and start ≤ end.
func RangeToPrefixes(start, end netip.Addr) ([]netip.Prefix, error) {
	if !start.IsValid() || !end.IsValid() {
		return nil, errors.New("invalid range address")
	}
	if start.Is4() != end.Is4() {
		return nil, errors.New("range start and end must be the same address family")
	}
	if start.Compare(end) > 0 {
		return nil, errors.New("range end must not be lower than its start")
	}
	bits := start.BitLen()
	var out []netip.Prefix
	cur := start
	for {
		var p netip.Prefix
		for b := 0; b <= bits; b++ { // the first b that fits is the largest block
			cand, err := cur.Prefix(b)
			if err != nil || cand.Addr() != cur || lastAddr(cand).Compare(end) > 0 {
				continue
			}
			p = cand
			break
		}
		out = append(out, p)
		last := lastAddr(p)
		if last == end {
			return out, nil
		}
		cur = last.Next()
	}
}

// lastAddr is the highest address inside p.
func lastAddr(p netip.Prefix) netip.Addr {
	a := p.Masked().Addr()
	if a.Is4() {
		b := a.As4()
		setHostBits(b[:], p.Bits())
		return netip.AddrFrom4(b)
	}
	b := a.As16()
	setHostBits(b[:], p.Bits())
	return netip.AddrFrom16(b)
}

func setHostBits(b []byte, bits int) {
	for i := bits; i < len(b)*8; i++ {
		b[i/8] |= 1 << (7 - uint(i%8))
	}
}

// aggregate masks, deduplicates and aggregates prefixes: covered prefixes are dropped and sibling
// pairs merged into their parent, repeatedly. Output: IPv4 ascending, then IPv6 ascending.
func aggregate(ps []netip.Prefix) []netip.Prefix {
	var v4, v6 []netip.Prefix
	for _, p := range ps {
		p = p.Masked()
		if p.Addr().Is4() {
			v4 = append(v4, p)
		} else {
			v6 = append(v6, p)
		}
	}
	return append(aggregateFamily(v4), aggregateFamily(v6)...)
}

func aggregateFamily(ps []netip.Prefix) []netip.Prefix {
	sort.Slice(ps, func(i, j int) bool {
		if c := ps[i].Addr().Compare(ps[j].Addr()); c != 0 {
			return c < 0
		}
		return ps[i].Bits() < ps[j].Bits()
	})
	out := make([]netip.Prefix, 0, len(ps))
	for _, p := range ps {
		// sorted by address: only the last kept prefix can cover p (kept prefixes are disjoint)
		if n := len(out); n > 0 && out[n-1].Bits() <= p.Bits() && out[n-1].Contains(p.Addr()) {
			continue
		}
		out = append(out, p)
		for len(out) >= 2 {
			parent, ok := siblings(out[len(out)-2], out[len(out)-1])
			if !ok {
				break
			}
			out = append(out[:len(out)-2], parent)
		}
	}
	return out
}

// siblings returns the parent of a and b when they are its two halves (a the lower one).
func siblings(a, b netip.Prefix) (netip.Prefix, bool) {
	if a.Bits() != b.Bits() || a.Bits() == 0 || a.Addr() == b.Addr() {
		return netip.Prefix{}, false
	}
	pa, err1 := a.Addr().Prefix(a.Bits() - 1)
	pb, err2 := b.Addr().Prefix(b.Bits() - 1)
	if err1 != nil || err2 != nil || pa != pb {
		return netip.Prefix{}, false
	}
	return pa, true
}

// ZoneInterfaces returns the interfaces of zone (sorted), or ErrUnknownObject.
func ZoneInterfaces(doc *vrxv1.ObjectsConfig, zone string) ([]string, error) {
	z, ok := doc.GetZones()[zone]
	if !ok {
		return nil, fmt.Errorf("%w: %q is not a zone", ErrUnknownObject, zone)
	}
	out := append([]string(nil), z.GetInterfaces()...)
	sort.Strings(out)
	return out, nil
}
