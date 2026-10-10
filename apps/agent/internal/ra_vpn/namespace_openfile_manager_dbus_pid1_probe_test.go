//go:build lab_ra_pid1_readonly

package ravpn

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"ngfw/agent/internal/vpp/bootid"
)

// Explicit opt-in laboratory probe: literal read-only Manager.Version only.
// Uses the production protected-socket/PID1/authentication/transport boundary;
// never Subscribe, Hello, unit loading, activation, reload or subscription.
func TestManagerDBusActualPID1ReadOnlyVersion(t *testing.T) {
	managerDBusActualPID1ReadOnlyVersion(t, 40, 5)
}

// A separate ordinary request proof; never substitutes for the stress40 result.
func TestManagerDBusActualPID1SingleReadOnlyVersion(t *testing.T) {
	managerDBusActualPID1ReadOnlyVersion(t, 1, 1)
}

func managerDBusActualPID1ReadOnlyVersion(t *testing.T, queries, connections int) {
	t.Helper()
	if os.Getenv("NGFW_RA_PID1_READONLY_PROBE") != "1" || os.Geteuid() != 0 {
		t.Fatal("explicit root read-only probe authorization required")
	}
	for i := 0; i < connections; i++ {
		t.Run(time.Now().UTC().Format("150405.000000000"), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), managerDBusQueryBudget)
			defer cancel()
			identity := (bootid.Reader{}).ForPID(1)
			held, err := holdManagerDBusSocket()
			if err != nil {
				t.Fatal(err)
			}
			defer held.Close()
			raw, err := (&net.Dialer{}).DialContext(ctx, "unix", managerDBusSocket)
			if err != nil {
				t.Fatal(err)
			}
			conn, ok := raw.(*net.UnixConn)
			if !ok {
				raw.Close()
				t.Fatal("unexpected transport")
			}
			transport := &managerDBusTransport{conn: conn, pending: make(map[uint32]bool)}
			defer transport.Close()
			deadline, ok := ctx.Deadline()
			if !ok || conn.SetDeadline(deadline) != nil || held.Verify() != nil || !managerDBusPeer(conn, identity) {
				t.Fatal("protected PID1 boundary refused")
			}
			done := make(chan struct{})
			stop := context.AfterFunc(ctx, func() { defer close(done); transport.Close() })
			defer func() {
				if !stop() {
					<-done
				}
			}()
			bus, err := dbus.NewConn(transport, dbus.WithContext(ctx))
			if err != nil {
				t.Fatal(err)
			}
			defer bus.Close()
			if err := bus.Auth([]dbus.Auth{dbus.AuthExternal("0")}); err != nil {
				t.Fatal("actual PID1 auth", err)
			}
			count := 0
			for count < queries {
				var value dbus.Variant
				call := bus.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", dbus.FlagNoAutoStart, "org.freedesktop.systemd1.Manager", "Version")
				err = call.Store(&value)
				if err != nil {
					break
				}
				version, ok := value.Value().(string)
				if !ok || version == "" {
					err = ErrBoundary
					break
				}
				count++
				if count < queries {
					timer := time.NewTimer(40 * time.Millisecond)
					select {
					case <-timer.C:
					case <-ctx.Done():
						timer.Stop()
						err = ctx.Err()
					}
					if err != nil {
						break
					}
				}
			}
			if closeErr := bus.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if held.Verify() != nil || !(bootid.Reader{}).ForPID(1).Equal(identity) {
				t.Fatal("PID1/socket identity changed")
			}
			t.Logf("read-only fixed replies=%d signals=%d counted-bytes=%d limit=%d query-budget=%s", count, transport.signals, transport.read, managerDBusLimit, managerDBusQueryBudget)
			if err != nil {
				t.Fatal("actual read-only query failed under unchanged budgets", err)
			}
			if count != queries {
				t.Fatal("missing genuine property replies")
			}
		})
	}
}
