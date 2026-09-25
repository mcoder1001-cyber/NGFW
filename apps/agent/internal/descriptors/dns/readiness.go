package dns

import (
	"context"
	"errors"
	"net/netip"
	"sync"

	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// Readiness is DF-8's live fact behind D-137: on the VPP instance that runs NOW, this agent (the globals owner) added at
// least one IPv4 name server and then dns_enable_disable(1) succeeded. It is never derived from the stored document:
// the facts are recorded only when VPP accepted the messages, they belong to the VPP boot identity (kernel boot_id, VPP
// main PID and start time — internal/vpp/bootid) observed when they were recorded, and Ready compares that identity
// with the running instance on every call. A VPP restart or crash, a disconnect to another instance, a disable and
// a server delete all clear the fact until the resync applies the servers and the switch again.
//
// Why IPv4: VPP 26.06 (`plugins/dns/dns.c:576-624`) starts every request with the IPv4 family and dereferences
// `ip4_name_servers + rotor`, which is NULL until an IPv4 server was added since VPP started — for an API lookup
// (dns_resolve_name / dns_resolve_ip) and for any IPv4 UDP-53 request to a VPP address while enabled. IPv6-only
// servers crash it the same way (docs/vpp-code-track.md).
type Readiness struct {
	client   vpp.Client
	identity func(context.Context, vpp.Client) (string, error)

	mu      sync.Mutex
	boot    string // identity of the VPP instance the facts below belong to ("" = none recorded)
	v4      map[netip.Addr]bool
	enabled bool
}

// ErrIdentityUnknown means the running VPP instance could not be identified completely: nothing is ready then.
var ErrIdentityUnknown = errors.New("dns: VPP boot identity incomplete (boot_id, VPP PID or start time unreadable)")

// NewReadiness returns the readiness fact of the VPP behind client.
func NewReadiness(client vpp.Client) *Readiness {
	return &Readiness{client: client, identity: currentIdentity, v4: map[netip.Addr]bool{}}
}

func currentIdentity(ctx context.Context, c vpp.Client) (string, error) {
	id, err := bootid.Current(ctx, c)
	if err != nil {
		return "", err
	}
	if !id.Complete() || id.PID == 0 {
		return "", ErrIdentityUnknown
	}
	return id.String(), nil
}

// WithIdentity replaces the VPP instance identity source (tests: a simulated VPP restart).
func (r *Readiness) WithIdentity(f func(context.Context, vpp.Client) (string, error)) *Readiness {
	r.identity = f
	return r
}

// current returns the running instance's identity; facts of another instance are dropped.
func (r *Readiness) current(ctx context.Context) (string, bool) {
	id, err := r.identity(ctx, r.client)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil || id == "" {
		r.boot, r.enabled, r.v4 = "", false, map[netip.Addr]bool{}
		return "", false
	}
	if id != r.boot {
		r.boot, r.enabled, r.v4 = id, false, map[netip.Addr]bool{}
	}
	return id, true
}

// serverAdded records an IPv4 server VPP accepted (IPv6 servers never make the resolver usable, see the type doc).
func (r *Readiness) serverAdded(ctx context.Context, a netip.Addr) {
	if r == nil || !a.Is4() {
		return
	}
	if _, ok := r.current(ctx); !ok {
		return
	}
	r.mu.Lock()
	r.v4[a] = true
	r.mu.Unlock()
}

// serverRemoved forgets a deleted server (the caller disabled the switch first).
func (r *Readiness) serverRemoved(a netip.Addr) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.v4, a)
	r.mu.Unlock()
}

// hasIPv4 reports whether an IPv4 server was added on the running instance (the precondition of an enable).
func (r *Readiness) hasIPv4(ctx context.Context) bool {
	if r == nil {
		return false
	}
	if _, ok := r.current(ctx); !ok {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.v4) > 0
}

// setEnabled records a successful dns_enable_disable.
func (r *Readiness) setEnabled(ctx context.Context, on bool) {
	if r == nil {
		return
	}
	if !on {
		r.mu.Lock()
		r.enabled = false
		r.mu.Unlock()
		return
	}
	if _, ok := r.current(ctx); !ok {
		return
	}
	r.mu.Lock()
	r.enabled = true
	r.mu.Unlock()
}

// Ready reports the fact (see the type doc): the running VPP instance is the one the facts were recorded on, the
// resolver was enabled there and at least one IPv4 server was added. A nil Readiness (an agent that is not the
// globals owner) is never ready. It costs one control_ping.
func (r *Readiness) Ready(ctx context.Context) bool {
	if r == nil {
		return false
	}
	if _, ok := r.current(ctx); !ok {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled && len(r.v4) > 0
}
