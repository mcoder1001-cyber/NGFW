// ngfw-ra-namespace-broker performs one fixed root-only NSFS operation.
package main

import (
	"fmt"
	ravpn "ngfw/agent/internal/ra_vpn"
	"os"
)

func main() {
	var err error
	if len(os.Args) == 2 {
		err = ravpn.RunManagedNamespaceBroker(os.Args[1])
	} else {
		err = ravpn.RunNamespaceBroker(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "remote-access: namespace handoff refused")
		os.Exit(1)
	}
}
