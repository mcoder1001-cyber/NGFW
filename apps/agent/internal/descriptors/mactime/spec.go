// Package mactime holds the F-bridge-l2 descriptors of VPP's mactime plugin (time-range source-MAC
// filter), built on descriptors/dfkit (D-077):
//
//	mactime.range/<name>        one device of the VPP-wide device table (mactime_add_del_range):
//	                            MAC, allow/drop, weekly ranges; VPP device_name "<owner>:<name>"
//	                            (the table is shared by every owner: only owner-prefixed devices are
//	                            ever touched, D-071); Retrieve decodes mactime_dump
//	mactime.enable/<interface>  the filter on one hardware interface (mactime_enable_disable);
//	                            read back with feature_is_enabled("device-input", "mactime"), and
//	                            reported only with the applied-once record of this VPP boot
//	                            (D-076/D-080, keyed on sw_if_index and logical name)
//
// See docs/agent/descriptors/mactime.md.
package mactime

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
)

// Descriptor names.
const (
	RangeName  = "mactime.range"
	EnableName = "mactime.enable"
)

// VPP facts (src/plugins/mactime): the filter is the "mactime" node on the device-input arc (and
// "mactime-tx" on interface-output); device flags; a week in seconds.
const (
	arcDeviceInput = "device-input"
	featureMactime = "mactime"

	flagStaticDrop   = 1 << 0
	flagStaticAllow  = 1 << 1
	flagDynamicDrop  = 1 << 2
	flagDynamicAllow = 1 << 3

	// Week is the length of VPP's range clock: seconds since Sunday 00:00.
	Week = 7 * 86400
	// maxDeviceName is the usable length of mactime's device_name[64].
	maxDeviceName = 63
)

// Range is one time range in seconds since Sunday 00:00 of VPP's mactime clock.
type Range struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Device is the desired value of mactime.range/<name> (dfkit structpb stand-in, D-055).
type Device struct {
	// Name is the record name; VPP stores "<owner>:<name>".
	Name string `json:"name"`
	// MAC is the source MAC, lower-case "aa:bb:cc:dd:ee:ff".
	MAC string `json:"mac"`
	// Drop: false = allow (only inside the ranges, when there are any), true = drop (only inside).
	Drop bool `json:"drop"`
	// Ranges sorted by (start, end); empty = the action is static.
	Ranges []Range `json:"ranges"`
}

// Enable is the desired value of mactime.enable/<interface>.
type Enable struct {
	// Interface is the logical interface name (D-069).
	Interface string `json:"interface"`
}

// Canon sorts the ranges (Retrieve reports them sorted) and lower-cases the MAC.
func (d Device) Canon() Device {
	out := d
	out.Ranges = append([]Range(nil), d.Ranges...)
	if out.Ranges == nil {
		out.Ranges = []Range{}
	}
	sort.Slice(out.Ranges, func(i, j int) bool {
		if out.Ranges[i].Start != out.Ranges[j].Start {
			return out.Ranges[i].Start < out.Ranges[j].Start
		}
		return out.Ranges[i].End < out.Ranges[j].End
	})
	if m, err := parseMAC(d.MAC); err == nil {
		out.MAC = formatMAC(m)
	}
	return out
}

func (d Device) validate() error {
	if d.Name == "" {
		return dfkit.Specf("mactime device: empty name")
	}
	if _, err := parseMAC(d.MAC); err != nil {
		return dfkit.Specf("mactime device %s: %v", d.Name, err)
	}
	for _, r := range d.Ranges {
		if r.Start < 0 || r.End > Week || r.Start >= r.End {
			return dfkit.Specf("mactime device %s: range %v–%v outside 0 ≤ start < end ≤ %d", d.Name, r.Start, r.End, Week)
		}
	}
	return nil
}

// Proto encodes a device as the scheduler value.
func (d Device) Proto() proto.Message { return dfkit.Encode(d.Canon()) }

// Proto encodes an enable as the scheduler value.
func (e Enable) Proto() proto.Message { return dfkit.Encode(e) }

// RangeKey is the key of device name.
func RangeKey(name string) scheduler.Key { return scheduler.Join(RangeName, name) }

// EnableKey is the key of the filter on interface name.
func EnableKey(name string) scheduler.Key { return scheduler.Join(EnableName, name) }

func decodeDevice(m proto.Message) (Device, error) {
	var d Device
	if err := dfkit.Decode(m, &d); err != nil {
		return Device{}, err
	}
	return d, nil
}

func decodeEnable(m proto.Message) (Enable, error) {
	var e Enable
	if err := dfkit.Decode(m, &e); err != nil {
		return Enable{}, err
	}
	if e.Interface == "" {
		return Enable{}, dfkit.Specf("mactime enable: empty interface")
	}
	return e, nil
}

func parseMAC(s string) ([6]byte, error) {
	var m [6]byte
	if _, err := fmt.Sscanf(s, "%02x:%02x:%02x:%02x:%02x:%02x", &m[0], &m[1], &m[2], &m[3], &m[4], &m[5]); err != nil || len(s) != 17 {
		return m, fmt.Errorf("invalid MAC %q", s)
	}
	return m, nil
}

func formatMAC(m [6]byte) string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", m[0], m[1], m[2], m[3], m[4], m[5])
}
