// ngfw-ra-daemon is invoked only by the dedicated network-namespace unit.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	ravpn "ngfw/agent/internal/ra_vpn"
)

func run() error {
	return runWith(ravpn.ConfigureNamespace, ravpn.ValidatePrivateFile, syscall.Exec)
}

func runWith(configure func(context.Context, string) error, validate func(string, int64) error, execute func(string, []string, []string) error) error {
	if len(os.Args) != 2 || !ravpn.ValidInstance(os.Args[1]) {
		return ravpn.ErrBoundary
	}
	instance := os.Args[1]
	if err := configure(context.Background(), instance); err != nil {
		return err
	}
	config := filepath.Join(ravpn.InstanceRoot, instance, "strongswan.conf")
	if err := validate(config, 1<<20); err != nil {
		return err
	}
	// No environment inheritance, shell, user executable, plugin or config path.
	return execute("/opt/ngfw-ra/sbin/charon-systemd", []string{"charon-systemd"}, []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "STRONGSWAN_CONF=" + config})
}
func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "remote-access: isolated daemon startup refused")
		os.Exit(1)
	}
}
