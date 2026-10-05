// Package hasync owns NAT44-EI global HA endpoints. Slots may only require values.
package hasync

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"ngfw/agent/binapi/ip_types"
	natapi "ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// NameListener identifies the native HA listener singleton.
const NameListener = "nat44-ei-ha.listener"

// NameFailover identifies the native HA failover singleton.
const NameFailover = "nat44-ei-ha.failover"

// ListenerKey is the scheduler key of the listener singleton.
var ListenerKey = scheduler.Join(NameListener, "global")

// FailoverKey is the scheduler key of the failover singleton.
var FailoverKey = scheduler.Join(NameFailover, "global")

// Listener is the typed desired native UDP listener.
type Listener struct {
	Address string `json:"address"`
	Port    uint16 `json:"port"`
	PathMtu uint32 `json:"path_mtu"`
}

// Failover is the typed desired native UDP peer.
type Failover struct {
	Address           string `json:"address"`
	Port              uint16 `json:"port"`
	SessionRefreshSec uint32 `json:"session_refresh_sec"`
}

// Address validates and encodes a unicast IPv4 endpoint.
func Address(value string) (ip_types.IP4Address, error) {
	a, err := netip.ParseAddr(value)
	if err != nil || !a.Is4() || a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() || a == netip.MustParseAddr("255.255.255.255") {
		return ip_types.IP4Address{}, dfkit.Specf("HA endpoint must be unicast IPv4")
	}
	return ip_types.IP4Address(a.As4()), nil
}

// Validate rejects invalid listener ports, addresses and MTUs.
func (l Listener) Validate() error {
	if _, err := Address(l.Address); err != nil {
		return err
	}
	if l.Port == 0 || l.PathMtu < 576 || l.PathMtu > 9000 {
		return dfkit.Specf("HA listener port/MTU out of bounds")
	}
	return nil
}

// Validate rejects invalid peer ports, addresses and refresh intervals.
func (f Failover) Validate() error {
	if _, err := Address(f.Address); err != nil {
		return err
	}
	if f.Port == 0 || f.SessionRefreshSec < 1 || f.SessionRefreshSec > 3600 {
		return dfkit.Specf("HA peer port/refresh out of bounds")
	}
	return nil
}

// Lock serializes with the project's globals lock and honors cancellation while waiting.
// lockDir is fixed by registration; tests use a dedicated temporary directory.
func Lock(ctx context.Context, lockDir string) (func(), error) {
	path := filepath.Join(lockDir, "ngfw-globals.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // fixed basename in configured lock directory
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err = ctx.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			_ = f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// ReadListener observes native listener values without mutation.
func ReadListener(ctx context.Context, c vpp.Client) (Listener, error) {
	r, err := natapi.NewServiceClient(c).Nat44EiHaGetListener(ctx, &natapi.Nat44EiHaGetListener{})
	if err != nil {
		return Listener{}, err
	}
	return Listener{Address: r.IPAddress.String(), Port: r.Port, PathMtu: r.PathMtu}, nil
}

// ReadFailover observes native failover values without mutation.
func ReadFailover(ctx context.Context, c vpp.Client) (Failover, error) {
	r, err := natapi.NewServiceClient(c).Nat44EiHaGetFailover(ctx, &natapi.Nat44EiHaGetFailover{})
	if err != nil {
		return Failover{}, err
	}
	return Failover{Address: r.IPAddress.String(), Port: r.Port, SessionRefreshSec: r.SessionRefreshInterval}, nil
}

// NewListener builds an owner setter or slot requirement descriptor.
func NewListener(c vpp.Client, opts ...natcommon.Option) *natcommon.Descriptor[Listener] {
	cfg := natcommon.BuildConfig(opts)
	set := func(ctx context.Context, l Listener) error {
		if err := l.Validate(); err != nil {
			return err
		}
		ip, _ := Address(l.Address)
		release, err := Lock(ctx, cfg.LockDir)
		if err != nil {
			return err
		}
		defer release()
		_, err = natapi.NewServiceClient(c).Nat44EiHaSetListener(ctx, &natapi.Nat44EiHaSetListener{IPAddress: ip, Port: l.Port, PathMtu: l.PathMtu})
		return err
	}
	return natcommon.Global(cfg, natcommon.GlobalOps[Listener]{Name: NameListener, ID: "global",
		Deps: func(Listener) []scheduler.Dependency { return []scheduler.Dependency{natcommon.Dep(nat44ei.EnableKey)} },
		Read: func(ctx context.Context) (natcommon.GlobalState[Listener], error) {
			l, err := ReadListener(ctx, c)
			return natcommon.GlobalState[Listener]{Value: l, Present: l.Port != 0, Observable: true}, err
		}, Set: set,
		Reset: func(ctx context.Context, _ Listener) error {
			release, err := Lock(ctx, cfg.LockDir)
			if err != nil {
				return err
			}
			defer release()
			_, err = natapi.NewServiceClient(c).Nat44EiHaSetListener(ctx, &natapi.Nat44EiHaSetListener{PathMtu: 1500})
			return err
		},
	})
}

// NewFailover builds an owner setter or slot requirement descriptor.
func NewFailover(c vpp.Client, opts ...natcommon.Option) *natcommon.Descriptor[Failover] {
	cfg := natcommon.BuildConfig(opts)
	set := func(ctx context.Context, f Failover) error {
		if err := f.Validate(); err != nil {
			return err
		}
		ip, _ := Address(f.Address)
		release, err := Lock(ctx, cfg.LockDir)
		if err != nil {
			return err
		}
		defer release()
		_, err = natapi.NewServiceClient(c).Nat44EiHaSetFailover(ctx, &natapi.Nat44EiHaSetFailover{IPAddress: ip, Port: f.Port, SessionRefreshInterval: f.SessionRefreshSec})
		return err
	}
	return natcommon.Global(cfg, natcommon.GlobalOps[Failover]{Name: NameFailover, ID: "global",
		Deps: func(Failover) []scheduler.Dependency {
			return []scheduler.Dependency{natcommon.Dep(nat44ei.EnableKey), natcommon.Dep(ListenerKey)}
		},
		Read: func(ctx context.Context) (natcommon.GlobalState[Failover], error) {
			f, err := ReadFailover(ctx, c)
			return natcommon.GlobalState[Failover]{Value: f, Present: f.Port != 0, Observable: true}, err
		}, Set: set,
		Reset: func(ctx context.Context, _ Failover) error {
			release, err := Lock(ctx, cfg.LockDir)
			if err != nil {
				return err
			}
			defer release()
			_, err = natapi.NewServiceClient(c).Nat44EiHaSetFailover(ctx, &natapi.Nat44EiHaSetFailover{SessionRefreshInterval: 10})
			return err
		},
	})
}

// Register adds both HA singleton descriptors.
func Register(r scheduler.Registry, c vpp.Client, opts ...natcommon.Option) {
	r.Register(NewListener(c, opts...))
	r.Register(NewFailover(c, opts...))
}

// ExplainUnavailable formats an observation failure without inventing state.
func ExplainUnavailable(err error) string {
	return fmt.Sprintf("native NAT44-EI HA observation unavailable: %v", err)
}
