// The fixture owns a NAT plugin only when both ED/EI were disabled at entry.
// It never disables a preexisting plugin or one with any remaining config object.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
)

type fixture struct {
	conn        vppapi.Connection
	slot        int
	mode        string
	fingerprint string
}

func drain[T any](receive func() (T, error)) error {
	count := 0
	for {
		_, err := receive()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		count++
	}
	if count != 0 {
		return fmt.Errorf("NAT fixture contains %d objects; refuse mutation", count)
	}
	return nil
}
func emptyED(ctx context.Context, conn vppapi.Connection) error {
	s := nat44_ed.NewServiceClient(conn)
	a, e := s.Nat44InterfaceDump(ctx, &nat44_ed.Nat44InterfaceDump{})
	if e != nil {
		return e
	}
	if e = drain(a.Recv); e != nil {
		return e
	}
	b, e := s.Nat44EdOutputInterfaceGet(ctx, &nat44_ed.Nat44EdOutputInterfaceGet{})
	if e != nil {
		return e
	}
	if e = drain(func() (*nat44_ed.Nat44EdOutputInterfaceDetails, error) { item, _, err := b.Recv(); return item, err }); e != nil {
		return e
	}
	c, e := s.Nat44AddressDump(ctx, &nat44_ed.Nat44AddressDump{})
	if e != nil {
		return e
	}
	if e = drain(c.Recv); e != nil {
		return e
	}
	d, e := s.Nat44InterfaceAddrDump(ctx, &nat44_ed.Nat44InterfaceAddrDump{})
	if e != nil {
		return e
	}
	if e = drain(d.Recv); e != nil {
		return e
	}
	f, e := s.Nat44StaticMappingDump(ctx, &nat44_ed.Nat44StaticMappingDump{})
	if e != nil {
		return e
	}
	if e = drain(f.Recv); e != nil {
		return e
	}
	g, e := s.Nat44IdentityMappingDump(ctx, &nat44_ed.Nat44IdentityMappingDump{})
	if e != nil {
		return e
	}
	if e = drain(g.Recv); e != nil {
		return e
	}
	h, e := s.Nat44LbStaticMappingDump(ctx, &nat44_ed.Nat44LbStaticMappingDump{})
	if e != nil {
		return e
	}
	if e = drain(h.Recv); e != nil {
		return e
	}
	tables, e := s.Nat44EdVrfTablesV2Dump(ctx, &nat44_ed.Nat44EdVrfTablesV2Dump{})
	if e != nil {
		return e
	}
	if e = drain(tables.Recv); e != nil {
		return e
	}
	users, e := s.Nat44UserDump(ctx, &nat44_ed.Nat44UserDump{})
	if e != nil {
		return e
	}
	return drain(users.Recv)
}
func emptyEI(ctx context.Context, conn vppapi.Connection) error {
	s := nat44_ei.NewServiceClient(conn)
	a, e := s.Nat44EiInterfaceDump(ctx, &nat44_ei.Nat44EiInterfaceDump{})
	if e != nil {
		return e
	}
	if e = drain(a.Recv); e != nil {
		return e
	}
	b, e := s.Nat44EiOutputInterfaceGet(ctx, &nat44_ei.Nat44EiOutputInterfaceGet{})
	if e != nil {
		return e
	}
	if e = drain(func() (*nat44_ei.Nat44EiOutputInterfaceDetails, error) { item, _, err := b.Recv(); return item, err }); e != nil {
		return e
	}
	c, e := s.Nat44EiAddressDump(ctx, &nat44_ei.Nat44EiAddressDump{})
	if e != nil {
		return e
	}
	if e = drain(c.Recv); e != nil {
		return e
	}
	d, e := s.Nat44EiInterfaceAddrDump(ctx, &nat44_ei.Nat44EiInterfaceAddrDump{})
	if e != nil {
		return e
	}
	if e = drain(d.Recv); e != nil {
		return e
	}
	f, e := s.Nat44EiStaticMappingDump(ctx, &nat44_ei.Nat44EiStaticMappingDump{})
	if e != nil {
		return e
	}
	if e = drain(f.Recv); e != nil {
		return e
	}
	g, e := s.Nat44EiIdentityMappingDump(ctx, &nat44_ei.Nat44EiIdentityMappingDump{})
	if e != nil {
		return e
	}
	if e = drain(g.Recv); e != nil {
		return e
	}
	users, e := s.Nat44EiUserDump(ctx, &nat44_ei.Nat44EiUserDump{})
	if e != nil {
		return e
	}
	return drain(users.Recv)
}
func lease(slot int) error {
	path := fmt.Sprintf("/run/ngfw-test/w%d/traffic-a-lease.json", slot)
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	state, ok := info.Sys().(*syscall.Stat_t)
	if !ok || state.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() <= 0 || info.Size() > 4096 {
		return errors.New("invalid manager lease file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return err
	}
	var value struct {
		Task    string  `json:"task"`
		Slot    int     `json:"slot"`
		Prefix  string  `json:"prefix"`
		Boot    string  `json:"boot_id"`
		Expires float64 `json:"expires_unix"`
		ID      string  `json:"lease_id"`
	}
	pairs := json.NewDecoder(strings.NewReader(string(raw)))
	token, err := pairs.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("invalid lease JSON")
	}
	seen := map[string]bool{}
	for pairs.More() {
		token, err = pairs.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return errors.New("duplicate lease field")
		}
		seen[name] = true
		var discard json.RawMessage
		if err = pairs.Decode(&discard); err != nil {
			return err
		}
	}
	if _, err = pairs.Token(); err != nil {
		return err
	}
	if _, err = pairs.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing lease JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&value); err != nil {
		return err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}
	now := float64(time.Now().Unix())
	if value.Task != "TEST-traffic-A" || value.Slot != slot || value.Prefix != fmt.Sprintf("w%d", slot) || value.Boot != strings.TrimSpace(string(boot)) || value.Expires <= now || value.Expires > now+7200 || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(value.ID) {
		return errors.New("manager lease identity/window invalid")
	}
	return nil
}

func ownedState(mode string, edOn, eiOn bool, expected, observed string) error {
	if mode == "" && (edOn || eiOn) {
		return errors.New("NAT enabled by another owner")
	}
	if mode == "ed" && (!edOn || eiOn) || mode == "ei" && (!eiOn || edOn) {
		return errors.New("owned NAT mode changed")
	}
	if mode != "" && expected != observed {
		return errors.New("owned NAT running fingerprint changed")
	}
	return nil
}

func (f *fixture) transition(target string) error {
	if target != "off" || f.mode == "" {
		if err := lease(f.slot); err != nil {
			return err
		}
	}
	if target != "ed" && target != "ei" && target != "off" {
		return errors.New("invalid fixture command")
	}
	lock, err := os.OpenFile("/run/lock/ngfw-globals.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ed := nat44_ed.NewServiceClient(f.conn)
	ei := nat44_ei.NewServiceClient(f.conn)
	edState, err := ed.Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
	if err != nil {
		return err
	}
	eiState, err := ei.Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
	if err != nil {
		return err
	}
	edOn, eiOn := edState.Sessions != 0, eiState.Sessions != 0
	observed := ""
	if f.mode == "ed" {
		raw, _ := json.Marshal(edState)
		observed = string(raw)
	}
	if f.mode == "ei" {
		raw, _ := json.Marshal(eiState)
		observed = string(raw)
	}
	if err = ownedState(f.mode, edOn, eiOn, f.fingerprint, observed); err != nil {
		return err
	}
	if f.mode == "" && (edOn || eiOn) {
		return errors.New("NAT enabled by another owner; refuse")
	}
	table := uint32(f.slot*1000 + 10)
	if f.mode == "ed" && (!edOn || eiOn || edState.InsideVrf != table || edState.OutsideVrf != table) {
		return errors.New("owned ED global state changed; preserve")
	}
	if f.mode == "ei" && (!eiOn || edOn || eiState.InsideVrf != table || eiState.OutsideVrf != table) {
		return errors.New("owned EI global state changed; preserve")
	}
	if f.mode == "ed" {
		if err = emptyED(ctx, f.conn); err != nil {
			return err
		}
		_, err = ed.Nat44EdPluginEnableDisable(ctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: false})
	}
	if f.mode == "ei" {
		if err = emptyEI(ctx, f.conn); err != nil {
			return err
		}
		_, err = ei.Nat44EiPluginEnableDisable(ctx, &nat44_ei.Nat44EiPluginEnableDisable{Enable: false})
	}
	if err != nil {
		return err
	}
	f.mode = ""
	if target == "ed" {
		_, err = ed.Nat44EdPluginEnableDisable(ctx, &nat44_ed.Nat44EdPluginEnableDisable{Enable: true, InsideVrf: table, OutsideVrf: table})
	}
	if target == "ei" {
		_, err = ei.Nat44EiPluginEnableDisable(ctx, &nat44_ei.Nat44EiPluginEnableDisable{Enable: true, InsideVrf: table, OutsideVrf: table})
	}
	if err == nil && target != "off" {
		f.mode = target
		if target == "ed" {
			snapshot, e := ed.Nat44ShowRunningConfig(ctx, &nat44_ed.Nat44ShowRunningConfig{})
			if e != nil {
				return e
			}
			raw, _ := json.Marshal(snapshot)
			f.fingerprint = string(raw)
		}
		if target == "ei" {
			snapshot, e := ei.Nat44EiShowRunningConfig(ctx, &nat44_ei.Nat44EiShowRunningConfig{})
			if e != nil {
				return e
			}
			raw, _ := json.Marshal(snapshot)
			f.fingerprint = string(raw)
		}
	}
	return err
}
func validSlot(slot int) bool { return slot >= 1 && slot <= 11 || slot >= 14 && slot <= 32 }
func main() {
	slot := flag.Int("slot", 0, "allocated slot")
	flag.Parse()
	if !validSlot(*slot) || os.Getenv("NGFW_INTEGRATION") != "1" || os.Getenv("NGFW_TRAFFIC_A_HOST") != "1" {
		fmt.Fprintln(os.Stderr, "explicit allocated host fixture required")
		os.Exit(1)
	}
	conn, err := govpp.Connect("/run/vpp/api.sock")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Disconnect()
	f := fixture{conn: conn, slot: *slot}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024)
	for scanner.Scan() {
		target := scanner.Text()
		err = f.transition(target)
		message := ""
		if err != nil {
			message = err.Error()
		}
		encoded, _ := json.Marshal(map[string]any{"ok": err == nil, "mode": f.mode, "error": message})
		fmt.Println(string(encoded))
	}
	// EOF cannot silently disable active NAT objects; only an empty plugin we enabled is restored.
	if f.mode != "" {
		if err = f.transition("off"); err != nil {
			fmt.Fprintln(os.Stderr, "fixture retained:", err)
			os.Exit(1)
		}
	}
	if err = scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
