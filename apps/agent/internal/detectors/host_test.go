package detectors

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func record(t *testing.T, id, msg, comm, uid, transport string, now time.Time) []byte {
	t.Helper()
	b, e := json.Marshal(Journal{Cursor: id, Message: msg, Comm: comm, UID: uid, Transport: transport, Timestamp: strconv.FormatInt(now.UnixMicro(), 10)})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestTrustedSSHAndReplay(t *testing.T) {
	now := time.Now()
	h := NewHost()
	msg := "Failed password for invalid user demo from 192.0.2.7 port 43000 ssh2"
	b := record(t, "1", msg, "sshd", "0", "journal", now)
	if ob := h.Observe(b, now, time.Minute, 3, 100); ob == nil || ob.Source != "192.0.2.7" || ob.Kind != "ssh" {
		t.Fatalf("SSH source: %v", ob)
	}
	if h.Observe(b, now, time.Minute, 3, 100) != nil {
		t.Fatal("cursor replay counted")
	}
	for i, j := range []Journal{{Comm: "sshd", UID: "1000"}, {Comm: "logger", UID: "0"}} {
		b := record(t, strconv.Itoa(i+2), msg, j.Comm, j.UID, "journal", now)
		if h.Observe(b, now, time.Minute, 3, 100) != nil {
			t.Fatal("untrusted syslog counted")
		}
	}
	if h.Observe(record(t, "4", msg, "sshd", "0", "journal", now.Add(-time.Minute)), now, time.Minute, 3, 100) != nil {
		t.Fatal("historical failure replayed")
	}
}
func TestCharonPacketSourceNotIdentity(t *testing.T) {
	now := time.Now()
	h := NewHost()
	packet := record(t, "1", "01[NET] <site|7> received packet: from 192.0.2.7[500] to 192.0.2.1[500]", "charon", "0", "journal", now)
	failure := record(t, "2", "01[IKE] <site|7> peer authentication failed for identity 198.51.100.9", "charon", "0", "journal", now)
	h.Observe(packet, now, time.Minute, 3, 100)
	ob := h.Observe(failure, now, time.Minute, 3, 100)
	if ob == nil || ob.Source != "192.0.2.7" {
		t.Fatalf("unsafe correlation: %v", ob)
	}
	if h.Observe(record(t, "3", "01[IKE] <other|8> EAP authentication failed for 198.51.100.9", "charon", "0", "journal", now), now, time.Minute, 3, 100) != nil {
		t.Fatal("identity without packet source accepted")
	}
}
func TestDistinctScanPortsAndExpiry(t *testing.T) {
	now := time.Now()
	h := NewHost()
	emit := func(id, port string, at time.Time) *Observation {
		return h.Observe(record(t, id, "ngfw:scan IN=eth0 SRC=192.0.2.7 DST=192.0.2.1 PROTO=TCP DPT="+port, "", "", "kernel", at), at, 10*time.Second, 3, 100)
	}
	if emit("1", "22", now) == nil || emit("2", "22", now) != nil || emit("3", "443", now) == nil || emit("4", "53", now) == nil {
		t.Fatal("ports not distinct")
	}
	if emit("5", "22", now.Add(11*time.Second)) == nil {
		t.Fatal("window did not expire")
	}
	if h.Observe(record(t, "6", "ngfw:scan SRC=192.0.2.7 DPT=25", "logger", "0", "journal", now), now, time.Minute, 3, 100) != nil {
		t.Fatal("spoofed kernel log accepted")
	}
}
