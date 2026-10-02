package basepolicy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/scheduler"
)

const DescriptorName = "base-policy.punt-interface"

// OwnedPairs returns only this agent's actual LCP pairs, with effective Netns.
type OwnedPairs func(context.Context) ([]lcp.ItfPair, error)
type Descriptor struct {
	renderer *Renderer
	config   Config
	pairs    OwnedPairs
}

func NewDescriptor(renderer *Renderer, config Config, pairs OwnedPairs) (*Descriptor, error) {
	if renderer == nil || pairs == nil {
		return nil, errors.New("basepolicy: missing descriptor dependency")
	}
	if _, err := Members(config.Management, config.Permanent, nil); err != nil {
		return nil, err
	}
	if renderer.management != config.Management {
		return nil, errors.New("basepolicy: management identity mismatch")
	}
	config.Permanent = slices.Clone(config.Permanent)
	return &Descriptor{renderer: renderer, config: config, pairs: pairs}, nil
}
func (*Descriptor) Name() string        { return DescriptorName }
func (*Descriptor) RecordsNoOwnership() {}
func (*Descriptor) KeyOf(value proto.Message) scheduler.Key {
	var a Admission
	_ = dfkit.Decode(value, &a)
	return scheduler.Join(DescriptorName, a.Host)
}
func (*Descriptor) Dependencies(value proto.Message) []scheduler.Dependency {
	var a Admission
	if dfkit.Decode(value, &a) != nil || a.Pair == "" {
		return nil
	}
	return []scheduler.Dependency{{Key: scheduler.Key(a.Pair)}}
}
func (d *Descriptor) spec(value proto.Message, requiredPair bool) (Admission, error) {
	var a Admission
	if err := dfkit.Decode(value, &a); err != nil {
		return a, err
	}
	if _, err := Members(d.config.Management, nil, []string{a.Host}); err != nil {
		return a, err
	}
	if slices.Contains(d.config.Permanent, a.Host) {
		return a, errors.New("basepolicy: permanent admission cannot be dynamically owned")
	}
	if requiredPair && (!strings.HasPrefix(a.Pair, lcp.NameItfPair+"/") || strings.TrimPrefix(a.Pair, lcp.NameItfPair+"/") == "") {
		return a, errors.New("basepolicy: missing LCP pair identity")
	}
	return a, nil
}
func (d *Descriptor) actual(ctx context.Context) (map[string]string, error) {
	pairs, err := d.pairs(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, pair := range pairs {
		if err := pair.Validate(); err != nil {
			return nil, err
		}
		if pair.Netns != "" {
			continue
		}
		if _, ok := result[pair.HostIfName]; ok {
			return nil, fmt.Errorf("basepolicy: ambiguous owned root host %q", pair.HostIfName)
		}
		result[pair.HostIfName] = string(scheduler.Join(lcp.NameItfPair, pair.Interface))
	}
	return result, nil
}
func (d *Descriptor) Create(ctx context.Context, value proto.Message) (any, error) {
	a, err := d.spec(value, true)
	if err != nil {
		return nil, err
	}
	pairs, err := d.actual(ctx)
	if err != nil {
		return nil, err
	}
	if pairs[a.Host] != a.Pair {
		return nil, errors.New("basepolicy: admission does not match an owned root LCP pair")
	}
	err = d.renderer.Element(ctx, a.Host, true)
	if errors.Is(err, scheduler.ErrUncertainOutcome) {
		return nil, scheduler.PartialCreate(err)
	}
	return nil, err
}
func (d *Descriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}
func (d *Descriptor) Delete(ctx context.Context, value proto.Message, _ any) error {
	a, err := d.spec(value, false)
	if err != nil {
		return err
	}
	return d.renderer.Element(ctx, a.Host, false)
}
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	names, err := d.renderer.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	pairs, err := d.actual(ctx)
	if err != nil {
		return nil, err
	}
	var result []scheduler.KV
	for _, name := range names {
		if slices.Contains(d.config.Permanent, name) {
			continue
		}
		a := Admission{Host: name, Pair: pairs[name]}
		// Unmatched members are orphan admissions: retain them for fail-closed
		// scheduler deletion rather than silently dropping drift from the dump.
		result = append(result, scheduler.KV{Key: scheduler.Join(DescriptorName, name), Value: dfkit.Encode(a)})
	}
	return result, nil
}
func (d *Descriptor) Validate(_ context.Context, _ scheduler.Key, value proto.Message, view scheduler.ReadOnlyView) error {
	if _, err := d.spec(value, true); err != nil {
		return err
	}
	var names []string
	for _, kv := range view.List(DescriptorName) {
		a, err := d.spec(kv.Value, true)
		if err != nil {
			return err
		}
		names = append(names, a.Host)
	}
	_, err := Members(d.config.Management, d.config.Permanent, names)
	return err
}

var _ scheduler.Descriptor = (*Descriptor)(nil)
var _ scheduler.Validator = (*Descriptor)(nil)
