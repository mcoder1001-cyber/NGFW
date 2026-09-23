package memif

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"google.golang.org/protobuf/proto"

	memifapi "ngfw/agent/binapi/memif"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	SocketName = "memif.socket"
	MemifName  = "memif.memif"
)

// ErrEmptyValue is returned for a nil or foreign desired value.
var ErrEmptyValue = errors.New("memif: nil or wrong desired value type")

// DefaultSocketDir is where an agent with owner keeps its memif sockets: the production agent
// ("vrx") under /run/vrx/memif, every other owner (a worker's VRX_TEST_PREFIX) under
// /run/vrx-test/<owner>/memif (docs/lab/shared-host-rules.md).
func DefaultSocketDir(owner string) string {
	if owner == "vrx" {
		return "/run/vrx/memif"
	}
	return filepath.Join("/run/vrx-test", owner, "memif")
}

type base struct {
	client vpp.Client
	owner  string
	dir    string
}

func (b base) svc() memifapi.RPCService { return memifapi.NewServiceClient(b.client) }

// SocketDescriptor implements memif.socket (memif_socket_filename_add_del_v2). memif sockets have
// no tag, so ownership is the directory: a socket is ours iff its filename lives directly in the
// owner's socket dir. Socket id 0 is VPP's built-in default and is never ours.
type SocketDescriptor struct{ base }

// NewSocket returns the descriptor for owner; dir "" means DefaultSocketDir(owner).
func NewSocket(c vpp.Client, owner, dir string) *SocketDescriptor {
	if dir == "" {
		dir = DefaultSocketDir(owner)
	}
	return &SocketDescriptor{base{c, owner, filepath.Clean(dir)}}
}

// SocketMeta is the socket id.
type SocketMeta struct{ ID uint32 }

// SocketKey is the key of socket id.
func SocketKey(id uint32) scheduler.Key {
	return scheduler.Join(SocketName, strconv.FormatUint(uint64(id), 10))
}

// Name implements scheduler.Descriptor.
func (*SocketDescriptor) Name() string { return SocketName }

// KeyOf implements scheduler.Descriptor.
func (*SocketDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	return SocketKey(obj.(*Socket).GetId())
}

// Dependencies implements scheduler.Descriptor.
func (*SocketDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *SocketDescriptor) owned(filename string) bool {
	return filepath.Dir(filepath.Clean(filename)) == d.dir && filepath.Base(filename) != "."
}

// Create implements scheduler.Descriptor.
func (d *SocketDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*Socket)
	if !ok {
		return nil, ErrEmptyValue
	}
	if o.GetId() == 0 || o.GetId() == ^uint32(0) {
		return nil, fmt.Errorf("memif: socket id %d is reserved", o.GetId())
	}
	if !d.owned(o.GetFilename()) {
		return nil, fmt.Errorf("memif: socket filename %q must be directly under %s", o.GetFilename(), d.dir)
	}
	if err := os.MkdirAll(d.dir, 0o750); err != nil {
		return nil, fmt.Errorf("memif: socket dir: %w", err)
	}
	_, err := d.svc().MemifSocketFilenameAddDelV2(ctx, &memifapi.MemifSocketFilenameAddDelV2{IsAdd: true, SocketID: o.GetId(), SocketFilename: o.GetFilename()})
	if err != nil {
		return nil, fmt.Errorf("memif_socket_filename_add_del_v2: %w", err)
	}
	return SocketMeta{o.GetId()}, nil
}

// Update implements scheduler.Descriptor.
func (*SocketDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *SocketDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(SocketMeta)
	if !ok {
		return fmt.Errorf("memif: unexpected meta %T", meta)
	}
	o, ok := obj.(*Socket)
	if !ok {
		return ErrEmptyValue
	}
	_, err := d.svc().MemifSocketFilenameAddDelV2(ctx, &memifapi.MemifSocketFilenameAddDelV2{IsAdd: false, SocketID: m.ID, SocketFilename: o.GetFilename()})
	if err != nil {
		return fmt.Errorf("memif_socket_filename_add_del_v2 (del): %w", err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor.
func (d *SocketDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := d.svc().MemifSocketFilenameDump(ctx, &memifapi.MemifSocketFilenameDump{})
	if err != nil {
		return nil, fmt.Errorf("memif_socket_filename_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		s, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("memif_socket_filename_dump: %w", err)
		}
		if s.SocketID == 0 || !d.owned(s.SocketFilename) {
			continue
		}
		out = append(out, scheduler.KV{Key: SocketKey(s.SocketID), Value: &Socket{Id: s.SocketID, Filename: s.SocketFilename}, Meta: SocketMeta{s.SocketID}})
	}
	return out, nil
}
