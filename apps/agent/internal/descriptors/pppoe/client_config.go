package pppoe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/scheduler"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ClientConfigName is the singleton daemon configuration descriptor.
const ClientConfigName = "pppoe.client.config"

// ClientConfigKey identifies the owner client configuration.
var ClientConfigKey = scheduler.Join(ClientConfigName, "ngfw")

// ClientRuntime applies resolved sessions without returning credentials.
type ClientRuntime interface {
	Apply(context.Context, []ren.Session) error
}

// ClientConfig resolves credentials only at the final renderer boundary.
type ClientConfig struct {
	mu           sync.RWMutex
	secrets      func(string) ([]byte, error)
	runtime      ClientRuntime
	renderer     *ren.Renderer
	manifest     string
	carrierOwner string
	// Check is the structural checker; pppd has no offline configuration checker.
	Check func(context.Context, *ren.Renderer, []ren.Session) error
}

// NewClientConfig connects a renderer to its owning runtime and recovery manifest.
func NewClientConfig(runtime ClientRuntime, r *ren.Renderer, manifest string) *ClientConfig {
	return &ClientConfig{runtime: runtime, renderer: r, manifest: manifest}
}

// SetCarrierOwner selects the product-only isolated kernel carrier contract.
// Construction-time only, before registering the descriptor.
func (d *ClientConfig) SetCarrierOwner(owner string) { d.carrierOwner = owner }

// SetSecretSource installs the owner-scoped sealed secret cache lookup (secretchannel.Store.Text), which resolves a
// literal "password/<name>" reference in the transaction-selected snapshot, as FRR and PKI do. The cache's Resolve
// matches keyed fingerprints only and never resolves a config reference.
func (d *ClientConfig) SetSecretSource(source func(string) ([]byte, error)) {
	d.mu.Lock()
	d.secrets = source
	d.mu.Unlock()
}

// Name implements scheduler.Descriptor.
func (d *ClientConfig) Name() string { return ClientConfigName }

// KeyOf implements scheduler.Descriptor.
func (d *ClientConfig) KeyOf(proto.Message) scheduler.Key { return ClientConfigKey }

// Stage orders supervision after VPP dependencies.
func (d *ClientConfig) Stage() scheduler.Stage { return scheduler.StageDaemon }

// Dependencies orders parent taps and negotiated interfaces before dialing.
func (d *ClientConfig) Dependencies(value proto.Message) []scheduler.Dependency {
	doc, _ := value.(*ngfwv1.DesiredState)
	var out []scheduler.Dependency
	seen := map[scheduler.Key]bool{}
	for name, itf := range doc.GetInterfaces() {
		if itf.Pppoe == nil {
			continue
		}
		keys := []scheduler.Key{iface.AliasKey(name), scheduler.Join(lcp.NameItfPair, itf.Pppoe.GetParent())}
		if d.carrierOwner != "" {
			mtu := itf.Pppoe.GetMtu()
			if mtu == 0 {
				mtu = 1492
			}
			spec, err := ren.NewCarrierSpec(d.carrierOwner, name, itf.Pppoe.GetParent(), mtu)
			if err != nil {
				continue
			}
			keys = []scheduler.Key{iface.AliasKey(name), CarrierNamespaceKey(spec.Token()), l2.XconnectKey(string(iface.AliasKey(spec.Parent))), l2.XconnectKey(string(iface.AliasKey(spec.RawLogical())))}
		}
		for _, key := range keys {
			if !seen[key] {
				out = append(out, scheduler.Dependency{Key: key})
				seen[key] = true
			}
		}
	}
	return out
}
func (d *ClientConfig) sessions(_ context.Context, value proto.Message, strict bool) ([]ren.Session, *rfkit.Redactor, error) {
	doc, ok := value.(*ngfwv1.DesiredState)
	if !ok {
		return nil, nil, errors.New("invalid PPPoE client configuration")
	}
	d.mu.RLock()
	secrets := d.secrets
	d.mu.RUnlock()
	redactor := &rfkit.Redactor{}
	var out []ren.Session
	names := make([]string, 0, len(doc.Interfaces))
	for name := range doc.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := doc.Interfaces[name].GetPppoe()
		if c == nil || (c.Enabled != nil && !c.GetEnabled()) {
			continue
		}
		if err := rfkit.CheckRef(c.GetPasswordRef(), "password"); err != nil {
			return nil, redactor, scheduler.InvalidAt(passwordPointer(name), errors.New("PPPoE requires a password reference"))
		}
		var carrier *ren.CarrierSpec
		var host string
		var err error
		if d.carrierOwner != "" {
			mtu := c.GetMtu()
			if mtu == 0 {
				mtu = 1492
			}
			spec, e := ren.NewCarrierSpec(d.carrierOwner, name, c.GetParent(), mtu)
			if e != nil {
				return nil, redactor, scheduler.InvalidAt("/interfaces/"+stringReplace(name)+"/pppoe/parent", e)
			}
			carrier, host = &spec, spec.RawHost()
		} else {
			host, err = lcpmap.HostName(c.GetParent(), doc.Interfaces[c.GetParent()].GetLcp())
		}
		if err != nil {
			return nil, redactor, err
		}
		password := ""
		var material []byte
		if secrets != nil {
			material, err = secrets(c.GetPasswordRef())
			if err == nil {
				password = string(material)
				redactor.Add(password)
			}
			clear(material)
		}
		if err != nil || password == "" {
			if strict {
				return nil, redactor, scheduler.InvalidAt(passwordPointer(name), errors.New("PPPoE password reference is unavailable"))
			}
			password = "validation-placeholder"
		}
		mtu := c.GetMtu()
		if c.Mtu == nil {
			mtu = 1492
		}
		holdoff := uint32(5)
		if c.GetReconnect() != nil && c.GetReconnect().HoldoffSec != nil {
			holdoff = c.GetReconnect().GetHoldoffSec()
		}
		out = append(out, ren.Session{Carrier: carrier, Iface: name, HostIf: host, Username: c.GetUsername(), Password: password, ServiceName: c.GetServiceName(), MTU: mtu, MSSClamp: c.MssClamp == nil || c.GetMssClamp(), DefaultRoute: c.DefaultRoute == nil || c.GetDefaultRoute(), DNSFromPeer: c.GetDnsFromPeer(), IPv6: c.GetIpv6(), HoldoffSec: holdoff, MaxFail: c.GetReconnect().GetMaxFail()})
	}
	return out, redactor, nil
}
func passwordPointer(name string) string {
	return "/interfaces/" + stringReplace(name) + "/pppoe/passwordRef"
}
func stringReplace(s string) string { // RFC 6901
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '~':
			b.WriteString("~0")
		case '/':
			b.WriteString("~1")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Validate stages a read-only structural pppd check before product writes.
func (d *ClientConfig) Validate(ctx context.Context, _ scheduler.Key, value proto.Message, _ scheduler.ReadOnlyView) error {
	sessions, redactor, err := d.sessions(ctx, value, false)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "ngfw-pppoe-validate-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	staged := ren.New(ren.WithPaths(ren.PathsUnder(dir)))
	files, err := staged.Render(sessions)
	if err == nil {
		for path := range files {
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = renderers.WriteFiles(files)
	}
	if err == nil {
		if d.Check != nil {
			err = d.Check(ctx, staged, sessions)
		} else {
			err = staged.Validate(sessions)
		}
	}
	if err != nil {
		return redactor.Error(err)
	}
	return ctx.Err()
}

// Create resolves references and applies the owner session files.
func (d *ClientConfig) Create(ctx context.Context, value proto.Message) (any, error) {
	sessions, redactor, err := d.sessions(ctx, value, true)
	if err != nil {
		return nil, err
	}
	if err = d.renderer.Validate(sessions); err != nil {
		return nil, redactor.Error(err)
	}
	if err = d.runtime.Apply(ctx, sessions); err != nil {
		return nil, redactor.Error(err)
	}
	b, err := proto.Marshal(value)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(d.manifest), 0700); err != nil {
		return nil, err
	}
	err = renderers.WriteFiles(renderers.Files{d.manifest: {Mode: 0600, Content: b}})
	return nil, err
}

// Update replaces the desired owner sessions.
func (d *ClientConfig) Update(ctx context.Context, _, next proto.Message, _ any) (any, error) {
	return d.Create(ctx, next)
}

// Delete withdraws runtime effects and removes owner session files.
func (d *ClientConfig) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if err := d.runtime.Apply(ctx, nil); err != nil {
		return err
	}
	err := os.Remove(d.manifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Retrieve compares actual files against the reference-only applied manifest.
func (d *ClientConfig) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(d.manifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	doc := &ngfwv1.DesiredState{}
	if err = proto.Unmarshal(b, doc); err != nil {
		return nil, fmt.Errorf("PPPoE applied manifest is invalid")
	}
	sessions, redactor, err := d.sessions(ctx, doc, true)
	if err != nil {
		return nil, err
	}
	var files renderers.Files
	if d.carrierOwner != "" {
		recovery, ok := d.runtime.(interface {
			CarrierFiles([]ren.Session) (renderers.Files, error)
			Remember([]ren.Session)
		})
		if !ok {
			return nil, errors.New("carrier recovery readback is unavailable")
		}
		// Retain the last committed sessions even on file drift so Apply can
		// stop their fixed units before replacing any credential or hook.
		recovery.Remember(sessions)
		files, err = recovery.CarrierFiles(sessions)
	} else {
		files, err = d.renderer.Render(sessions)
	}
	if err != nil {
		return nil, redactor.Error(err)
	}
	for path, f := range files {
		actual, e := os.ReadFile(path) //nolint:gosec // fixed paths generated by this renderer
		if e != nil || !bytes.Equal(actual, f.Content) {
			drift, _ := structpb.NewStruct(map[string]any{"drift": "PPPoE rendered files differ from the applied manifest"})
			return []scheduler.KV{{Key: ClientConfigKey, Value: drift}}, nil
		}
	}
	if remember, ok := d.runtime.(interface{ Remember([]ren.Session) }); ok {
		remember.Remember(sessions)
	}
	return []scheduler.KV{{Key: ClientConfigKey, Value: doc}}, nil
}

var _ scheduler.Descriptor = (*ClientConfig)(nil)
var _ scheduler.Validator = (*ClientConfig)(nil)

// RecordsNoOwnership declares no VPP claim or boot ownership records.
func (d *ClientConfig) RecordsNoOwnership() {}
