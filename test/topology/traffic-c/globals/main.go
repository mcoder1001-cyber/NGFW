// Lease-gated runner passes its already exclusive globals lock descriptor.
// Snapshot and exact restoration only; product API configures the scenario.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.fd.io/govpp"
	"ngfw/agent/binapi/flowprobe"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/binapi/mpls"
	"ngfw/agent/binapi/sr"
)

type saved struct {
	TableZero bool                              `json:"tableZero"`
	Exporter  ipfix_export.IpfixExporterDetails `json:"exporter"`
	Params    flowprobe.FlowprobeGetParamsReply `json:"params"`
	Source    string                            `json:"source"`
	HopLimit  string                            `json:"hopLimit"`
	MPLS      string                            `json:"mpls"`
}

func cli(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "timeout", append([]string{"10", "vppctl"}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("VPP read/restore failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
func source() (string, string, error) {
	s, e := cli("show", "sr", "encaps", "source", "addr")
	if e != nil {
		return "", "", e
	}
	h, e := cli("show", "sr", "encaps", "hop-limit")
	if e != nil {
		return "", "", e
	}
	a := regexp.MustCompile(`= (\S+)`).FindStringSubmatch(s)
	b := regexp.MustCompile(`= (\d+)`).FindStringSubmatch(h)
	if a == nil || b == nil {
		return "", "", errors.New("cannot parse exact SR globals")
	}
	return a[1], b[1], nil
}
func run(operation, path string) error {
	if os.Getenv("NGFW_TRAFFIC_C_GLOBALS") != "1" {
		return errors.New("requires leased globals runner")
	}
	n, err := strconv.Atoi(os.Getenv("NGFW_GLOBAL_LOCK_FD"))
	if err != nil || n < 3 {
		return errors.New("missing inherited lock descriptor")
	}
	f := os.NewFile(uintptr(n), "globals-lock")
	if f == nil {
		return errors.New("invalid lock descriptor")
	}
	defer f.Close()
	a, err := f.Stat()
	if err != nil {
		return err
	}
	b, err := os.Stat("/run/lock/ngfw-globals.lock")
	if err != nil || !os.SameFile(a, b) {
		return errors.New("foreign lock descriptor")
	}
	if err = syscall.Flock(n, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	conn, err := govpp.Connect("/run/vpp/api.sock")
	if err != nil {
		return err
	}
	defer conn.Disconnect()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	read := func() (saved, error) {
		var s saved
		stream, e := ipfix_export.NewServiceClient(conn).IpfixExporterDump(ctx, &ipfix_export.IpfixExporterDump{})
		if e != nil {
			return s, e
		}
		detail, e := stream.Recv()
		if e != nil {
			return s, e
		}
		s.Exporter = *detail
		if _, e = stream.Recv(); !errors.Is(e, io.EOF) {
			return s, errors.New("exporter dump did not contain exactly exporter zero")
		}
		p, e := flowprobe.NewServiceClient(conn).FlowprobeGetParams(ctx, &flowprobe.FlowprobeGetParams{})
		if e != nil {
			return s, e
		}
		s.Params = *p
		s.Source, s.HopLimit, e = source()
		if e != nil {
			return s, e
		}
		tables, e := mpls.NewServiceClient(conn).MplsTableDump(ctx, &mpls.MplsTableDump{})
		if e != nil {
			return s, e
		}
		for {
			entry, e := tables.Recv()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return s, e
			}
			if entry.MtTable.MtTableID == 0 {
				s.TableZero = true
			}
		}
		s.MPLS, e = cli("show", "mpls", "fib", "table", "0")
		return s, e
	}
	if operation == "snapshot" {
		bindings, e := flowprobe.NewServiceClient(conn).FlowprobeInterfaceDump(ctx, &flowprobe.FlowprobeInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
		if e != nil {
			return e
		}
		if _, e = bindings.Recv(); !errors.Is(e, io.EOF) {
			return errors.New("existing flowprobe binding prevents globals ownership")
		}

		s, e := read()
		if e != nil {
			return e
		}
		data, e := json.MarshalIndent(s, "", "  ")
		if e != nil {
			return e
		}
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		defer file.Close()
		if _, e = file.Write(data); e != nil {
			return e
		}
		return file.Sync()
	}
	if operation != "restore" {
		return errors.New("operation must be snapshot or restore")
	}
	file, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return e
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 65536 {
		return errors.New("snapshot must be private bounded regular file")
	}
	var old saved
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&old); e != nil {
		return e
	}
	var extra any
	if e = decoder.Decode(&extra); !errors.Is(e, io.EOF) {
		return errors.New("trailing snapshot data")
	}
	export := old.Exporter
	if _, e = ipfix_export.NewServiceClient(conn).SetIpfixExporter(ctx, &ipfix_export.SetIpfixExporter{CollectorAddress: export.CollectorAddress, CollectorPort: export.CollectorPort, SrcAddress: export.SrcAddress, VrfID: export.VrfID, PathMtu: export.PathMtu, TemplateInterval: export.TemplateInterval, UDPChecksum: export.UDPChecksum}); e != nil {
		return e
	}
	if _, e = flowprobe.NewServiceClient(conn).FlowprobeSetParams(ctx, &flowprobe.FlowprobeSetParams{RecordFlags: old.Params.RecordFlags, ActiveTimer: old.Params.ActiveTimer, PassiveTimer: old.Params.PassiveTimer}); e != nil {
		return e
	}
	ip, e := ip_types.ParseIP6Address(old.Source)
	if e != nil {
		return e
	}
	hop, e := strconv.ParseUint(old.HopLimit, 10, 8)
	if e != nil {
		return e
	}
	if _, e = sr.NewServiceClient(conn).SrSetEncapSource(ctx, &sr.SrSetEncapSource{EncapsSource: ip}); e != nil {
		return e
	}
	if _, e = sr.NewServiceClient(conn).SrSetEncapHopLimit(ctx, &sr.SrSetEncapHopLimit{HopLimit: uint8(hop)}); e != nil {
		return e
	}

	after, e := read()
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(after, old) {
		return errors.New("global snapshot differs after exact restore")
	}
	report, e := json.Marshal(map[string]any{"status": "GLOBALS_EXACTLY_RESTORED", "before": old, "restored": after})
	if e != nil {
		return e
	}
	fmt.Println(string(report))
	return nil
}
func main() {
	op := flag.String("operation", "snapshot", "snapshot or restore")
	path := flag.String("snapshot", "", "owned private snapshot path")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "snapshot path required")
		os.Exit(1)
	}
	if err := run(*op, *path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
