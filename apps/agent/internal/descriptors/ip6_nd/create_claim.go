package ip6nd

import (
	"context"
	"errors"

	"ngfw/agent/internal/scheduler"
)

// A failed RA operation may already have changed VPP (configuration is several
// calls; prefix creation includes timer capture). Retain the prior durable claim
// and supply the interface handle for rollback whenever readback finds a mutation
// or cannot prove its absence. Release only a verified uncreated claim.
func raCreateFailure(ctx context.Context, descriptor scheduler.Descriptor, key scheduler.Key, meta RaMeta, undo func(), cause error) (any, error) {
	live, err := descriptor.Retrieve(ctx)
	if err != nil {
		return meta, scheduler.PartialCreate(errors.Join(cause, err))
	}
	for _, kv := range live {
		if kv.Key == key {
			return meta, scheduler.PartialCreate(cause)
		}
	}
	undo()
	return nil, cause
}
