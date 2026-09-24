// Command terraform-provider-vrx is the Terraform provider of the VRX appliance (not published to any registry).
//
// Local use: build it (`go build -o terraform-provider-vrx`) and point Terraform at it with a dev_overrides block
// for `registry.terraform.io/vrx/vrx` (docs/user/system/sdk-terraform-ansible.md).
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"ngfw/sdk/terraform/internal/provider"
)

var version = "0.1.0-dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/vrx/vrx",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
