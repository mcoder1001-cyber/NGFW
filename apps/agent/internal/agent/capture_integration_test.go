package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/socketclient"
	"go.fd.io/govpp/core"
	"ngfw/agent/internal/vpp/vpptest"

	interfaces "ngfw/agent/binapi/interface"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	capturetrace "ngfw/agent/internal/actions/capture-trace"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/descriptors/pcap"
)

// Exercises recovery against real VPP, including boot-bound ownership and durable
// metadata. It sends no traffic and is separate from packet acceptance on the rig.
func TestCaptureInterruptedRecoveryOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	socket := os.Getenv("NGFW_CAPTURE_DEDICATED_VPP_SOCKET")
	if socket == "" || socket == "/run/vpp/api.sock" {
		t.Fatal("capture requires NGFW_CAPTURE_DEDICATED_VPP_SOCKET for a dedicated per-slot VPP; shared VPP dispatch capture is banned")
	}
	vpptest.LockLab(t)
	conn, err := core.Connect(socketclient.NewVppClient(socket))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Disconnect)
	h := &dfkittest.Host{Conn: conn, Owner: vpptest.Prefix(t)}
	h.SkipUnlessCompatible(t, "pcap", &interfaces.PcapTraceOn{}, &interfaces.PcapTraceOff{})
	name, _ := h.Loopback(t, 88)
	dir := t.TempDir()
	bootPath := filepath.Join(dir, "boot.json")
	boot, err := dfkit.NewFileBootStore(bootPath)
	if err != nil {
		t.Fatal(err)
	}
	id := h.Owner + "-host-recovery"
	spec := pcap.Capture{Rx: true, Tx: true, Interface: name, MaxPackets: 10, MaxBytesPerPacket: 128, File: id + ".pcap"}.Proto()
	d := pcap.NewCapture(h.Client(), h.Owner, boot)
	t.Cleanup(func() {
		if err := d.Delete(context.Background(), spec, nil); err != nil {
			t.Error(err)
		}
		if err := os.Remove(filepath.Join("/tmp", id+".pcap")); err != nil && !os.IsNotExist(err) {
			t.Error(err)
		}
	})
	if _, err := d.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	record := capturetrace.Record{ID: id, State: "running", Interface: name, StartedAt: time.Now().UTC()}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	// Re-open disk state as a restarted agent, without restarting VPP.
	boot2, err := dfkit.NewFileBootStore(bootPath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := capturetrace.New(capturetrace.Config{Client: h.Client(), Owner: h.Owner, Dir: dir, Boot: boot2})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Captures) != 1 || list.Captures[0].State != "interrupted" || list.Captures[0].Reason != "agent-restart" {
		t.Fatalf("recovery metadata: %+v", list)
	}
	if _, ok := boot2.Get(string(pcap.KeyCapture)); ok {
		t.Fatal("capture boot ownership remained after stop")
	}
	if _, err := os.Lstat(filepath.Join("/tmp", id+".pcap")); !os.IsNotExist(err) {
		t.Fatalf("VPP temporary file remains: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(dir, id+".pcap")); err == nil {
		if fi.Mode().Perm() != 0600 {
			t.Fatalf("kept permissions: %o", fi.Mode().Perm())
		}
		if err := m.Read(id, func(*ngfwv1.CaptureChunk) error { return nil }); err != nil {
			t.Fatal(err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := m.Delete(id); err != nil {
		t.Fatal(err)
	}
	list, err = m.List(context.Background())
	if err != nil || len(list.Captures) != 0 {
		t.Fatalf("delete: %+v %v", list, err)
	}
	t.Log("same-boot restart recovered own capture; tmp clean; DELETE removed metadata")
}
