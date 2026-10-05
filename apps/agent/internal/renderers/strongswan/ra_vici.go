package strongswan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/strongswan/govici/vici"
	"golang.org/x/sys/unix"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// DialRAVICI requires a root-owned private socket and the exact runtime-managed
// daemon PID. A connection-wide deadline also bounds govici.Subscribe, whose
// library API does not accept a context. Each operation uses a fresh connection.
func DialRAVICI(ctx context.Context, socket string, expectedPID int) (ViciConn, error) {
	if expectedPID <= 0 || !filepath.IsAbs(socket) || filepath.Clean(socket) != socket {
		return nil, ErrRAObservation
	}
	for _, path := range []string{filepath.Dir(socket), socket} {
		metadata, err := os.Lstat(path)
		if err != nil {
			return nil, ErrRAObservation
		}
		owner, ok := metadata.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != 0 || metadata.Mode()&0077 != 0 {
			return nil, ErrRAObservation
		}
		if path == socket {
			if metadata.Mode()&os.ModeSocket == 0 {
				return nil, ErrRAObservation
			}
		} else if !metadata.IsDir() {
			return nil, ErrRAObservation
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	session, err := vici.NewSession(vici.WithSocketPath(socket), vici.WithDialContext(func(callctx context.Context, network, address string) (net.Conn, error) {
		dialer := net.Dialer{Deadline: deadline}
		connection, err := dialer.DialContext(callctx, network, address)
		if err != nil {
			return nil, ErrRAObservation
		}
		fail := func() (net.Conn, error) { _ = connection.Close(); return nil, ErrRAObservation }
		unixConnection, ok := connection.(*net.UnixConn)
		if !ok {
			return fail()
		}
		raw, err := unixConnection.SyscallConn()
		if err != nil {
			return fail()
		}
		var credentials *unix.Ucred
		var credentialError error
		err = raw.Control(func(fd uintptr) {
			credentials, credentialError = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		})
		if err != nil || credentialError != nil || credentials == nil || credentials.Uid != 0 || int(credentials.Pid) != expectedPID {
			return fail()
		}
		if err := connection.SetDeadline(deadline); err != nil {
			return fail()
		}
		return &boundedConn{Conn: connection, max: MaxPacket}, nil
	}))
	if err != nil {
		return nil, ErrRAObservation
	}
	return session, nil
}

// MaxRASessions bounds the complete private daemon session snapshot.
const MaxRASessions = 1024

// ErrRAObservation reports an unsafe or incomplete read without daemon details.
var ErrRAObservation = errors.New("remote-access: incomplete or unsafe daemon observation")

// RASession contains only observed public session facts, never credential fields.
type RASession struct {
	ID                 string   `json:"id"`
	Profile            string   `json:"profile"`
	Identity           string   `json:"identity"`
	Addresses          []string `json:"addresses"`
	EstablishedSeconds uint64   `json:"establishedSeconds,string"`
	BytesIn            uint64   `json:"bytesIn,string"`
	BytesOut           uint64   `json:"bytesOut,string"`
	uniqueID           string
}

// ObserveRASessions reads only the expected private daemon connection. Generation
// is the runtime's verified daemon boot identity, preventing stale-ID reuse.
// The event buffer holds the full configured bound, avoiding govici's smaller
// CallStreaming queue; stats before/after must prove a complete stable snapshot.
func ObserveRASessions(ctx context.Context, client ViciConn, profile string, generation string,
	pools []*ngfwv1.RemoteAccessPool) ([]RASession, error) {
	if !objectNameRe.MatchString(profile) || generation == "" || len(generation) > 128 {
		return nil, ErrRAObservation
	}
	name, err := ConnName(profile)
	if err != nil {
		return nil, ErrRAObservation
	}
	name = "ra-" + name
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conns, err := client.Call(ctx, "get-conns", nil)
	if err != nil {
		return nil, ErrRAObservation
	}
	names := strs(conns, "conns")
	if len(names) != 1 || names[0] != name {
		return nil, ErrRAObservation
	}
	count := func() (uint64, error) {
		stats, err := client.Call(ctx, "stats", nil)
		if err != nil {
			return 0, ErrRAObservation
		}
		n, err := strconv.ParseUint(str(sub(stats, "ikesas"), "total"), 10, 64)
		if err != nil || n > MaxRASessions {
			return 0, ErrRAObservation
		}
		return n, nil
	}
	before, err := count()
	if err != nil {
		return nil, err
	}
	events := make(chan vici.Event, MaxRASessions+1)
	client.NotifyEvents(events)
	if err := client.Subscribe("list-sa"); err != nil {
		return nil, ErrRAObservation
	}
	if _, err := client.Call(ctx, "list-sas", msg("ike", name, "noblock", "yes")); err != nil {
		return nil, ErrRAObservation
	}
	observed := uint64(0)
	sessions := []RASession{}
	seen := map[string]bool{}
	for len(events) > 0 {
		event, ok := <-events
		if !ok {
			return nil, ErrRAObservation
		}
		if event.Name != "list-sa" || event.Message == nil {
			return nil, ErrRAObservation
		}
		for _, connection := range event.Message.Keys() {
			if connection != name {
				return nil, ErrRAObservation
			}
			sa := sub(event.Message, connection)
			if sa == nil {
				return nil, ErrRAObservation
			}
			observed++
			if observed > MaxRASessions {
				return nil, ErrRAObservation
			}
			unique := str(sa, "uniqueid")
			numeric, err := strconv.ParseUint(unique, 10, 64)
			if err != nil || numeric == 0 || seen[unique] {
				return nil, ErrRAObservation
			}
			seen[unique] = true
			if str(sa, "state") != "ESTABLISHED" {
				continue
			}
			identity := str(sa, "remote-eap-id")
			if identity == "" {
				identity = str(sa, "remote-id")
			}
			if identity == "" || len(identity) > 255 {
				return nil, ErrRAObservation
			}
			if _, err := Quote(identity); err != nil {
				return nil, ErrRAObservation
			}
			addresses := strs(sa, "remote-vips")
			if len(addresses) == 0 || len(addresses) > 16 {
				return nil, ErrRAObservation
			}
			for _, address := range addresses {
				ip, err := netip.ParseAddr(address)
				if err != nil {
					return nil, ErrRAObservation
				}
				owned := false
				for _, pool := range pools {
					prefix, err := netip.ParsePrefix(pool.GetPrefix())
					if err == nil && prefix.Contains(ip) {
						owned = true
						break
					}
				}
				if !owned {
					return nil, ErrRAObservation
				}
			}
			established, err := strconv.ParseUint(str(sa, "established"), 10, 64)
			if err != nil {
				return nil, ErrRAObservation
			}
			digest := sha256.Sum256([]byte(generation + "\x00" + name + "\x00" + unique))
			session := RASession{ID: hex.EncodeToString(digest[:]), Profile: profile, Identity: identity, Addresses: slices.Clone(addresses), EstablishedSeconds: established, uniqueID: unique}
			children := sub(sa, "child-sas")
			if children == nil {
				return nil, ErrRAObservation
			}
			for _, childName := range children.Keys() {
				child := sub(children, childName)
				if child == nil {
					return nil, ErrRAObservation
				}
				if str(child, "state") != "INSTALLED" {
					continue
				}
				for _, field := range []string{"if-id-in", "if-id-out"} {
					id, err := strconv.ParseUint(strings.TrimPrefix(str(child, field), "0x"), 16, 32)
					if err != nil || id != 1 {
						return nil, ErrRAObservation
					}
				}
				for _, field := range []string{"bytes-in", "bytes-out"} {
					value, err := strconv.ParseUint(str(child, field), 10, 64)
					if err != nil {
						return nil, ErrRAObservation
					}
					target := &session.BytesIn
					if field == "bytes-out" {
						target = &session.BytesOut
					}
					if ^uint64(0)-*target < value {
						return nil, ErrRAObservation
					}
					*target += value
				}
			}
			sessions = append(sessions, session)
		}
	}
	after, err := count()
	if err != nil || before != after || observed != before {
		return nil, ErrRAObservation
	}
	slices.SortFunc(sessions, func(a, b RASession) int { return strings.Compare(a.ID, b.ID) })
	return sessions, nil
}

// DisconnectRASession proves current private-daemon membership before using its
// numeric IKE unique ID. Foreign/stale caller strings never become VICI filters.
// The runtime must hold its generation/daemon lock across this operation.
func DisconnectRASession(ctx context.Context, client ViciConn, profile, generation, id string,
	pools []*ngfwv1.RemoteAccessPool) error {
	if len(id) != 64 {
		return ErrRAObservation
	}
	if _, err := hex.DecodeString(id); err != nil {
		return ErrRAObservation
	}
	sessions, err := ObserveRASessions(ctx, client, profile, generation, pools)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.ID == id {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if _, err := client.Call(ctx, "terminate", msg("ike-id", session.uniqueID, "timeout", "5000")); err != nil {
				return fmt.Errorf("remote-access: owned session termination failed")
			}
			remaining, err := ObserveRASessions(ctx, client, profile, generation, pools)
			if err != nil {
				return err
			}
			for _, item := range remaining {
				if item.ID == id {
					return ErrRAObservation
				}
			}
			return nil
		}
	}
	return ErrRAObservation
}
