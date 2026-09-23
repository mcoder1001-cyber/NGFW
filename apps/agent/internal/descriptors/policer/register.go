package policer

import (
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Register constructs every descriptor of the policer plugin and registers them in
// dependency-friendly order (policers before their attachments).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewPolicer(c, owner, opts...))
	r.Register(NewInterface(c, owner, opts...))
	r.Register(NewBind(c, owner, opts...))
	r.Register(NewClassify(c, owner, opts...))
}
