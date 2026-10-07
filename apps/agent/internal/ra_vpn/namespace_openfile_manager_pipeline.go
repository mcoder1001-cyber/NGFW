package ravpn

import (
	"context"
	"strings"

	"github.com/godbus/dbus/v5"
)

const managerDBusRolePendingLimit = 12

type managerDBusOwnedCall struct {
	call      *dbus.Call
	completed bool
}

func (owned *managerDBusOwnedCall) await(ctx context.Context) (dbus.Variant, error) {
	if owned.call == nil || owned.call.Done == nil || cap(owned.call.Done) != 1 || owned.call.Context().Done() == nil {
		return dbus.Variant{}, ErrBoundary
	}
	select {
	case completed := <-owned.call.Done:
		owned.completed = true
		if completed != owned.call || ctx.Err() != nil {
			return dbus.Variant{}, ErrBoundary
		}
		var value dbus.Variant
		if completed.Store(&value) != nil {
			return dbus.Variant{}, ErrBoundary
		}
		return value, nil
	case <-ctx.Done():
		return dbus.Variant{}, ErrBoundary
	}
}

// readManagerDBusPipelineProperties owns every SDK call until its buffered
// completion and context retirement. Only one fixed role's public fields can
// be outstanding; Id and role transitions remain synchronous barriers.
func readManagerDBusPipelineProperties(parent context.Context, roles []managerDBusRole, dispatch func(context.Context, managerDBusRole, string) *dbus.Call, closeConnection func() error) (result []map[string]string, resultErr error) {
	if parent.Err() != nil || !validManagerDBusRoles(roles) || dispatch == nil || closeConnection == nil {
		return nil, ErrBoundary
	}
	ctx, cancel := context.WithCancel(parent)
	var calls []*managerDBusOwnedCall
	defer func() {
		cancel()
		if resultErr != nil && closeConnection() != nil {
			resultErr = ErrBoundary
		}
		for _, owned := range calls {
			if owned.call == nil || owned.call.Context().Done() == nil {
				result = nil
				resultErr = ErrBoundary
				continue
			}
			owned.call.ContextCancel()
			if !owned.completed {
				if owned.call.Done == nil || cap(owned.call.Done) != 1 {
					result = nil
					resultErr = ErrBoundary
					continue
				}
				if <-owned.call.Done != owned.call {
					result = nil
					resultErr = ErrBoundary
				}
				owned.completed = true
			}
			// SDK done publishes the result before canceling the call context.
			// This waits that documented ownership boundary, not a private worker API.
			if owned.call.Context().Done() == nil {
				result = nil
				resultErr = ErrBoundary
				continue
			}
			<-owned.call.Context().Done()
		}
	}()
	enqueue := func(role managerDBusRole, key string) (*managerDBusOwnedCall, error) {
		if ctx.Err() != nil {
			return nil, ErrBoundary
		}
		owned := &managerDBusOwnedCall{call: dispatch(ctx, role, key)}
		calls = append(calls, owned)
		if owned.call == nil || owned.call.Done == nil || cap(owned.call.Done) != 1 || owned.call.Context().Done() == nil {
			return nil, ErrBoundary
		}
		return owned, nil
	}
	for _, role := range roles {
		keys := strings.Split(role.fields, ",")
		if len(keys) < 2 || keys[0] != "Id" || len(keys)-1 > managerDBusRolePendingLimit {
			return nil, ErrBoundary
		}
		first, err := enqueue(role, "Id")
		if err != nil {
			return nil, ErrBoundary
		}
		identity, err := first.await(ctx)
		if err != nil {
			return nil, ErrBoundary
		}
		id, err := managerDBusPropertyValue("Id", identity)
		if err != nil || id != role.name {
			return nil, ErrBoundary
		}
		fields := map[string]string{"Id": id}
		pending := make([]*managerDBusOwnedCall, 0, len(keys)-1)
		for _, key := range keys[1:] {
			owned, err := enqueue(role, key)
			if err != nil {
				return nil, ErrBoundary
			}
			pending = append(pending, owned)
		}
		for i, owned := range pending {
			value, err := owned.await(ctx)
			if err != nil {
				return nil, ErrBoundary
			}
			text, err := managerDBusPropertyValue(keys[i+1], value)
			if err != nil {
				return nil, ErrBoundary
			}
			fields[keys[i+1]] = text
		}
		last, err := enqueue(role, "Id")
		if err != nil {
			return nil, ErrBoundary
		}
		identity, err = last.await(ctx)
		if err != nil {
			return nil, ErrBoundary
		}
		id, err = managerDBusPropertyValue("Id", identity)
		if err != nil || id != role.name {
			return nil, ErrBoundary
		}
		result = append(result, fields)
	}
	if ctx.Err() != nil {
		return nil, ErrBoundary
	}
	return result, nil
}
