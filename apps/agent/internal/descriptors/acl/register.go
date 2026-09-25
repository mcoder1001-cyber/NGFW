package acl

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// InterfaceKeyFunc maps a VPP interface name to the scheduler key of the object that creates it
// (DF-1's interface descriptors). The binding descriptors declare it as an optional dependency so
// bindings are applied after the interface exists and removed before it goes away.
type InterfaceKeyFunc func(ifName string) scheduler.Key

// DefaultInterfaceKey is the interface key scheme assumed until DF-1 is merged: "interface/<name>".
// Wire the real one with WithInterfaceKey.
func DefaultInterfaceKey(ifName string) scheduler.Key { return scheduler.Join("interface", ifName) }

// Option configures Register and the binding descriptor constructors.
type Option func(*options)

type options struct {
	ifaceKey InterfaceKeyFunc
	claims   ClaimStore
}

func buildOptions(opts []Option) options {
	o := options{ifaceKey: DefaultInterfaceKey}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithInterfaceKey sets the interface key scheme used in Dependencies of the binding descriptors.
func WithInterfaceKey(f InterfaceKeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.ifaceKey = f
		}
	}
}

// WithEtypeClaims sets the store in which acl.etype-whitelist records the untagged interfaces
// it has applied a whitelist to (see ClaimStore). Default: a fresh in-memory store per
// descriptor, which forgets its claims when the agent restarts.
func WithEtypeClaims(s ClaimStore) Option {
	return func(o *options) {
		if s != nil {
			o.claims = s
		}
	}
}

// interfaceDependency is the optional dependency of a binding on its interface object.
func (o options) interfaceDependency(ifName string) scheduler.Dependency {
	return scheduler.Dependency{Key: o.ifaceKey(ifName), Optional: true}
}

// Register constructs every descriptor of the acl plugin with the shared client and owner and
// registers them, in dependency-friendly order (ACLs before the bindings that reference them).
// This is the one entry point the agent (P05) wires.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, client vpp.Client, owner string, opts ...Option) {
	r.Register(NewACL(client, owner))
	r.Register(NewMacipACL(client, owner))
	r.Register(NewInterfaceBinding(client, owner, opts...))
	r.Register(NewEtypeWhitelist(client, owner, opts...))
	r.Register(NewMacipBinding(client, owner, opts...))
	r.Register(NewStatsEnable(client))
}
