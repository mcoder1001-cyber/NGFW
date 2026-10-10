package main

import (
	"context"
	"fmt"
	"go.fd.io/govpp"
	"go.fd.io/govpp/core"
	"ngfw/agent/internal/descriptors/acl"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

type client struct{ *core.Connection }

func (c client) Connected() bool { return true }
func main() {
	info, e := os.Lstat("/run/vpp/startup.conf")
	if e != nil {
		panic(e)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || !info.Mode().IsRegular() {
		panic("private startup identity refused")
	}
	marker, e := os.ReadFile("/run/vpp/startup.conf")
	if e != nil {
		panic(e)
	}
	own, e1 := os.Readlink("/proc/self/ns/mnt")
	root, e2 := os.Readlink("/proc/1/ns/mnt")
	if os.Getenv("NGFW_DISPOSABLE_VPP") != "1" || !regexp.MustCompile(`(?m)^api-segment \{ prefix fulltest[0-9]+ \}$`).Match(marker) || e1 != nil || e2 != nil || own == root {
		panic("private marker/namespace refused")
	}
	pid := os.Getenv("NGFW_TRAFFIC_PRIVATE_VPP_PID")
	if !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(pid) {
		panic("private PID required")
	}
	ns, e := os.Readlink("/proc/" + pid + "/ns/mnt")
	if e != nil || ns != own {
		panic("VPP mount namespace mismatch")
	}
	exe, e := os.Readlink("/proc/" + pid + "/exe")
	if e != nil || filepath.Base(exe) != "vpp" {
		panic("private executable mismatch")
	}
	socket := os.Getenv("NGFW_VPP_API_SOCKET")
	if socket == "" || socket == "/run/vpp/api.sock" {
		panic("explicit private socket alias required")
	}
	a, e := os.Stat(socket)
	if e != nil {
		panic(e)
	}
	b, e := os.Stat("/run/vpp/api.sock")
	if e != nil || !os.SameFile(a, b) || a.Mode()&os.ModeSocket == 0 {
		panic("private socket mapping mismatch")
	}
	c, e := govpp.Connect("/run/vpp/api.sock")
	if e != nil {
		panic(e)
	}
	defer c.Disconnect()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e := acl.EnableCounters(ctx, client{c}); e != nil {
		panic(e)
	}
	fmt.Println("PRODUCT_COUNTERS_ENABLE_PRIVATE_ONLY_PASS")
}
