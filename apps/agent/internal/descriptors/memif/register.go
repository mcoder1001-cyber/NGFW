package memif

import (
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Option configures Register.
type Option func(*options)

type options struct{ socketDir string }

// WithSocketDir overrides the owner's memif socket directory (default DefaultSocketDir(owner)).
func WithSocketDir(dir string) Option { return func(o *options) { o.socketDir = dir } }

// Register registers the memif descriptors with r.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...Option) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	r.Register(NewSocket(c, owner, o.socketDir))
	r.Register(NewMemif(c, owner))
}
