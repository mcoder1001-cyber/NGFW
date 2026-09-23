// Command vrx-keepalived-notify is the only notify target the keepalived renderer ever
// renders (notify_master / notify_backup / notify_fault / notify_stop). keepalived runs it with
// a fixed argv taken from the rendered config:
//
//	vrx-keepalived-notify <state-dir> INSTANCE|GROUP <name> MASTER|BACKUP|FAULT|STOP
//
// and it writes <state-dir>/<name>.state atomically as one JSON line
//
//	{"name":"vi1","type":"INSTANCE","state":"MASTER","time":"2026-09-24T01:40:00.123456789Z"}
//
// which the renderer's Retrieve and event poller read back. That file is the state and event
// channel between keepalived and the agent: no shell, no user-provided script text, no
// network. Every argument is validated; anything unexpected exits 2 without writing.
//
// Product path /usr/libexec/vrx/vrx-keepalived-notify (P10 packaging); tests build it into
// the slot's check directory.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var (
	nameRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)
	states  = map[string]bool{"MASTER": true, "BACKUP": true, "FAULT": true, "STOP": true}
	kinds   = map[string]bool{"INSTANCE": true, "GROUP": true}
	pathRe  = regexp.MustCompile(`^/[A-Za-z0-9_./-]{1,200}$`)
	usage   = "usage: vrx-keepalived-notify <state-dir> INSTANCE|GROUP <name> MASTER|BACKUP|FAULT|STOP [priority]"
	nowFunc = time.Now
)

// Record is the content of one <name>.state file.
type Record struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	State string `json:"state"`
	Time  string `json:"time"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vrx-keepalived-notify:", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	// keepalived appends the priority to the generic `notify` script only; accept and ignore it.
	if len(args) != 4 && len(args) != 5 {
		return fmt.Errorf("%s (got %d arguments)", usage, len(args))
	}
	dir, kind, name, state := args[0], args[1], args[2], args[3]
	switch {
	case !pathRe.MatchString(dir) || filepath.Clean(dir) != dir:
		return fmt.Errorf("state dir must be an absolute clean path")
	case !kinds[kind]:
		return fmt.Errorf("type must be INSTANCE or GROUP")
	case !nameRe.MatchString(name):
		return fmt.Errorf("name must match %s", nameRe)
	case !states[state]:
		return fmt.Errorf("state must be MASTER, BACKUP, FAULT or STOP")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() { //nolint:gosec // path validated (absolute, clean, [A-Za-z0-9_./-]) or test temp dir
		return fmt.Errorf("state dir %s is not a directory", dir)
	}
	line, err := json.Marshal(Record{Name: name, Type: kind, State: state, Time: nowFunc().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, name+".state"), append(line, '\n'))
}

func writeAtomic(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name) //nolint:gosec // path validated (absolute, clean, [A-Za-z0-9_./-]) or test temp dir
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name) //nolint:gosec // path validated (absolute, clean, [A-Za-z0-9_./-]) or test temp dir
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name) //nolint:gosec // path validated (absolute, clean, [A-Za-z0-9_./-]) or test temp dir
		return err
	}
	if err := os.Rename(name, path); err != nil { //nolint:gosec // path validated (absolute, clean, [A-Za-z0-9_./-]) or test temp dir
		_ = os.Remove(name) //nolint:gosec // path validated (absolute, clean, [A-Za-z0-9_./-]) or test temp dir
		return err
	}
	return nil
}
