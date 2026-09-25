// Package hoststack holds the reconciler descriptors of VPP's host stack (task F-host-stack, WBS
// D7.10, scoped down by D-085): the session layer (global), application namespaces, session rules
// (the host-stack "firewall"), the TCP source-address pool and — opt-in only — the http_static
// server. Message names come only from apps/agent/binapi/{session,tcp,http_static};
// docs/agent/descriptors/hoststack.md is the object ↔ message table.
//
// What VPP lets us read back (checked against binapi 2026-09-24):
//
//	hoststack.session       no getter            write-only; globals owner sets, others require (D-071)
//	hoststack.namespace     no dump              write-only; idempotent re-add (VPP updates in place)
//	hoststack.session-rule  session_rules_v2_dump Retrieve, filtered to this owner's tag prefix
//	hoststack.tcp-src       no dump, no delete   write-only; D-076 applied-once record, irreversible per fib
//	hoststack.http-static   no dump, no disable  write-only; globals owner + VRX_HOSTSTACK_HTTP_STATIC=1 only
//
// The session layer is never disabled or re-engined by this package: Delete is a no-op and Create
// is a no-op when the layer already answers (the host VPP's layer is whatever startup.conf set,
// D-012). Everything startup.conf-only (buffers, congestion control, event queues) is the start-up
// generator's (F-startup-gen), not here.
package hoststack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// Descriptor names.
const (
	NameSession     = "hoststack.session"
	NameNamespace   = "hoststack.namespace"
	NameSessionRule = "hoststack.session-rule"
	NameTCPSrc      = "hoststack.tcp-src"
	NameHTTPStatic  = "hoststack.http-static"
)

// GlobalID is the object id of the singletons.
const GlobalID = "global"

// EnvHTTPStatic is the opt-in of the http_static descriptor (D-064/D-077 Q3: it cannot be undone).
const EnvHTTPStatic = "VRX_HOSTSTACK_HTTP_STATIC"

// WWWRoot is the only directory tree http_static may serve (D-049, DF-8 review M5).
const WWWRoot = "/var/lib/vrx/www/"

// Session is the hoststack.session singleton: the session layer must be on (rule-table engine).
type Session struct {
	Enabled bool `json:"enabled"`
}

// Namespace is one application namespace. Interface is a logical interface name ("" = none);
// Vrf is the fib id of both families.
type Namespace struct {
	ID        string `json:"id"`
	Interface string `json:"interface"`
	Vrf       uint32 `json:"vrf"`
}

// Rule is one session rule. Ports 0 = any; AppNamespace "" = the default namespace (index 0).
type Rule struct {
	Tag              string `json:"tag"`
	Scope            string `json:"scope"`     // global | local
	Transport        string `json:"transport"` // tcp | udp
	Local            string `json:"local"`
	LocalPort        uint32 `json:"localPort"`
	Remote           string `json:"remote"`
	RemotePort       uint32 `json:"remotePort"`
	Action           string `json:"action"` // allow | deny | redirect
	RedirectAppIndex uint32 `json:"redirectAppIndex"`
	AppNamespace     string `json:"appNamespace"`
}

// TCPSrc is the TCP source-address pool of one fib.
type TCPSrc struct {
	First string `json:"first"`
	Last  string `json:"last"`
	Vrf   uint32 `json:"vrf"`
}

// HTTPStatic is the http_static server.
type HTTPStatic struct {
	WWWRoot     string `json:"wwwRoot"`
	URI         string `json:"uri"`
	CacheSizeMB uint32 `json:"cacheSizeMb"`
}

// Proto returns the canonical structpb document.
func (s Session) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s Namespace) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s Rule) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s TCPSrc) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Proto returns the canonical structpb document.
func (s HTTPStatic) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Keys.
var (
	KeySession    = scheduler.Join(NameSession, GlobalID)
	KeyHTTPStatic = scheduler.Join(NameHTTPStatic, GlobalID)
)

// KeyNamespace is the key of namespace id.
func KeyNamespace(id string) scheduler.Key { return scheduler.Join(NameNamespace, id) }

// KeyRule is the key of the rule with tag.
func KeyRule(tag string) scheduler.Key { return scheduler.Join(NameSessionRule, tag) }

// KeyTCPSrc is the key of the pool of fib vrf.
func KeyTCPSrc(vrf uint32) scheduler.Key { return scheduler.Join(NameTCPSrc, fmt.Sprint(vrf)) }

// ValidID checks a namespace id or rule tag (D-049): 1..40 of [A-Za-z0-9_.-], no "..".
func ValidID(s string) error {
	if s == "" || len(s) > 40 {
		return dfkit.Specf("id %q: length must be 1..40", s)
	}
	if strings.Contains(s, "..") {
		return dfkit.Specf("id %q: must not contain ..", s)
	}
	for _, r := range s {
		if !(r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return dfkit.Specf("id %q: invalid character %q", s, r)
		}
	}
	return nil
}

// ValidWWWRoot checks an http_static web root (D-049, review M5).
func ValidWWWRoot(p string) error {
	if !strings.HasPrefix(p, WWWRoot) || len(p) <= len(WWWRoot) || len(p) > 255 {
		return dfkit.Specf("www root %q must be a path under %s", p, WWWRoot)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return dfkit.Specf("www root %q must not contain ..", p)
		}
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return dfkit.Specf("www root %q contains control characters", p)
		}
	}
	return nil
}

// ---- options, registration --------------------------------------------------------------------

type config struct {
	boot    dfkit.BootStore
	globals dfkit.Globals
}

// Option configures Register / RegisterGlobals.
type Option func(*config)

// WithBootStore passes the agent's persisted D-076 store (Wiring.BootStore()); the default is an
// in-memory store (tests only: an agent restart would re-add the TCP source pool once more).
func WithBootStore(s dfkit.BootStore) Option { return func(c *config) { c.boot = s } }

// WithGlobalsOwner records the D-071 role: Register then leaves hoststack.session to
// RegisterGlobals (owner) instead of registering its require-only form.
func WithGlobalsOwner(owner bool) Option {
	return func(c *config) { c.globals = dfkit.GlobalsOwner(owner) }
}

func newConfig(opts []Option) *config {
	c := &config{}
	for _, o := range opts {
		o(c)
	}
	if c.boot == nil {
		c.boot = dfkit.NewMemoryBootStore()
	}
	return c
}

// Register registers the per-owner descriptors: namespaces, session rules, the TCP source pool
// and — for an agent that is not the globals owner — the require-only hoststack.session (it never
// sets the session layer, D-071). Namespaces are registered before rules (tie-break order).
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...Option) {
	c := newConfig(opts)
	st := stateFor(owner, client, c.boot)
	if !c.globals.Owner() {
		r.Register(NewSession(client, dfkit.GlobalsOwner(false)))
	}
	r.Register(newNamespace(client, owner, st))
	r.Register(newRule(client, owner, st))
	r.Register(NewTCPSrc(client, c.boot))
}

// RegisterGlobals registers the VPP-global descriptors as the globals owner (D-071): the session
// layer and, only with VRX_HOSTSTACK_HTTP_STATIC=1 (D-064: http_static cannot be disabled), the
// http_static server. Call it only in the globals owner's agent, together with Register(…,
// WithGlobalsOwner(true)).
func RegisterGlobals(r scheduler.Registry, client vpp.Client, opts ...Option) {
	c := newConfig(opts)
	r.Register(NewSession(client, dfkit.GlobalsOwner(true)))
	if os.Getenv(EnvHTTPStatic) == "1" {
		r.Register(NewHTTPStatic(client, c.boot))
	}
}

// ---- per-owner state: namespace indexes -------------------------------------------------------

// nsIndexKey is the BootStore key of an owner's namespace id → appns_index map (bound to the VPP
// boot identity: indexes are meaningless after a VPP restart).
func nsIndexKey(owner string) string { return "hoststack.namespace-index/" + owner }

// state is what the namespace and rule descriptors (and HostStackState) share for one owner.
type state struct {
	mu     sync.Mutex
	client vpp.Client
	boot   dfkit.BootStore
	owner  string
}

var (
	statesMu sync.Mutex
	states   = map[string]*state{}
)

func stateFor(owner string, c vpp.Client, boot dfkit.BootStore) *state {
	statesMu.Lock()
	defer statesMu.Unlock()
	s := &state{client: c, boot: boot, owner: owner}
	states[owner] = s
	return s
}

func lookupState(owner string) (*state, bool) {
	statesMu.Lock()
	defer statesMu.Unlock()
	s, ok := states[owner]
	return s, ok
}

// indexes returns the recorded namespace indexes of the VPP instance cur (empty when the record
// belongs to another instance).
func (s *state) indexes(cur bootid.Identity) map[string]uint32 {
	out := map[string]uint32{}
	r, ok := s.boot.Get(nsIndexKey(s.owner))
	if !ok || !bootid.Matches(r.Identity, cur) {
		return out
	}
	_ = json.Unmarshal([]byte(r.Value), &out)
	return out
}

// currentIndexes is indexes for the running VPP instance.
func (s *state) currentIndexes(ctx context.Context) (map[string]uint32, error) {
	id, err := dfkit.IdentitySource(ctx, s.client)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.indexes(id), nil
}

func (s *state) setIndex(ctx context.Context, id string, idx uint32, del bool) error {
	cur, err := dfkit.IdentitySource(ctx, s.client)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.indexes(cur)
	if del {
		delete(m, id)
	} else {
		m[id] = idx
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.boot.Put(dfkit.BootRecord{Key: nsIndexKey(s.owner), Identity: cur.String(), Value: string(raw)})
}

func sortedKeys(m map[string]uint32) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
