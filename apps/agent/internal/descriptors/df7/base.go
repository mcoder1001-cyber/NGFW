package df7

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Base carries what every DF-7 descriptor holds: its name, the shared client, the owner and
// the plugin options.
type Base struct {
	name   string
	Client vpp.Client
	Owner  string
	Opts   Options
}

// NewBase returns a Base.
func NewBase(name string, c vpp.Client, owner string, opts []Option) Base {
	return Base{name: name, Client: c, Owner: owner, Opts: BuildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (b Base) Name() string { return b.name }

// Key builds "<name>/<id parts>".
func (b Base) Key(id ...string) scheduler.Key { return scheduler.Join(b.name, id...) }

// Ifaces takes an interface snapshot with this descriptor's owner and options.
func (b Base) Ifaces(ctx context.Context) (*Interfaces, error) {
	return DumpInterfaces(ctx, b.Client, b.Owner, b.Opts)
}

// Wrap prefixes err with the descriptor name and the VPP message.
func (b Base) Wrap(msg string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s: %w", b.name, msg, err)
}

// Spec is implemented by every typed desired-state spec of this factory.
type Spec interface {
	Validate() error
}

// DecodeValid decodes a Value into T and validates it.
func DecodeValid[T Spec](msg proto.Message) (T, error) {
	v, err := Decode[T](msg)
	if err != nil {
		return v, err
	}
	if err := v.Validate(); err != nil {
		return v, err
	}
	return v, nil
}

// KV builds a scheduler.KV from a key, a spec and meta.
func KV(key scheduler.Key, spec any, meta any) scheduler.KV {
	return scheduler.KV{Key: key, Value: Encode(spec), Meta: meta}
}
