// Package trafficbtest contains opt-in topology-fixture REST control only.
package trafficbtest

import (
	"context"
	"encoding/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

type Client struct {
	cmd        *exec.Cmd
	input      io.WriteCloser
	decoder    *json.Decoder
	once       sync.Once
	cancel     context.CancelFunc
	configured bool
}
type Reply struct {
	Status   string          `json:"status"`
	Response json.RawMessage `json:"response"`
	Error    string          `json:"error"`
}

func Enabled() bool { return os.Getenv("NGFW_TRAFFIC_B_REST") == "1" }
func New(t *testing.T, socket, owner, phase string, expectedPID ...int) *Client {
	t.Helper()
	if !Enabled() || os.Getenv("NGFW_DISPOSABLE_VPP") != "1" {
		t.Fatal("private REST fixture opt-in required")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test helper source unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
	slot, err := strconv.Atoi(os.Getenv("NGFW_TRAFFIC_B_SLOT"))
	if err != nil || slot < 1 || slot > 32 {
		t.Fatal("allocated physical slot required")
	}
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer checkCancel()
	connection, err := grpc.DialContext(checkCtx, "unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := ngfwv1.NewDataplaneClient(connection).Retrieve(checkCtx, &ngfwv1.RetrieveRequest{Owner: owner, Subsystems: []string{"interfaces"}}); err != nil {
		t.Fatal("REST attach owner verification: ", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	//nolint:gosec // G204: repository-owned fixed helper; generated private fixture socket/owner and allocated slot only.
	cmd := exec.CommandContext(ctx, "python3", filepath.Join(root, "test/topology/traffic-b/rest_bridge.py"), "--slot", strconv.Itoa(slot), "--agent-socket", socket, "--owner", owner, "--evidence", filepath.Join(os.Getenv("NGFW_TRAFFIC_B_EVIDENCE"), phase+"-rest.json"))
	pid := os.Getpid()
	if len(expectedPID) == 1 {
		pid = expectedPID[0]
	}
	cmd.Env = append(os.Environ(), "NGFW_TRAFFIC_VERIFIED_OWNER="+owner, "NGFW_TRAFFIC_EXPECTED_AGENT_PID="+strconv.Itoa(pid))
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 30 * time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(os.Getenv("NGFW_TRAFFIC_B_EVIDENCE"), 0o700); err != nil {
		t.Fatal(err)
	}
	diagnostic, err := os.OpenFile(filepath.Join(os.Getenv("NGFW_TRAFFIC_B_EVIDENCE"), phase+"-rest.private.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = diagnostic
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &Client{cmd: cmd, input: input, decoder: json.NewDecoder(io.LimitReader(output, 32<<20)), cancel: cancel}
	t.Cleanup(func() { c.Close(t); _ = diagnostic.Close() })
	var ready struct {
		Ready bool `json:"ready"`
	}
	if err := c.decoder.Decode(&ready); err != nil || !ready.Ready {
		cancel()
		t.Fatal("REST fixture startup failed; private diagnostics retained", err)
	}
	return c
}
func (c *Client) Apply(t *testing.T, txn string, desired *ngfwv1.DesiredState, secrets map[string][]byte) *ngfwv1.ApplyResponse {
	t.Helper()
	raw, err := protojson.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	request := struct {
		Txn     string            `json:"txn"`
		Desired json.RawMessage   `json:"desired"`
		Secrets map[string][]byte `json:"secrets,omitempty"`
	}{txn, raw, secrets}
	if err := json.NewEncoder(c.input).Encode(request); err != nil {
		t.Fatal("REST private input failed", err)
	}
	var reply Reply
	if err := c.decoder.Decode(&reply); err != nil {
		t.Fatal("REST response failed; private diagnostics retained", err)
	}
	if reply.Error != "" {
		t.Fatal("REST fixture refused", reply.Error)
	}
	if reply.Status != "applied" && reply.Status != "unchanged" {
		t.Fatal("REST configuration not applied", reply.Status)
	}
	if !c.configured && reply.Status != "applied" {
		t.Fatal("initial primary REST commit must apply")
	}
	c.configured = true
	result := new(ngfwv1.ApplyResponse)
	if err := protojson.Unmarshal(reply.Response, result); err != nil {
		t.Fatal("REST actual result conversion failed", err)
	}
	t.Logf("REST commit %s status=%s (actual object results=%d)", txn, reply.Status, len(result.GetResults()))
	return result
}
func (c *Client) Close(t *testing.T) {
	t.Helper()
	c.once.Do(func() {
		_ = json.NewEncoder(c.input).Encode(map[string]bool{"close": true})
		_ = c.input.Close()
		if err := c.cmd.Wait(); err != nil {
			t.Errorf("REST fixture rollback/cleanup failed: %v", err)
		}
		c.cancel()
	})
}
