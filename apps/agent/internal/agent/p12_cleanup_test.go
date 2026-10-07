package agent

import (
	"context"
	"fmt"
	"testing"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// A failed or missing read is not affirmative evidence of an empty inventory.
// These helpers are used by the live cleanup and exercised without a lab.
func p12CleanupVPPInventory(response *ngfwv1.ListRoutesResponse, readErr error) error {
	if readErr != nil {
		return fmt.Errorf("route read failed: %w", readErr)
	}
	if response == nil {
		return fmt.Errorf("route read returned no inventory")
	}
	if response.GetTotal() != 0 || len(response.GetRoutes()) != 0 {
		return fmt.Errorf("route read found residue: total=%d rows=%d", response.GetTotal(), len(response.GetRoutes()))
	}
	return nil
}

func p12CleanupKernelInventory(output string, readErr error) error {
	if readErr != nil {
		return fmt.Errorf("kernel route read failed: %w", readErr)
	}
	if countLines(output) != 0 {
		return fmt.Errorf("kernel BGP routes remain: %s", output)
	}
	return nil
}

func TestP12CleanupInventoryRequiresSuccessfulRead(t *testing.T) {
	t.Run("VPP", func(t *testing.T) {
		cases := []struct {
			name     string
			response *ngfwv1.ListRoutesResponse
			readErr  error
			wantErr  bool
		}{
			{"empty successful inventory", &ngfwv1.ListRoutesResponse{}, nil, false},
			{"failed read with empty response", &ngfwv1.ListRoutesResponse{}, context.DeadlineExceeded, true},
			{"failed read without response", nil, context.Canceled, true},
			{"missing inventory without error", nil, nil, true},
			{"retained route count", &ngfwv1.ListRoutesResponse{Total: 1}, nil, true},
			{"retained row despite zero total", &ngfwv1.ListRoutesResponse{Routes: []*ngfwv1.ListRoutesEntry{{}}}, nil, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if err := p12CleanupVPPInventory(tc.response, tc.readErr); (err != nil) != tc.wantErr {
					t.Fatalf("inventory verdict = %v; want rejection %v", err, tc.wantErr)
				}
			})
		}
	})
	t.Run("kernel", func(t *testing.T) {
		cases := []struct {
			name    string
			output  string
			readErr error
			wantErr bool
		}{
			{"empty successful inventory", "", nil, false},
			{"empty output from failed command", "", context.DeadlineExceeded, true},
			{"partial output from failed command", "10.14.0.0/16 proto bgp\n", context.Canceled, true},
			{"successful read with residue", "10.14.0.0/16 proto bgp\n", nil, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if err := p12CleanupKernelInventory(tc.output, tc.readErr); (err != nil) != tc.wantErr {
					t.Fatalf("inventory verdict = %v; want rejection %v", err, tc.wantErr)
				}
			})
		}
	})
}
