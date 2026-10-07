package subsystems

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// BFD endpoint claims reuse the journaled boot-bound store, with the logical
// interface encoded in each key. Exact lookup rejects ambiguous records.
type bfdEndpointClaims struct{ store *KeyedClaims }

func bfdEndpointPrefix(local, peer string) string {
	return "endpoint/" + base64.RawURLEncoding.EncodeToString([]byte(local)) + "/" + base64.RawURLEncoding.EncodeToString([]byte(peer)) + "/"
}
func (c *bfdEndpointClaims) Persistent() bool { return true }
func (c *bfdEndpointClaims) Lookup(local, peer string) (string, bool) {
	prefix := bfdEndpointPrefix(local, peer)
	boot, ok := c.store.current()
	if !ok {
		return "", false
	}
	c.store.mu.Lock()
	defer c.store.mu.Unlock()
	c.ensureIndexLocked()
	name := ""
	found := false
	for k := range c.store.endpointIndex[prefix] {
		c.store.endpointIndexVisits++
		if c.store.recs[k].Boot != boot {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(k, prefix))
		if err != nil || found {
			return "", false
		}
		name = string(raw)
		found = true
	}
	return name, found
}
func (c *bfdEndpointClaims) Claim(local, peer, iface string) error {
	prefix := bfdEndpointPrefix(local, peer)
	boot, ok := c.store.current()
	if !ok {
		return ErrNoIdentity
	}
	c.store.mu.Lock()
	defer c.store.mu.Unlock()
	c.ensureIndexLocked()
	wanted := prefix + base64.RawURLEncoding.EncodeToString([]byte(iface))
	for key := range c.store.endpointIndex[prefix] {
		c.store.endpointIndexVisits++
		if key != wanted && c.store.recs[key].Boot == boot {
			return fmt.Errorf("BFD multihop endpoint tuple already claimed")
		}
	}
	return c.store.setLocked(wanted, &claimRecord{Key: wanted, Boot: boot})
}
func (c *bfdEndpointClaims) Release(local, peer, iface string) error {
	return c.store.Release(bfdEndpointPrefix(local, peer) + base64.RawURLEncoding.EncodeToString([]byte(iface)))
}

// endpointTuplePrefix accepts the existing journal format, including malformed
// interface suffixes so lookup still fails closed rather than hiding ambiguity.
func endpointTuplePrefix(key string) string {
	parts := strings.SplitN(key, "/", 4)
	if len(parts) != 4 || parts[0] != "endpoint" {
		return ""
	}
	return strings.Join(parts[:3], "/") + "/"
}
func (c *fileClaims) updateEndpointIndexLocked(key string, rec *claimRecord) {
	if c.endpointIndex == nil {
		return
	}
	prefix := endpointTuplePrefix(key)
	if prefix == "" {
		return
	}
	if rec == nil {
		delete(c.endpointIndex[prefix], key)
		if len(c.endpointIndex[prefix]) == 0 {
			delete(c.endpointIndex, prefix)
		}
	} else {
		if c.endpointIndex[prefix] == nil {
			c.endpointIndex[prefix] = map[string]struct{}{}
		}
		c.endpointIndex[prefix][key] = struct{}{}
	}
}
func (c *fileClaims) rebuildEndpointIndexLocked() {
	if c.endpointIndex == nil {
		return
	}
	c.endpointIndex = map[string]map[string]struct{}{}
	for key, rec := range c.recs {
		c.endpointIndexVisits++
		c.updateEndpointIndexLocked(key, &rec)
	}
}
func (c *bfdEndpointClaims) ensureIndexLocked() {
	if c.store.endpointIndex == nil {
		c.store.endpointIndex = map[string]map[string]struct{}{}
		c.store.rebuildEndpointIndexLocked()
	}
}
