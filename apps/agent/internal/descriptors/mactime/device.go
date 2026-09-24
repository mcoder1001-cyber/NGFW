package mactime

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ethernet_types"
	mactimeapi "ngfw/agent/binapi/mactime"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// DeviceDescriptor implements mactime.range/<name>: one device of VPP's mactime device table.
//
// VPP keys the table by MAC and an add for a MAC that exists APPENDS its ranges (mactime.c
// add_del_range: "add more ranges"), so an add is never repeated: Update is delete + add, and
// Create adopts nothing but VPP's own auto-entry for the MAC ("mac-<mac>", static allow, created
// by the data path for an unknown source MAC on a filtered interface), which it replaces. A MAC
// held by any other device name is another owner's and fails with dfkit.ErrNotOurs (D-071).
type DeviceDescriptor struct {
	client vpp.Client
	owner  string
}

var _ scheduler.Descriptor = (*DeviceDescriptor)(nil)

// NewDevice returns the mactime.range descriptor.
func NewDevice(c vpp.Client, owner string) *DeviceDescriptor {
	return &DeviceDescriptor{client: c, owner: owner}
}

// DeviceMeta is the runtime handle: the MAC (the table key).
type DeviceMeta struct{ MAC [6]byte }

// Name implements scheduler.Descriptor.
func (*DeviceDescriptor) Name() string { return RangeName }

// KeyOf implements scheduler.Descriptor.
func (*DeviceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	d, err := decodeDevice(obj)
	if err != nil {
		return scheduler.Join(RangeName, "invalid")
	}
	return RangeKey(d.Name)
}

// Dependencies implements scheduler.Descriptor: none (the table is global).
func (*DeviceDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Normalize implements scheduler.Normalizer: sorted ranges, lower-case MAC (Retrieve's form).
func (*DeviceDescriptor) Normalize(obj proto.Message) proto.Message {
	d, err := decodeDevice(obj)
	if err != nil {
		return obj
	}
	return d.Proto()
}

func (d *DeviceDescriptor) svc() mactimeapi.RPCService { return mactimeapi.NewServiceClient(d.client) }

// deviceName is the VPP device name of record name: "<owner>:<name>".
func (d *DeviceDescriptor) deviceName(name string) (string, error) {
	n, err := vpp.OwnerTag(d.owner, name)
	if err != nil {
		return "", err
	}
	if len(n) > maxDeviceName {
		return "", dfkit.Specf("mactime device name %q longer than %d bytes", n, maxDeviceName)
	}
	return n, nil
}

// Dump returns VPP's whole mactime device table. mactime_dump answers with details, then its own
// mactime_dump_reply, then (the generated stream client's) control_ping_reply: the generated
// MactimeDump client rejects the dump reply as an unexpected message, so the stream is read here.
func Dump(ctx context.Context, c vpp.Client) ([]*mactimeapi.MactimeDetails, error) {
	stream, err := c.NewStream(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	if err := stream.SendMsg(&mactimeapi.MactimeDump{}); err != nil {
		return nil, dfkit.PluginError("mactime", err)
	}
	if err := stream.SendMsg(&memclnt.ControlPing{}); err != nil {
		return nil, err
	}
	var out []*mactimeapi.MactimeDetails
	for {
		msg, err := stream.RecvMsg()
		if err != nil {
			return nil, dfkit.PluginError("mactime", fmt.Errorf("mactime_dump: %w", err))
		}
		switch m := msg.(type) {
		case *mactimeapi.MactimeDetails:
			out = append(out, m)
		case *mactimeapi.MactimeDumpReply:
			// retval NO_CHANGE only with a non-zero my_table_epoch; we always ask for the full table
		case *memclnt.ControlPingReply:
			return out, nil
		default:
			return nil, fmt.Errorf("mactime_dump: unexpected message %T", msg)
		}
	}
}

func trimName(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return s[:i]
		}
	}
	return s
}

// autoName is the name VPP gives a device it learns on a filtered interface.
func autoName(mac [6]byte) string { return "mac-" + formatMAC(mac) }

// find returns the table entry of mac (nil when absent).
func (d *DeviceDescriptor) find(ctx context.Context, mac [6]byte) (*mactimeapi.MactimeDetails, error) {
	all, err := Dump(ctx, d.client)
	if err != nil {
		return nil, err
	}
	for _, e := range all {
		if [6]byte(e.MacAddress) == mac {
			return e, nil
		}
	}
	return nil, nil
}

func (d *DeviceDescriptor) addDel(ctx context.Context, dev Device, mac [6]byte, add bool) error {
	req := &mactimeapi.MactimeAddDelRange{IsAdd: add, MacAddress: ethernet_types.MacAddress(mac)}
	if add {
		name, err := d.deviceName(dev.Name)
		if err != nil {
			return err
		}
		req.DeviceName = name
		req.Drop, req.Allow = dev.Drop, !dev.Drop
		for _, r := range dev.Ranges {
			req.Ranges = append(req.Ranges, mactimeapi.TimeRange{Start: r.Start, End: r.End})
		}
		req.Count = uint32(len(req.Ranges)) //nolint:gosec // ≤ 28 by the schema
	}
	if _, err := d.svc().MactimeAddDelRange(ctx, req); err != nil {
		return dfkit.PluginError("mactime", fmt.Errorf("mactime_add_del_range (add=%v %s): %w", add, formatMAC(mac), err))
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *DeviceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	dev, err := decodeDevice(obj)
	if err != nil {
		return nil, err
	}
	dev = dev.Canon()
	if err := dev.validate(); err != nil {
		return nil, err
	}
	want, err := d.deviceName(dev.Name)
	if err != nil {
		return nil, err
	}
	mac, _ := parseMAC(dev.MAC)
	cur, err := d.find(ctx, mac)
	if err != nil {
		return nil, err
	}
	if cur != nil {
		name := trimName(cur.DeviceName)
		switch {
		case name == want:
			if proto.Equal(decodeDetails(cur, dev.Name), dev.Proto()) {
				return DeviceMeta{mac}, nil // ours already (a Retrieve raced the add)
			}
		case name == autoName(mac) && cur.Flags == flagStaticAllow && cur.Nranges == 0:
			// VPP's own learned entry: replaced by the configured device
		default:
			return nil, fmt.Errorf("%w: mactime device for %s is %q", dfkit.ErrNotOurs, dev.MAC, name)
		}
		if err := d.addDel(ctx, dev, mac, false); err != nil {
			return nil, err
		}
	}
	if err := d.addDel(ctx, dev, mac, true); err != nil {
		return nil, err
	}
	return DeviceMeta{mac}, nil
}

// Update implements scheduler.Descriptor: delete + add (an add on an existing MAC would append).
func (d *DeviceDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, ok := meta.(DeviceMeta)
	if !ok {
		return nil, dfkit.ErrBadMeta
	}
	o, err := decodeDevice(oldObj)
	if err != nil {
		return nil, err
	}
	n, err := decodeDevice(newObj)
	if err != nil {
		return nil, err
	}
	n = n.Canon()
	if err := n.validate(); err != nil {
		return nil, err
	}
	if nm, _ := parseMAC(n.MAC); nm != m.MAC || o.Name != n.Name {
		return nil, scheduler.ErrRecreate
	}
	if err := d.Delete(ctx, oldObj, meta); err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, n, m.MAC, true); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Delete: only while the MAC is still this owner's device.
func (d *DeviceDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(DeviceMeta)
	if !ok {
		return dfkit.ErrBadMeta
	}
	dev, err := decodeDevice(obj)
	if err != nil {
		return err
	}
	want, err := d.deviceName(dev.Name)
	if err != nil {
		return err
	}
	cur, err := d.find(ctx, m.MAC)
	if err != nil {
		return err
	}
	if cur == nil || trimName(cur.DeviceName) != want {
		return nil // gone, or no longer ours: never delete another owner's device
	}
	return d.addDel(ctx, dev, m.MAC, false)
}

// decodeDetails is the desired-form value of a table entry named name.
func decodeDetails(e *mactimeapi.MactimeDetails, name string) proto.Message {
	dev := Device{Name: name, MAC: formatMAC([6]byte(e.MacAddress)), Drop: e.Flags&(flagStaticDrop|flagDynamicDrop) != 0}
	for _, r := range e.Ranges {
		dev.Ranges = append(dev.Ranges, Range{Start: r.Start, End: r.End})
	}
	return dev.Proto()
}

// Retrieve implements scheduler.Descriptor: every device whose name is "<owner>:<name>".
func (d *DeviceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	all, err := Dump(ctx, d.client)
	if err != nil {
		if errors.Is(err, dfkit.ErrPluginNotLoaded) {
			return nil, nil // nothing of ours can exist without the plugin
		}
		return nil, err
	}
	var out []scheduler.KV
	for _, e := range all {
		name, ok := vpp.ParseOwnerTag(trimName(e.DeviceName), d.owner)
		if !ok {
			continue
		}
		out = append(out, scheduler.KV{Key: RangeKey(name), Value: decodeDetails(e, name), Meta: DeviceMeta{[6]byte(e.MacAddress)}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return dfkit.Dedupe(out), nil
}
