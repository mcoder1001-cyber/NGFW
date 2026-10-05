package ravpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"ngfw/agent/internal/scheduler"
)

const NamespaceName = "ra.namespace"

var _ scheduler.Descriptor = (*NamespaceDescriptor)(nil)

func (*NamespaceDescriptor) KeyOf(value proto.Message) scheduler.Key {
	object, ok := value.(*structpb.Struct)
	if !ok {
		return scheduler.Join(NamespaceName, "")
	}
	return scheduler.Join(NamespaceName, NamespaceKeyID(object.GetFields()["instance"].GetStringValue()))
}

type NamespaceMeta struct{ Inode uint64 }
type NamespaceDescriptor struct {
	owner   string
	Handoff NamespaceHandoff
	Guard   MutationGuard
}

// The protected root network.json records ownership and both kernel NSFS
// identities. Retrieve validates those persisted records against held bindings.
func (*NamespaceDescriptor) CheckPersistent() error { return nil }

func NewNamespaceDescriptor(owner string) *NamespaceDescriptor {
	return &NamespaceDescriptor{owner: owner}
}
func (*NamespaceDescriptor) Name() string                                      { return NamespaceName }
func (*NamespaceDescriptor) Stage() scheduler.Stage                            { return scheduler.StageVPP }
func (*NamespaceDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// NamespaceValue contains public input only. Runtime inode belongs to Meta,
// never the desired protobuf (which must remain equal across restarts).
func NamespaceValue(plan *NetworkPlan) (*structpb.Struct, error) {
	if plan.Validate() != nil || plan.NamespaceInode != 0 || plan.HostNamespaceInode != 0 {
		return nil, ErrPlan
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, ErrPlan
	}
	var fields map[string]any
	if json.Unmarshal(data, &fields) != nil {
		return nil, ErrPlan
	}
	return structpb.NewStruct(fields)
}
func (d *NamespaceDescriptor) input(value proto.Message) (*NetworkPlan, error) {
	object, ok := value.(*structpb.Struct)
	if !ok {
		return nil, ErrPlan
	}
	data, err := json.Marshal(object.AsMap())
	if err != nil {
		return nil, ErrPlan
	}
	var plan NetworkPlan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil || decoder.Decode(new(any)) != io.EOF || plan.Owner != d.owner || plan.NamespaceInode != 0 || plan.HostNamespaceInode != 0 || plan.Validate() != nil {
		return nil, ErrPlan
	}
	return &plan, nil
}
func (d *NamespaceDescriptor) Create(ctx context.Context, value proto.Message) (any, error) {
	plan, err := d.input(value)
	if err != nil {
		return nil, err
	}
	if d.Guard != nil {
		if err := d.Guard(ctx, plan); err != nil {
			return nil, err
		}
	}
	if err = CreateNamespace(ctx, plan); err != nil {
		if plan.NamespaceInode != 0 || plan.HostNamespaceInode != 0 {
			return NamespaceMeta{plan.NamespaceInode}, scheduler.PartialCreate(errors.Join(err, scheduler.ErrUncertainOutcome))
		}
		return nil, err
	}
	if d.Handoff != nil {
		if err := d.Handoff.Export(ctx, plan); err != nil {
			return NamespaceMeta{plan.NamespaceInode}, scheduler.PartialCreate(errors.Join(err, scheduler.ErrUncertainOutcome))
		}
		if err := d.Handoff.Verify(ctx, plan); err != nil {
			return NamespaceMeta{plan.NamespaceInode}, scheduler.PartialCreate(errors.Join(err, scheduler.ErrUncertainOutcome))
		}
	}
	return NamespaceMeta{plan.NamespaceInode}, nil
}
func (d *NamespaceDescriptor) Update(_ context.Context, old, new proto.Message, meta any) (any, error) {
	if _, err := d.input(new); err != nil {
		return nil, err
	}
	return meta, scheduler.ErrRecreate
}
func (d *NamespaceDescriptor) Delete(ctx context.Context, value proto.Message, meta any) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	plan, err := d.input(value)
	if err != nil {
		return err
	}
	identity, ok := meta.(NamespaceMeta)
	if !ok || identity.Inode == 0 {
		return ErrBoundary
	}
	actual, err := ReadAgentPlan(plan.Instance)
	if err != nil || actual.NamespaceInode != identity.Inode {
		return ErrBoundary
	}
	if d.Guard != nil {
		if err := d.Guard(ctx, actual); err != nil {
			return err
		}
	}
	if d.Handoff != nil {
		if err := d.Handoff.Remove(ctx, actual); err != nil {
			return err
		}
	}
	if err = RemoveNamespace(plan.Instance, identity.Inode); err != nil {
		return err
	}
	dir := filepath.Join(InstanceRoot, plan.Instance)
	if os.Remove(filepath.Join(dir, "network.json")) != nil || os.Remove(dir) != nil {
		return ErrBoundary
	}
	return nil
}
func (d *NamespaceDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	entries, err := os.ReadDir(InstanceRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrBoundary
	}
	var result []scheduler.KV
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !entry.IsDir() || !ValidInstance(entry.Name()) {
			continue
		}
		path := filepath.Join(InstanceRoot, entry.Name(), "network.json")
		if ValidatePrivateFile(path, 16384) != nil {
			return nil, ErrBoundary
		}
		file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, ErrBoundary
		}
		var plan NetworkPlan
		err = json.NewDecoder(file).Decode(&plan)
		file.Close()
		if err != nil {
			return nil, ErrBoundary
		}
		if plan.Owner != d.owner {
			continue
		}
		if plan.Validate() != nil || plan.Instance != entry.Name() || plan.NamespaceInode == 0 || plan.HostNamespaceInode == 0 || plan.NamespaceInode == plan.HostNamespaceInode {
			return nil, ErrBoundary
		}
		binding := filepath.Join(InstanceRoot, entry.Name(), "netns")
		fd, err := unix.Open(binding, unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, ErrBoundary
		}
		var stat, host unix.Stat_t
		var fs unix.Statfs_t
		base, err := unix.Open(filepath.Join(InstanceRoot, entry.Name(), "hostnetns"), unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
		var baseFS unix.Statfs_t
		bad := err != nil || unix.Fstat(fd, &stat) != nil || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || stat.Ino != plan.NamespaceInode || unix.Fstat(base, &host) != nil || unix.Fstatfs(base, &baseFS) != nil || baseFS.Type != unix.NSFS_MAGIC || host.Ino != plan.HostNamespaceInode || stat.Ino == host.Ino && stat.Dev == host.Dev
		if base >= 0 {
			unix.Close(base)
		}
		unix.Close(fd)
		if bad || validateNamespaceAlias(&plan) != nil {
			return nil, ErrBoundary
		}
		if d.Handoff != nil {
			if err := d.Handoff.Verify(ctx, &plan); err != nil {
				return nil, err
			}
		}
		meta := NamespaceMeta{plan.NamespaceInode}
		plan.NamespaceInode = 0
		plan.HostNamespaceInode = 0
		plan.KernelLinks = nil
		value, err := NamespaceValue(&plan)
		if err != nil {
			return nil, err
		}
		result = append(result, scheduler.KV{Key: scheduler.Join(NamespaceName, NamespaceKeyID(plan.Instance)), Value: value, Meta: meta})
	}
	return result, nil
}
