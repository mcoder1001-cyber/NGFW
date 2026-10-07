// ngfw-ra-daemon is invoked only by the dedicated network-namespace unit.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	ravpn "ngfw/agent/internal/ra_vpn"
)

func run() error {
	if len(os.Args) != 2 || !ravpn.ValidInstance(os.Args[1]) {
		return &startupError{"arguments", ravpn.ErrBoundary}
	}
	if ravpn.ValidateDaemonSandbox() != nil {
		return &startupError{"sandbox", ravpn.ErrBoundary}
	}
	return runWith(ravpn.ConfigureNamespace, ravpn.ValidatePrivateFile, syscall.Exec)
}

func runWith(configure func(context.Context, string) error, validate func(string, int64) error, execute func(string, []string, []string) error) error {
	if len(os.Args) != 2 || !ravpn.ValidInstance(os.Args[1]) {
		return ravpn.ErrBoundary
	}
	instance := os.Args[1]
	if err := configure(context.Background(), instance); err != nil {
		phase := "namespace"
		var failure *ravpn.NamespaceFailure
		if errors.As(err, &failure) {
			phase += "-" + strconv.Itoa(int(failure.Step))
		}
		return &startupError{phase, err}
	}
	config := filepath.Join(ravpn.InstanceRoot, instance, "strongswan.conf")
	if err := validate(config, 1<<20); err != nil {
		return &startupError{"config", err}
	}
	// No environment inheritance, shell, user executable, plugin or config path.
	if err := execute("/opt/ngfw-ra/sbin/charon-systemd", []string{"charon-systemd"}, []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "STRONGSWAN_CONF=" + config}); err != nil {
		return &startupError{"daemon", err}
	}
	return nil
}

type startupError struct {
	phase string
	cause error
}

func (e *startupError) Error() string {
	return "remote-access: isolated daemon startup refused (" + e.phase + ")"
}
func (e *startupError) Unwrap() error { return e.cause }
func main() {
	if err := run(); err != nil {
		if phase, ok := err.(*startupError); ok {
			fmt.Fprintln(os.Stderr, phase.Error())
		} else {
			fmt.Fprintln(os.Stderr, "remote-access: isolated daemon startup refused")
		}
		os.Exit(1)
	}
}
