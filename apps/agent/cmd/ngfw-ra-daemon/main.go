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
	if len(os.Args) != 2 || !ravpn.ValidInstance(os.Args[1]) {
		return ravpn.ErrBoundary
	}
	instance := os.Args[1]
	if err := ravpn.ConfigureNamespace(context.Background(), instance); err != nil {
		return err
	}
	config := filepath.Join(ravpn.InstanceRoot, instance, "strongswan.conf")
	if err := ravpn.ValidatePrivateFile(config, 1<<20); err != nil {
		return err
	}
	// No environment inheritance, shell, user executable, plugin or config path.
	return syscall.Exec("/opt/ngfw-ra/libexec/ipsec/charon", []string{"charon"}, []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "STRONGSWAN_CONF=" + config})
}
func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "remote-access: isolated daemon startup refused")
		os.Exit(1)
	}
}
