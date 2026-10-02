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

// DescriptorName identifies the per-host dynamic admission scheduler family.
const DescriptorName = "base-policy.punt-interface"

// OwnedPairs returns only this agent's actual LCP pairs, with effective Netns.
type OwnedPairs func(context.Context) ([]lcp.ItfPair, error)

// Descriptor journals dynamic host admissions against owned LCP dependencies.
type Descriptor struct {
	renderer *Renderer
	config   Config
	pairs    OwnedPairs
}

// NewDescriptor binds immutable bootstrap inputs and owned actual pair readback.
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
	renderer.permanent = slices.Clone(config.Permanent)
	return &Descriptor{renderer: renderer, config: config, pairs: pairs}, nil
}

// Name implements scheduler.Descriptor.
func (*Descriptor) Name() string { return DescriptorName }

// RecordsNoOwnership declares exclusive ownership of the packaging dynamic set.
func (*Descriptor) RecordsNoOwnership() {}

// KeyOf identifies admission by host name so old names remain revocable.
func (*Descriptor) KeyOf(value proto.Message) scheduler.Key {
	var a Admission
	_ = dfkit.Decode(value, &a)
	return scheduler.Join(DescriptorName, a.Host)
}

// Dependencies orders admission after its LCP pair and effective namespace.
func (*Descriptor) Dependencies(value proto.Message) []scheduler.Dependency {
	var a Admission
	if dfkit.Decode(value, &a) != nil || a.Pair == "" {
		return nil
	}
	return []scheduler.Dependency{{Key: scheduler.Key(a.Pair)}, {Key: lcp.KeyDefaultNetns, Optional: true}}
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

// Create admits an actual owned root-namespace pair and journals uncertain adds.
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

// Update requests recreation so old admission is revoked before replacement.
func (d *Descriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete revokes one dynamic admission, including orphan kernel members.
func (d *Descriptor) Delete(ctx context.Context, value proto.Message, _ any) error {
	a, err := d.spec(value, false)
	if err != nil {
		return err
	}
	return d.renderer.Element(ctx, a.Host, false)
}

// Retrieve reports exact dynamic kernel membership and retains orphan drift.
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
			return nil, errors.New("basepolicy: dynamic set overlaps permanent admission")
		}
		a := Admission{Host: name, Pair: pairs[name]}
		// Unmatched members are orphan admissions: retain them for fail-closed
		// scheduler deletion rather than silently dropping drift from the dump.
		result = append(result, scheduler.KV{Key: scheduler.Join(DescriptorName, name), Value: dfkit.Encode(a)})
	}
	return result, nil
}

// Validate enforces the final transaction admission union before mutations.
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
