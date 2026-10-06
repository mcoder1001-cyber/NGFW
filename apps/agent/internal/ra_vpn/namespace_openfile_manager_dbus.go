package ravpn

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

const managerDBusSocket = "/run/systemd/private"
const managerDBusQueryBudget = 2 * time.Second

type managerDBusSocketProof struct {
	files  []*os.File
	stamps []unix.Stat_t
}

func (p *managerDBusSocketProof) Close() error {
	var result error
	for _, file := range p.files {
		if file.Close() != nil {
			result = ErrBoundary
		}
	}
	p.files = nil
	return result
}

func managerDBusSameStamp(a, b unix.Stat_t) bool {
	if a.Dev != b.Dev || a.Ino != b.Ino || a.Mode != b.Mode || a.Uid != b.Uid || a.Gid != b.Gid {
		return false
	}
	// Protected directories may acquire unrelated children; their namespace
	// identity and permissions remain pinned. The exact socket stamp is immutable.
	return a.Mode&unix.S_IFMT == unix.S_IFDIR || a.Nlink == b.Nlink && a.Ctim == b.Ctim && a.Mtim == b.Mtim
}

func managerDBusProtectedStamp(st unix.Stat_t, socket bool) bool {
	if st.Uid != 0 || st.Gid != 0 || st.Mode&022 != 0 {
		return false
	}
	if !socket {
		return st.Mode&unix.S_IFMT == unix.S_IFDIR
	}
	return st.Mode&unix.S_IFMT == unix.S_IFSOCK && st.Nlink == 1 && (st.Mode&07777 == 0600 || st.Mode&07777 == 0700)
}

func holdManagerDBusSocket() (proof *managerDBusSocketProof, resultErr error) {
	proof = &managerDBusSocketProof{}
	defer func() {
		if resultErr != nil {
			if proof.Close() != nil {
				resultErr = ErrBoundary
			}
			proof = nil
		}
	}()
	fd, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return proof, ErrBoundary
	}
	proof.files = append(proof.files, os.NewFile(uintptr(fd), "/"))
	for index, part := range []string{"", "run", "systemd", "private"} {
		if index > 0 {
			next, err := unix.Openat(fd, part, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return proof, ErrBoundary
			}
			fd = next
			proof.files = append(proof.files, os.NewFile(uintptr(fd), part))
		}
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || !managerDBusProtectedStamp(st, index == 3) {
			return proof, ErrBoundary
		}
		proof.stamps = append(proof.stamps, st)
	}
	return proof, nil
}

func (p *managerDBusSocketProof) Verify() error {
	if p == nil || len(p.files) != 4 || len(p.stamps) != 4 {
		return ErrBoundary
	}
	for i, file := range p.files {
		var actual unix.Stat_t
		if unix.Fstat(int(file.Fd()), &actual) != nil || !managerDBusSameStamp(p.stamps[i], actual) {
			return ErrBoundary
		}
	}
	fresh, err := holdManagerDBusSocket()
	if err != nil {
		return ErrBoundary
	}
	valid := true
	for i, stamp := range fresh.stamps {
		if !managerDBusSameStamp(p.stamps[i], stamp) {
			valid = false
		}
	}
	if fresh.Close() != nil || !valid {
		return ErrBoundary
	}
	return nil
}

func managerDBusPeer(conn *net.UnixConn, expected bootid.Identity) bool {
	if !expected.Complete() || expected.PID != 1 || !(bootid.Reader{}).ForPID(1).Equal(expected) {
		return false
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	valid := false
	if raw.Control(func(fd uintptr) {
		peer, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		valid = err == nil && peer.Pid == 1 && peer.Uid == 0 && peer.Gid == 0
	}) != nil {
		return false
	}
	return valid && (bootid.Reader{}).ForPID(1).Equal(expected)
}

type managerDBusRole struct{ name, path, fields string }

var managerDBusPublisherRoles = []managerDBusRole{
	{"ngfw-ra-openfile.socket", "/org/freedesktop/systemd1/unit/ngfw_2dra_2dopenfile_2esocket", "Id,FragmentPath,DropInPaths,ActiveState,SubState,Listen"},
	{"ngfw-ra-openfile.service", "/org/freedesktop/systemd1/unit/ngfw_2dra_2dopenfile_2eservice", "Id,MainPID,ControlPID,ActiveState,SubState,ControlGroup,FragmentPath,DropInPaths,User,Group,CapabilityBoundingSet,NoNewPrivileges,ExecStart"},
}

var managerDBusSourceRole = managerDBusRole{"ngfw-agent.service", "/org/freedesktop/systemd1/unit/ngfw_2dagent_2eservice", "Id,MainPID,ControlGroup,FragmentPath,DropInPaths,User,Group,ExecStart"}

func managerDBusPropertyInterface(role managerDBusRole, key string) string {
	switch key {
	case "Id", "FragmentPath", "DropInPaths", "ActiveState", "SubState":
		return "org.freedesktop.systemd1.Unit"
	}
	if role.name == "ngfw-ra-openfile.socket" {
		return "org.freedesktop.systemd1.Socket"
	}
	return "org.freedesktop.systemd1.Service"
}

// readManagerDBusRoles performs only literal property reads from one verified
// PID1 connection. Freshness and the caller-clipped two-second bound cover the
// complete boundary; it neither loads units nor uses a system-bus fallback.
func readManagerDBusRoles(parent context.Context, roles []managerDBusRole) (result []map[string]string, resultErr error) {
	ctx, cancel := context.WithTimeout(parent, managerDBusQueryBudget)
	defer cancel()
	started := time.Now()
	checkpoint := uint8(1)
	defer func() {
		if line := numericPublisherCheckpointLine(ctx, 1, checkpoint, time.Since(started), resultErr != nil); line != "" {
			log.Print(line)
		}
	}()
	if ctx.Err() != nil || os.Geteuid() != 0 || len(roles) < 1 || len(roles) > 3 {
		return nil, ErrBoundary
	}
	checkpoint = 2
	if !validManagerDBusRoles(roles) {
		return nil, ErrBoundary
	}
	checkpoint = 3
	manager := (bootid.Reader{}).ForPID(1)
	held, err := holdManagerDBusSocket()
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() {
		if held.Close() != nil {
			checkpoint = 10
			result = nil
			resultErr = ErrBoundary
		}
	}()
	checkpoint = 4
	dialer := net.Dialer{}
	raw, err := dialer.DialContext(ctx, "unix", managerDBusSocket)
	if err != nil {
		return nil, ErrBoundary
	}
	conn, ok := raw.(*net.UnixConn)
	if !ok {
		if raw.Close() != nil {
			return nil, ErrBoundary
		}
		return nil, ErrBoundary
	}
	transport := &managerDBusTransport{conn: conn, pending: make(map[uint32]bool)}
	defer func() {
		if transport.Close() != nil {
			checkpoint = 10
			result = nil
			resultErr = ErrBoundary
		}
	}()
	checkpoint = 5
	deadline, ok := ctx.Deadline()
	if !ok || conn.SetDeadline(deadline) != nil || held.Verify() != nil || !managerDBusPeer(conn, manager) {
		return nil, ErrBoundary
	}
	callbackDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(callbackDone)
		if transport.Close() != nil {
			return
		}
	})
	defer func() {
		if !stop() {
			<-callbackDone
		}
	}()
	checkpoint = 6
	bus, err := dbus.NewConn(transport, dbus.WithContext(ctx))
	if err != nil {
		return nil, ErrBoundary
	}
	defer func() {
		if bus.Close() != nil {
			checkpoint = 10
			result = nil
			resultErr = ErrBoundary
		}
	}()
	checkpoint = 7
	if bus.Auth([]dbus.Auth{dbus.AuthExternal("0")}) != nil {
		return nil, ErrBoundary
	}
	checkpoint = 8
	result, err = readManagerDBusPipelineProperties(ctx, roles, func(callContext context.Context, role managerDBusRole, key string) *dbus.Call {
		return bus.Object("org.freedesktop.systemd1", dbus.ObjectPath(role.path)).GoWithContext(callContext, "org.freedesktop.DBus.Properties.Get", dbus.FlagNoAutoStart, make(chan *dbus.Call, 1), managerDBusPropertyInterface(role, key), key)
	}, bus.Close)
	if err != nil {
		return nil, ErrBoundary
	}

	checkpoint = 9
	if held.Verify() != nil || !managerDBusPeer(conn, manager) || ctx.Err() != nil {
		return nil, ErrBoundary
	}

	return result, nil
}

// readManagerDBusProperties is the fixed-role conversion stage. The production
// getter exists only after the real held socket/PID1/authentication proofs.
func readManagerDBusProperties(ctx context.Context, roles []managerDBusRole, get func(managerDBusRole, string) (dbus.Variant, error)) ([]map[string]string, error) {
	if ctx.Err() != nil || get == nil || !validManagerDBusRoles(roles) {
		return nil, ErrBoundary
	}
	var result []map[string]string
	for _, role := range roles {
		fields := make(map[string]string)
		for _, key := range strings.Split(role.fields, ",") {
			if ctx.Err() != nil {
				return nil, ErrBoundary
			}
			value, err := get(role, key)
			if err != nil || ctx.Err() != nil {
				return nil, ErrBoundary
			}
			text, err := managerDBusPropertyValue(key, value)
			if err != nil || key == "Id" && text != role.name {
				return nil, ErrBoundary
			}
			fields[key] = text
		}
		if ctx.Err() != nil {
			return nil, ErrBoundary
		}
		identity, err := get(role, "Id")
		if err != nil || ctx.Err() != nil {
			return nil, ErrBoundary
		}
		id, err := managerDBusPropertyValue("Id", identity)
		if err != nil || id != role.name {
			return nil, ErrBoundary
		}
		result = append(result, fields)
	}
	return result, nil
}
