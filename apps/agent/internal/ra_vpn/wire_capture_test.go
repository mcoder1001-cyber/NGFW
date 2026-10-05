package ravpn

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// The bounded capture never prints or persists packet bytes. It opens AF_PACKET
// only after proving the exact owned private namespace differs from its host.
const privateWireCapture = `import json,os,socket,struct,sys,time
private,host=int(sys.argv[1]),int(sys.argv[2])
if os.stat('/proc/self/ns/net').st_ino!=private or private==host:
    raise SystemExit(23)
s=socket.socket(socket.AF_PACKET,socket.SOCK_RAW,socket.htons(3))
s.bind(('outer0',0));s.settimeout(.2)
counts={'esp_rx':0,'esp_tx':0,'plaintext_icmp':0,'frames':0}
print('READY',flush=True)
end=time.monotonic()+9
while time.monotonic()<end:
    try: frame,address=s.recvfrom(65536)
    except socket.timeout: continue
    counts['frames']+=1
    if counts['frames']>4096: raise SystemExit(24)
    if len(frame)<34 or frame[12:14]!=b'\x08\x00': continue
    ip=frame[14:];ihl=(ip[0]&15)*4
    if ihl<20 or len(ip)<ihl: raise SystemExit(25)
    proto=ip[9];esp=proto==50
    if proto==17 and len(ip)>=ihl+12:
        src,dst=struct.unpack('!HH',ip[ihl:ihl+4])
        esp=(src==4500 or dst==4500) and ip[ihl+8:ihl+12]!=b'\0\0\0\0'
    if proto==1: counts['plaintext_icmp']+=1
    if esp: counts['esp_tx' if address[2]==socket.PACKET_OUTGOING else 'esp_rx']+=1
s.close()
print(json.dumps(counts),flush=True)
`

func capturePrivateWire(t *testing.T, plan *NetworkPlan) func() {
	t.Helper()
	verified, e := ReadAgentPlan(plan.Instance)
	if e != nil || verified.NamespaceInode != plan.NamespaceInode {
		t.Fatal("private capture boundary")
	}
	ns := filepath.Join(InstanceRoot, plan.Instance, "netns")
	// #nosec G204 -- fixed capture program enters only the current authenticated owned plan namespace; all extra arguments are canonical inode numbers.
	cmd := exec.Command("/usr/bin/nsenter", "--net="+ns, "--", "/usr/bin/python3", "-c", privateWireCapture, strconv.FormatUint(plan.NamespaceInode, 10), strconv.FormatUint(plan.HostNamespaceInode, 10))
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal("private capture pipe")
	}
	cmd.Stderr = nil
	if cmd.Start() != nil {
		t.Fatal("private capture launch")
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error("owned private capture termination failed")
			}
			var exitError *exec.ExitError
			if err := cmd.Wait(); err != nil && !errors.As(err, &exitError) {
				t.Error("owned private capture reap failed")
			}
		}
	})
	reader := bufio.NewReaderSize(stdout, 4096)
	ready, e := reader.ReadString('\n')
	if e != nil || ready != "READY\n" {
		t.Fatal("private capture namespace guard refused")
	}
	return func() {
		var counters struct {
			RX     uint64 `json:"esp_rx"`
			TX     uint64 `json:"esp_tx"`
			Plain  uint64 `json:"plaintext_icmp"`
			Frames uint64 `json:"frames"`
		}
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&counters) != nil || cmd.Wait() != nil || counters.RX == 0 || counters.TX == 0 || counters.Plain != 0 || counters.Frames > 4096 {
			t.Fatal("private underlay ESP-only/no-plaintext proof failed")
		}
		evidence := os.Getenv("NGFW_RA_EVIDENCE_ROOT")
		if evidence != "" {
			data, _ := json.Marshal(counters)
			if _, err := writePrivateFixtureEvidence(evidence, plan.Instance, data); err != nil {
				t.Fatal("private wire receipt publication refused")
			}

		}
		t.Logf("owned underlay ESP-only proof RX=%d TX=%d plaintextICMP=0", counters.RX, counters.TX)
	}
}
