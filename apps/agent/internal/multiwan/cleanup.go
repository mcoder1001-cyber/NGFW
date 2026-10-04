package multiwan

import (
	"context"
	"fmt"
	"ngfw/agent/internal/descriptors/nat44ed"
	"sort"
	"strings"
)

type SessionSource interface {
	Users(context.Context) ([]nat44ed.User, error)
	UserSessions(context.Context, nat44ed.User, int, int) ([]nat44ed.Session, error)
	DeleteSession(context.Context, nat44ed.Endpoint, string, uint32, nat44ed.Endpoint) error
}

type CleanupProgress struct {
	UsersIdentity string
	User, Offset  int
}

// ClearDeadSessions bounds dump and deletion work. Caller holds the transaction
// lock and retains pending addresses until a complete scan succeeds.
func ClearDeadSessions(ctx context.Context, src SessionSource, addresses map[string]bool, tables map[uint32]bool, progress *CleanupProgress) (deleted int, complete bool, err error) {
	users, err := src.Users(ctx)
	if err != nil {
		return 0, false, err
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].VRF != users[j].VRF {
			return users[i].VRF < users[j].VRF
		}
		return users[i].IP < users[j].IP
	})
	var ids []string
	for _, u := range users {
		ids = append(ids, fmt.Sprintf("%d/%s", u.VRF, u.IP))
	}
	identity := strings.Join(ids, ";")
	if progress.UsersIdentity != identity {
		progress.User = 0
		progress.Offset = 0
		progress.UsersIdentity = identity
	}
	dumps := 0
	for progress.User < len(users) {
		u := users[progress.User]
		if !tables[u.VRF] {
			progress.User++
			progress.Offset = 0
			continue
		}
		if dumps >= 32 {
			return deleted, false, nil
		}
		dumps++
		rows, e := src.UserSessions(ctx, u, progress.Offset, 513)
		if e != nil {
			return deleted, false, e
		}
		more := len(rows) > 512
		if more {
			rows = rows[:512]
		}
		pageDeleted := 0
		for _, s := range rows {
			if !addresses[s.Outside.IP] || s.Static {
				continue
			}
			if deleted >= 512 {
				return deleted, false, nil
			}
			if e := ctx.Err(); e != nil {
				return deleted, false, e
			}
			if e := src.DeleteSession(ctx, s.Inside, s.Protocol, u.VRF, s.ExtHost); e != nil {
				return deleted, false, e
			}
			deleted++
			pageDeleted++
		}
		if more {
			if pageDeleted == 0 {
				progress.Offset += 512
			}
			return deleted, false, nil
		}
		progress.User++
		progress.Offset = 0
	}
	*progress = CleanupProgress{}
	return deleted, true, nil
}
