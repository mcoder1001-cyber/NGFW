package subsystems

import (
	"context"
	"path/filepath"
	"sync"

	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
)

type nativeSecretResolver struct {
	mu          sync.RWMutex
	resolver    vpn.Resolver
	fingerprint func(context.Context, string) (string, error)
	ready       func(context.Context) error
	profile     *ikev2.Profile
}

func (r *nativeSecretResolver) Resolve(ctx context.Context, ref string) ([]byte, error) {
	r.mu.RLock()
	resolver := r.resolver
	r.mu.RUnlock()
	if resolver == nil {
		return nil, vpn.ErrNoResolver
	}
	return resolver.Resolve(ctx, ref)
}
func (r *nativeSecretResolver) Ref(ctx context.Context, ref string) (string, error) {
	r.mu.RLock()
	fn := r.fingerprint
	r.mu.RUnlock()
	if fn == nil {
		return "", vpn.ErrNoResolver
	}
	return fn(ctx, ref)
}

var nativeMu sync.Mutex
var nativeByOwner = map[string]*nativeSecretResolver{}
var nativeCurrent *nativeSecretResolver

func init() {
	Domains[VPN] = append(Domains[VPN], ikev2.ProfileName, ikev2.ResponderHostnameName, ikev2.LocalKeyName, ikev2.SleepIntervalName, ikev2.LivenessName, ikev2.MetaName)
}

func (w *Wiring) registerIKEv2(r scheduler.Registry) error {
	opts, err := w.IKEv2Options()
	if err != nil {
		return err
	}
	secrets := &nativeSecretResolver{ready: func(ctx context.Context) error { return ikev2.RequireSafeState(ctx, w.env.Client) }}
	cfg := ikev2.Config{Client: w.env.Client, Owner: w.env.Owner, Secrets: secrets}
	for _, opt := range opts {
		opt(&cfg)
	}
	secrets.profile = ikev2.NewProfile(cfg)
	ikev2.Register(wgRegistry{r}, w.env.Client, w.env.Owner, append(opts, ikev2.WithSecrets(secrets))...)
	store, err := ikev2.NewFileMetaStore(filepath.Join(w.env.StateDir, "ikev2-meta-"+w.env.Owner+".json"))
	if err != nil {
		return err
	}
	r.Register(ikev2.NewMeta(store))
	nativeMu.Lock()
	nativeByOwner[w.env.Owner] = secrets
	nativeCurrent = secrets
	nativeMu.Unlock()
	return nil
}

func IKEv2Profiles(ctx context.Context, owner string) ([]scheduler.KV, error) {
	nativeMu.Lock()
	r := nativeByOwner[owner]
	nativeMu.Unlock()
	if r == nil {
		return nil, vpn.ErrNoResolver
	}
	return r.profile.Retrieve(ctx)
}

// SetIKEv2Secrets attaches the API-delivered reference resolver without rebuilding
// descriptors. Resolver material stays outside scheduler values and RPC responses.
func SetIKEv2Secrets(owner string, resolver vpn.Resolver, fingerprint func(context.Context, string) (string, error)) error {
	nativeMu.Lock()
	r := nativeByOwner[owner]
	nativeMu.Unlock()
	if r == nil {
		return vpn.ErrNoResolver
	}
	r.mu.Lock()
	r.resolver = resolver
	r.fingerprint = fingerprint
	r.mu.Unlock()
	return nil
}
func IKEv2Projection() desired.IKEv2Env {
	nativeMu.Lock()
	r := nativeCurrent
	nativeMu.Unlock()
	if r == nil {
		return desired.IKEv2Env{}
	}
	return desired.IKEv2Env{SecretRef: r.Ref, CheckReady: r.ready}
}
