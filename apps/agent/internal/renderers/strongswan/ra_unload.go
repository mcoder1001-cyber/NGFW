package strongswan

import (
	"context"
	"slices"
	"strconv"
	"strings"
)

// UnloadRA removes exactly one private daemon's owned connection and pools,
// then requires complete empty readback. Unknown objects are never removed.
func UnloadRA(ctx context.Context, client ViciConn, connection string, pools []string) error {
	if client == nil || !strings.HasPrefix(connection, "ra-") || !validRAUnloadName(strings.TrimPrefix(connection, "ra-")) || len(pools) == 0 || len(pools) > 16 {
		return ErrRAObservation
	}
	seen := map[string]bool{}
	for _, name := range pools {
		if !validRAUnloadName(name) || seen[name] {
			return ErrRAObservation
		}
		seen[name] = true
	}
	conns, err := client.Call(ctx, "get-conns", nil)
	if err != nil || conns == nil {
		return ErrRAObservation
	}
	actual, ok := conns.Get("conns").([]string)
	if !ok || !slices.Equal(actual, []string{connection}) {
		return ErrRAObservation
	}
	observed, err := client.Call(ctx, "get-pools", nil)
	if err != nil || observed == nil || len(observed.Keys()) != len(pools) {
		return ErrRAObservation
	}
	for _, name := range observed.Keys() {
		if !seen[name] {
			return ErrRAObservation
		}
	}
	// Terminate the owned IKE name before removing its leased pools. No foreign
	// name can pass the complete connection/pool readback above.
	stats, e := client.Call(ctx, "stats", nil)
	if e != nil || stats == nil {
		return ErrRAObservation
	}
	total, e := strconv.ParseUint(str(sub(stats, "ikesas"), "total"), 10, 64)
	if e != nil || total > MaxRASessions {
		return ErrRAObservation
	}
	if total > 0 {
		result, e := client.Call(ctx, "terminate", msg("ike", connection, "timeout", "5000"))
		if e != nil || result == nil || str(result, "success") != "yes" {
			return ErrRAObservation
		}
	}
	for _, command := range []string{"unload-conn"} {
		result, e := client.Call(ctx, command, msg("name", connection))
		if e != nil || result == nil || str(result, "success") != "yes" {
			return ErrRAObservation
		}
	}
	for _, name := range pools {
		result, e := client.Call(ctx, "unload-pool", msg("name", name))
		if e != nil || result == nil || str(result, "success") != "yes" {
			return ErrRAObservation
		}
	}
	remaining, e := client.Call(ctx, "get-conns", nil)
	if e != nil || remaining == nil {
		return ErrRAObservation
	}
	names, ok := remaining.Get("conns").([]string)
	if !ok || len(names) != 0 {
		return ErrRAObservation
	}
	remaining, e = client.Call(ctx, "get-pools", nil)
	if e != nil || remaining == nil || len(remaining.Keys()) != 0 {
		return ErrRAObservation
	}
	return nil
}

func validRAUnloadName(name string) bool {
	canonical, err := ConnName(TunnelName(name))
	return err == nil && canonical == name
}
