// Package main checks the real agent Health RPC and the API liveness endpoint.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "ngfw/agent/gen/ngfw/v1"
)

func check(ctx context.Context, socket, endpoint string) error {
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	health, err := pb.NewDataplaneClient(conn).Health(ctx, &pb.HealthRequest{})
	if err != nil {
		return err
	}
	if !health.GetVppConnected() || health.GetDegraded() || health.GetReconcileInProgress() {
		return fmt.Errorf("agent or VPP unhealthy")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var body struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("API unhealthy")
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 65536)).Decode(&body); err != nil {
		return err
	}
	if body.Status != "ok" || body.Service != "ngfw-api" {
		return fmt.Errorf("invalid API health response")
	}
	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := check(ctx, "/run/ngfw/agent.sock", "http://127.0.0.1:3000/api/v1/health"); err != nil {
		// Connection errors can contain server diagnostics; print no input or secrets.
		fmt.Fprintln(os.Stderr, "NGFW upgrade health probe failed")
		os.Exit(1)
	}
}
