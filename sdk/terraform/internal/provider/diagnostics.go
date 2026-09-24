package provider

import (
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"ngfw/sdk/terraform/internal/client"
)

// addAPIError turns an error into diagnostics; a problem+json keeps its pointers (one line per offending field).
func addAPIError(d *diag.Diagnostics, what string, err error) {
	var ae *client.APIError
	if errors.As(err, &ae) {
		detail := fmt.Sprintf("%s → HTTP %d %s", what, ae.Status, ae.Title)
		if ae.Detail != "" {
			detail += ": " + ae.Detail
		}
		for _, fe := range ae.Errors {
			detail += fmt.Sprintf("\n  %s: %s", orRoot(fe.Pointer), fe.Message)
		}
		if len(ae.Lock) > 0 && string(ae.Lock) != "null" {
			detail += "\n  candidate lock: " + string(ae.Lock)
		}
		d.AddError(fmt.Sprintf("VRX API refused to %s", what), detail)
		return
	}
	d.AddError(fmt.Sprintf("VRX: %s failed", what), err.Error())
}

func orRoot(p string) string {
	if p == "" {
		return "/"
	}
	return p
}
