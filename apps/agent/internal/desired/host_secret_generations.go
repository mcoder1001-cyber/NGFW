package desired

import (
	"context"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/secretvalue"
)

// HostSecretOptions selects keyed generations from the transaction's sealed cache.
type HostSecretOptions struct {
	Ref func(context.Context, string) (string, error)
}

func bindHostSecrets(s Sink, value proto.Message, pointer string, options []HostSecretOptions) proto.Message {
	refs := secretvalue.References(value)
	if len(refs) == 0 {
		return value
	}
	if len(options) == 0 || options[0].Ref == nil {
		s.Errorf(pointer, RuleSecretChannel, "selected secret generations are unavailable")
		return nil
	}
	bindings := map[string]string{}
	for _, ref := range refs {
		generation, err := options[0].Ref(context.Background(), ref)
		if err != nil {
			s.Errorf(pointer, RuleSecretChannel, "selected secret generation is unavailable")
			return nil
		}
		bindings[ref] = generation
	}
	wrapped, err := secretvalue.Wrap(value, bindings)
	if err != nil {
		s.Errorf(pointer, RuleSecretChannel, "selected secret generation is invalid")
		return nil
	}
	return wrapped
}
