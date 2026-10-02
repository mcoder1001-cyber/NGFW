package basepolicy

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/scheduler"
)

func elementTransaction(host string, add bool) []byte {
	operation := "delete"
	if add {
		operation = "add"
	}
	quoted, _ := json.Marshal(host)
	return []byte(operation + " element inet vrx_base punt_interfaces { " + string(quoted) + " }\n")
}
func (r *Renderer) mutate(ctx context.Context, input []byte) error {
	out, err := r.runner.Run(ctx, renderers.Command{Path: NftBin, Args: []string{"-f", "-"}, Stdin: input, Timeout: 10 * time.Second})
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return errors.New("basepolicy: nft element mutation rejected")
	}
	return nil
}

// Element mutates exactly one dynamic-owned interface. Permanent exclusion and
// owned pair proof are descriptor obligations. Errors after possible mutation
// are resolved with independent readback/compensation before being returned.
func (r *Renderer) Element(ctx context.Context, host string, add bool) error {
	if _, err := Members(r.management, nil, []string{host}); err != nil {
		return err
	}
	before, err := r.Retrieve(ctx)
	if err != nil {
		return err
	}
	present := slices.Contains(before, host)
	if present == add {
		return nil
	}
	after := slices.Clone(before)
	if add {
		after = append(after, host)
		slices.Sort(after)
	} else {
		after = slices.DeleteFunc(after, func(name string) bool { return name == host })
	}
	if _, err := Members(r.management, nil, after); err != nil {
		return err
	}
	failure := r.mutate(ctx, elementTransaction(host, add))
	if failure == nil {
		return nil
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	observed, readErr := r.Retrieve(recovery)
	if readErr == nil {
		if slices.Equal(observed, after) {
			return nil
		}
		if slices.Equal(observed, before) {
			return failure
		}
	}
	// The inverse is exact-element only. Its exit status is not sufficient proof;
	// a failed inverse can still have committed, so always confirm via readback.
	compensationErr := r.mutate(recovery, elementTransaction(host, present))
	restored, verifyErr := r.Retrieve(recovery)
	if verifyErr == nil && slices.Equal(restored, before) {
		return failure
	}
	return errors.Join(scheduler.ErrUncertainOutcome, failure, readErr, compensationErr, verifyErr)
}
