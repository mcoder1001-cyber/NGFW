// ngfw-ra-namespace-broker performs one fixed root-only NSFS operation.
package main

import (
	"fmt"
	ravpn "ngfw/agent/internal/ra_vpn"
	"os"
)

func main() {
	var err error
	if len(os.Args) == 3 && os.Args[1] == "--observe-unit" {
		err = ravpn.RunUnitObservationProvider(os.Args[2])
	} else if len(os.Args) == 3 && os.Args[1] == "--targets" {
		err = ravpn.RunNamespaceTargetProvider(os.Args[2])
	} else if len(os.Args) != 1 {
		err = ravpn.ErrBoundary
	} else {
		err = ravpn.RunManagedNamespaceBroker(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "remote-access: namespace handoff refused")
		os.Exit(1)
	}
}
