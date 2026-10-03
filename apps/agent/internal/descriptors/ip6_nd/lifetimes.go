package ip6nd

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/internal/scheduler"
)

// Each expiry is bracketed by the request/response times. Unlike advertised integer
// lifetimes, these absolute deadlines remain fixed while VPP counts down.
type expiryWindow struct{ Earliest, Latest float64 }
type lifetimeRecord struct {
	Index                        uint32
	Valid, Preferred             uint32
	ValidExpiry, PreferredExpiry expiryWindow
}

type LifetimeStore struct {
	mu      sync.Mutex
	path    string
	records map[string]lifetimeRecord
}

// OpenLifetimeStore loads the configured timers and their observed VPP expiry windows.
// It contains no secrets and does not establish ownership (DF-2 claims still do that).
func OpenLifetimeStore(path string) (*LifetimeStore, error) {
	s := &LifetimeStore{path: path, records: map[string]lifetimeRecord{}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &s.records); err != nil {
		return nil, err
	}
	if s.records == nil {
		s.records = map[string]lifetimeRecord{}
	}
	return s, nil
}

func (s *LifetimeStore) save(key scheduler.Key, record *lifetimeRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]lifetimeRecord, len(s.records)+1)
	for k, v := range s.records {
		next[k] = v
	}
	if record == nil {
		delete(next, string(key))
	} else {
		next[string(key)] = *record
	}
	if s.path != "" {
		b, err := json.Marshal(next)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(s.path), ".ra-lifetimes-")
		if err != nil {
			return err
		}
		tmp := f.Name()
		defer os.Remove(tmp)
		if _, err = f.Write(b); err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if err = os.Rename(tmp, s.path); err != nil {
			return err
		}
	}
	s.records = next
	return nil
}

func expiry(seconds float64, start, end time.Time) expiryWindow {
	return expiryWindow{float64(start.UnixNano())/1e9 + seconds, float64(end.UnixNano())/1e9 + seconds}
}
func sameExpiry(a, b expiryWindow) bool {
	// Only floating-point timestamp rounding slack; elapsed seconds are in VPP's
	// fractional expiry, not a tolerance that can hide a changed integer lifetime.
	const slack = 0.001
	return a.Earliest <= b.Latest+slack && b.Earliest <= a.Latest+slack
}
func matchesLifetime(configured, advertised uint32, saved expiryWindow, remaining float64, start, end time.Time) bool {
	if configured == math.MaxUint32 {
		return advertised == math.MaxUint32
	}
	if advertised > configured || math.IsNaN(remaining) || math.IsInf(remaining, 0) {
		return false
	}
	return sameExpiry(saved, expiry(remaining, start, end))
}
func initialExpiryMatches(configured, advertised uint32, remaining float64, setStart, setEnd, start, end time.Time) bool {
	if configured == math.MaxUint32 {
		return advertised == math.MaxUint32
	}
	return matchesLifetime(configured, advertised, expiry(float64(configured), setStart, setEnd), remaining, start, end)
}

func (s *LifetimeStore) restore(key scheduler.Key, idx uint32, p ip6_nd.IP6ndRaPrefix, v *RaPrefix, start, end time.Time) {
	if !p.DecrementLifetimeFlag {
		return
	}
	s.mu.Lock()
	record, ok := s.records[string(key)]
	s.mu.Unlock()
	if !ok || record.Index != idx {
		return
	}
	if matchesLifetime(record.Valid, p.ValLifetime, record.ValidExpiry, p.ValidLifetimeExpires, start, end) &&
		matchesLifetime(record.Preferred, p.PrefLifetime, record.PreferredExpiry, p.PrefLifetimeExpires, start, end) {
		v.ValidLifetime = record.Valid
		v.PreferredLifetime = record.Preferred
	}
}
