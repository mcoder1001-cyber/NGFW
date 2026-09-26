package rsyslog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

// F-unbound-chrony-syslog: the singleton descriptor and the DeferredController of a slot instance.

func slotPaths(t *testing.T) (Paths, *DeferredController) {
	t.Helper()
	dir := t.TempDir()
	p := PathsUnder(dir, 0)
	// the stand-in "instance" is /usr/bin/sleep (Binary), never a real rsyslogd
	return p, &DeferredController{PendingFile: filepath.Join(dir, "vrx.pending"), PIDFile: filepath.Join(dir, "rsyslogd.pid"), Binary: "/usr/bin/sleep"}
}

func typed(targets ...*vrxv1.SyslogTarget) *vrxv1.ManagementConfig {
	return &vrxv1.ManagementConfig{Syslog: targets}
}

func retrieveOne(t *testing.T, d *Descriptor) proto.Message {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(kvs) > 1 || (len(kvs) == 1 && kvs[0].Key != Key) {
		t.Fatalf("retrieve %v", kvs)
	}
	if len(kvs) == 0 {
		return nil
	}
	return kvs[0].Value
}

func TestDescriptorDeferred(t *testing.T) {
	ctx := context.Background()
	p, dc := slotPaths(t)
	rr := renderers.NewRecordingRunner().Succeed(RsyslogdBin, "")
	d := NewDescriptor(New(rr, WithPaths(p), WithController(dc)), nil)
	if d.Name() != Name || Key != "rsyslog.config/vrx" || d.KeyOf(nil) != Key || d.Dependencies(nil) != nil {
		t.Fatal("identity")
	}
	in := typed(&vrxv1.SyslogTarget{
		Address: proto.String("127.0.0.1"), Port: proto.Uint32(31016), Protocol: proto.String("tcp"), Severity: proto.String("notice"),
		Facilities: []string{"local7", "daemon"}, Format: proto.String("rfc5424"), QueueSize: proto.Uint32(2000),
	})
	// no instance running: the file is written, a start is pending (not an error, no restart attempted)
	if _, err := d.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	for _, c := range rr.Calls() {
		if c.Path == rfkit.SystemctlBin {
			t.Fatalf("a slot agent ran systemctl: %v", c)
		}
	}
	conf, _ := os.ReadFile(p.ConfFile)
	for _, w := range []string{`if prifilt("daemon,local7.notice")`, `queue.size="2000"`, `port="31016" protocol="tcp" TCP_Framing="octet-counted"`, inputPrefix} {
		if !strings.Contains(string(conf), w) {
			t.Fatalf("missing %q:\n%s", w, conf)
		}
	}
	if got := retrieveOne(t, d); !proto.Equal(got, in) {
		t.Fatalf("retrieve:\n got %v\nwant %v", got, in)
	}
	if pend := d.Pending(ctx); len(pend) != 1 || pend[0].Action != "start" {
		t.Fatalf("pending %v", pend)
	}
	// an instance runs (pidfile): a changed export → a persisted restart request, returned until it restarted
	old := startRsyslogStandIn(t)
	writePidFile(t, dc.PIDFile, old)
	in2 := typed(&vrxv1.SyslogTarget{Address: proto.String("127.0.0.1"), Port: proto.Uint32(31015)})
	if _, err := d.Update(ctx, in, in2, nil); err != nil {
		t.Fatal(err)
	}
	pend := d.Pending(ctx)
	if len(pend) != 1 || pend[0].Action != "restart" {
		t.Fatalf("pending %v", pend)
	}
	var ar *rfkit.ActionRequired
	if err := d.r.Apply(ctx, mustRender(t, d.r, in2)); !errors.As(err, &ar) || !strings.Contains(ar.Reason, "still pending") {
		t.Fatalf("unchanged re-apply before the restart: %v", err)
	}
	writePidFile(t, dc.PIDFile, startRsyslogStandIn(t))
	if pend := d.Pending(ctx); len(pend) != 0 {
		t.Fatalf("pending after the restart: %v", pend)
	}
	// drift and delete
	if err := os.WriteFile(p.ConfFile, append(conf, "# edited\n"...), 0o644); err != nil { //nolint:gosec // test
		t.Fatal(err)
	}
	if _, ok := retrieveOne(t, d).(*structpb.Struct); !ok {
		t.Fatal("hand edit not reported as drift")
	}
	if err := d.Delete(ctx, in2, nil); err != nil {
		t.Fatal(err)
	}
	if v := retrieveOne(t, d); v != nil {
		t.Fatalf("after delete: %v", v)
	}
	if err := dc.Signal(ctx, 1); err == nil {
		t.Fatal("the deferred controller signalled a process")
	}
}

func mustRender(t *testing.T, r *Renderer, in proto.Message) renderers.Files {
	t.Helper()
	files, err := r.Render(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// startRsyslogStandIn starts the stand-in instance (its start time is after any request made before).
func startRsyslogStandIn(t *testing.T) int {
	t.Helper()
	time.Sleep(30 * time.Millisecond) // start times have 10 ms resolution
	cmd := exec.Command("/usr/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd.Process.Pid
}

func writePidFile(t *testing.T, path string, pid int) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDeferredControllerRefusesAForeignPid(t *testing.T) {
	_, dc := slotPaths(t)
	dc.Binary = "" // rsyslogd
	writePidFile(t, dc.PIDFile, startRsyslogStandIn(t))
	if dc.PID() != 0 {
		t.Fatal("a pid running another binary was taken for the rsyslog instance")
	}
	var ar *rfkit.ActionRequired
	if err := dc.Restart(context.Background()); !errors.As(err, &ar) || ar.Action != "start" {
		t.Fatalf("restart of a stopped instance: %v", err)
	}
}
