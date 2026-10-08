package subsystems

import (
	"context"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/secretvalue"
	"sync"
)

type hostSecretSource struct {
	mu      sync.RWMutex
	ref     func(context.Context, string) (string, error)
	history func(context.Context, string) ([]byte, error)
}

var hostSecretSources sync.Map

func hostServiceSecrets(owner string) *hostSecretSource {
	source, _ := hostSecretSources.LoadOrStore(owner, &hostSecretSource{})
	return source.(*hostSecretSource)
}
func SetHostServiceSecrets(owner string, ref func(context.Context, string) (string, error), history func(context.Context, string) ([]byte, error)) error {
	if ref == nil || history == nil {
		return secretvalue.ErrInvalid
	}
	source := hostServiceSecrets(owner)
	source.mu.Lock()
	source.ref, source.history = ref, history
	source.mu.Unlock()
	return nil
}
func HostServiceSecretOptions(owner string) desired.HostSecretOptions {
	source := hostServiceSecrets(owner)
	return desired.HostSecretOptions{Ref: func(ctx context.Context, ref string) (string, error) {
		source.mu.RLock()
		f := source.ref
		source.mu.RUnlock()
		if f == nil {
			return "", secretvalue.ErrInvalid
		}
		return f(ctx, ref)
	}}
}
func (s *hostSecretSource) resolve(ctx context.Context, ref string) ([]byte, error) {
	generation, ok := secretvalue.Binding(ctx, ref)
	if !ok {
		return nil, secretvalue.ErrInvalid
	}
	return s.resolveGeneration(ctx, generation)
}
func (s *hostSecretSource) resolveGeneration(ctx context.Context, generation string) ([]byte, error) {
	s.mu.RLock()
	f := s.history
	s.mu.RUnlock()
	if f == nil {
		return nil, secretvalue.ErrInvalid
	}
	raw, err := f(ctx, generation)
	if err != nil {
		return nil, secretvalue.ErrInvalid
	}
	return raw, nil
}
