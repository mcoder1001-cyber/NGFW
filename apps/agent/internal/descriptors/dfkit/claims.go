package dfkit

import "sync"

// ClaimStore records which descriptor configured something on an UNTAGGED interface (a physical
// or pre-existing NIC). Untagged interfaces carry no owner, so Retrieve reports an object on one
// only while its claim exists (D-071 claim rule; the DF-4/DF-1 ClaimStore pattern). Create
// claims, Delete releases. The default store is in memory per owner and forgets its claims when
// the agent restarts (the objects are then re-created once by the scheduler, which every DF-8
// Create tolerates); P05 installs a persistent store with SetClaimStore.
type ClaimStore interface {
	Claim(iface, holder string) error
	Release(iface, holder string) error
	Claimed(iface, holder string) bool
}

type memClaims struct {
	mu sync.Mutex
	m  map[[2]string]struct{}
}

// NewMemoryClaimStore returns an in-memory ClaimStore.
func NewMemoryClaimStore() ClaimStore { return &memClaims{m: map[[2]string]struct{}{}} }

func (s *memClaims) Claim(iface, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[[2]string{iface, holder}] = struct{}{}
	return nil
}

func (s *memClaims) Release(iface, holder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, [2]string{iface, holder})
	return nil
}

func (s *memClaims) Claimed(iface, holder string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[[2]string{iface, holder}]
	return ok
}

var (
	claimsMu sync.Mutex
	claims   = map[string]ClaimStore{}
)

// SetClaimStore installs the ClaimStore of an owner (P05: a persistent one in the state dir).
func SetClaimStore(owner string, s ClaimStore) {
	claimsMu.Lock()
	defer claimsMu.Unlock()
	claims[owner] = s
}

// Claims returns the owner's ClaimStore (an in-memory one unless SetClaimStore installed another).
func Claims(owner string) ClaimStore {
	claimsMu.Lock()
	defer claimsMu.Unlock()
	s, ok := claims[owner]
	if !ok {
		s = NewMemoryClaimStore()
		claims[owner] = s
	}
	return s
}
