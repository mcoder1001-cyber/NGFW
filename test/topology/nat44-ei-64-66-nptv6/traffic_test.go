package nat44ei6466nptv6

// Traffic of the packet tests: small Python programs run inside the rig's namespaces (python3 is on the host; no
// package is installed), started and stopped by PID, and tcpdump in a namespace. Test code only; no user input.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// holdServer accepts TCP connections and holds them open (a fixed 5-tuple stays a live NAT session).
const holdServer = `import socket, sys, time
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind((sys.argv[1], int(sys.argv[2])))
s.listen(64)
held = []
while True:
    c, _ = s.accept()
    try:
        c.sendall(b"vrx-nat-ok\n")
    except OSError:
        pass
    held.append(c)
`

// holdClient connects from a fixed source port, reads the server's greeting and holds the connection.
const holdClient = `import socket, sys, time
c = socket.socket()
c.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
c.bind((sys.argv[1], int(sys.argv[2])))
c.settimeout(5)
c.connect((sys.argv[3], int(sys.argv[4])))
print(c.recv(64).decode().strip(), flush=True)
time.sleep(float(sys.argv[5]) if len(sys.argv) > 5 else 600)
`

// connectOnce connects from a fixed source port, prints the greeting (proof the far end answered) and exits.
const connectOnce = `import socket, sys
c = socket.socket()
c.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
c.bind((sys.argv[1], int(sys.argv[2])))
c.settimeout(5)
c.connect((sys.argv[3], int(sys.argv[4])))
print(c.recv(64).decode().strip())
c.close()
`

// udpFlows sends one datagram from each of n source ports (one NAT session each).
const udpFlows = `import socket, sys
src, base, n, dst, dport = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4], int(sys.argv[5])
socks = []
for i in range(n):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind((src, base + i))
    s.sendto(b"vrx-nat-flow", (dst, dport))
    s.close()
print("sent", n)
`

// script writes a helper program into dir and returns its path.
func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// inNSProc starts a long-running program inside a namespace (`ip netns exec` execs it: the PID is the program's)
// and stops it by PID in Cleanup.
func inNSProc(t *testing.T, name, logDir, ns string, args ...string) *proc {
	t.Helper()
	p := start(t, name, filepath.Join(logDir, name+".log"), []string{"PATH=" + os.Getenv("PATH")}, "ip", append([]string{"netns", "exec", ns}, args...)...)
	t.Cleanup(func() { p.stop(t) })
	return p
}

// capture is a tcpdump in a namespace, line-buffered into a log.
type capture struct {
	p   *proc
	log string
}

func startCapture(t *testing.T, name, logDir, ns, dev string, filter ...string) *capture {
	t.Helper()
	args := append([]string{"tcpdump", "-n", "-l", "-i", dev}, filter...)
	c := &capture{log: filepath.Join(logDir, name+".log")}
	c.p = inNSProc(t, name, logDir, ns, args...)
	time.Sleep(700 * time.Millisecond) // "listening on …" before the first packet
	return c
}

// lines stops the capture and returns its packet lines.
func (c *capture) lines(t *testing.T) []string {
	t.Helper()
	time.Sleep(300 * time.Millisecond)
	c.p.stop(t)
	raw, _ := os.ReadFile(c.log) //nolint:gosec // our own log
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.Contains(l, " IP ") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

var tcpdumpIP = regexp.MustCompile(` IP (\d+\.\d+\.\d+\.\d+)\.(\d+) > (\d+\.\d+\.\d+\.\d+)\.(\d+):`)

// srcOf returns the source address and port of a tcpdump line ("", "" when it has none).
func srcOf(line string) (string, string) {
	m := tcpdumpIP.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	return m[1], m[2]
}
