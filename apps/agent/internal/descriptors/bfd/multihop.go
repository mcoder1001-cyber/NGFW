package bfd

import (
	"context"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	"ngfw/agent/internal/scheduler"
	"sync"
)

// MultihopClaims records exact endpoint tuples and their logical interface, scoped
// to the owner and VPP boot identity. Lookup must fail closed on ambiguous claims.
type MultihopClaims interface {
	Claim(local, peer, iface string) error
	Lookup(local, peer string) (string, bool)
	Release(local, peer, iface string) error
}

// MultihopEnvironment supplies persistent endpoint claims and globals-owner activation.
type MultihopEnvironment struct {
	Claims MultihopClaims
	// Lifecycle callbacks receive only successfully created/deleted or retrieved owned sessions.
	Created  func(string)
	Deleted  func(string)
	Snapshot func([]string)
	// Enable is supplied exclusively by the existing globals owner. VPP's enable
	// is one-way until restart; deleting sessions does not promise to disable it.
	Enable func(context.Context) error
}

var multihopEnvironments sync.Map

// SetMultihopEnvironment installs or removes the owner-scoped multihop dependencies.
func SetMultihopEnvironment(owner string, env *MultihopEnvironment) {
	if env == nil {
		multihopEnvironments.Delete(owner)
	} else {
		multihopEnvironments.Store(owner, *env)
	}
}
func multihopEnvironment(owner string) (MultihopEnvironment, bool) {
	v, ok := multihopEnvironments.Load(owner)
	if !ok {
		return MultihopEnvironment{}, false
	}
	return v.(MultihopEnvironment), true
}
func checkMultihopPersistent(owner string) error {
	env, ok := multihopEnvironment(owner)
	if !ok {
		return nil
	}
	return persist.Require("bfd multihop endpoint claims", env.Claims)
}

func ownedSession(owner string, ifs *df7.Interfaces, idx uint32, local, peer string) (string, bool) {
	if idx != df7.NoIndex {
		return ifs.Owned(idx, func(n string) string { return string(KeySession(n, local, peer)) })
	}
	env, ok := multihopEnvironment(owner)
	if !ok || env.Claims == nil {
		return "", false
	}
	name, ok := env.Claims.Lookup(local, peer)
	if !ok {
		return "", false
	}
	idx, err := ifs.Resolve(name)
	if err != nil {
		return "", false
	}
	return ifs.Owned(idx, func(n string) string { return string(KeySession(n, local, peer)) })
}

func sessionCreated(owner, key string) {
	if env, ok := multihopEnvironment(owner); ok && env.Created != nil {
		env.Created(key)
	}
}
func sessionDeleted(owner, key string) {
	if env, ok := multihopEnvironment(owner); ok && env.Deleted != nil {
		env.Deleted(key)
	}
}
func sessionSnapshot(owner string, kvs []scheduler.KV) {
	if env, ok := multihopEnvironment(owner); ok && env.Snapshot != nil {
		keys := make([]string, 0, len(kvs))
		for _, kv := range kvs {
			keys = append(keys, string(kv.Key))
		}
		env.Snapshot(keys)
	}
}
