package ravpn

import (
	"bytes"
	"context"
	"golang.org/x/sys/unix"
	"io"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"strings"
)

// brokerExecutableProof belongs to one capture in one process. It hashes the
// whole protected ELF once, holds ELF/receipt descriptors, and checks complete
// stamps and freshly reopened canonical paths at every later boundary. Each
// independent capture and supplier constructs its own proof; none is cached.
type brokerExecutableProof struct {
	source bootid.Identity
	files  []numericPublisherHeldArtifact
}

func newBrokerExecutableProof(ctx context.Context, path string) (*brokerExecutableProof, error) {
	p := &brokerExecutableProof{source: (bootid.Reader{}).ForPID(os.Getpid())}
	success := false
	defer func() {
		if !success {
			_ = p.Close()
		}
	}()
	for _, item := range []struct {
		path       string
		limit      int64
		executable bool
	}{{path, 32 << 20, true}, {path + ".sha256", 128, false}} {
		artifact, err := openNumericPublisherArtifact(item.path, item.limit, item.executable)
		if err != nil {
			return nil, ErrBoundary
		}
		p.files = append(p.files, artifact)
	}
	digest, header, err := hashNumericPublisherArtifact(ctx, p.files[0])
	if err != nil || !bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}) {
		return nil, ErrBoundary
	}
	receipt, err := io.ReadAll(io.LimitReader(p.files[1].file, 129))
	if err != nil || len(receipt) > 128 || strings.TrimSpace(string(receipt)) != digest || p.Verify(ctx, path) != nil {
		return nil, ErrBoundary
	}
	success = true
	return p, nil
}

func (p *brokerExecutableProof) Verify(ctx context.Context, path string) error {
	if ctx.Err() != nil || p == nil || len(p.files) != 2 || p.files[0].path != path || p.files[1].path != path+".sha256" || !p.source.Complete() || !(bootid.Reader{}).ForPID(os.Getpid()).Equal(p.source) {
		return ErrBoundary
	}
	for _, artifact := range p.files {
		var held unix.Stat_t
		if artifact.file == nil || unix.Fstat(int(artifact.file.Fd()), &held) != nil || !sameNumericPublisherArtifact(held, artifact.stat) {
			return ErrBoundary
		}
		fresh, err := openNumericPublisherArtifact(artifact.path, artifact.limit, artifact.executable)
		if err != nil {
			return ErrBoundary
		}
		same := sameNumericPublisherArtifact(fresh.stat, artifact.stat)
		closeErr := fresh.file.Close()
		if !same || closeErr != nil {
			return ErrBoundary
		}
	}
	if ctx.Err() != nil || !(bootid.Reader{}).ForPID(os.Getpid()).Equal(p.source) {
		return ErrBoundary
	}
	return nil
}

func (p *brokerExecutableProof) Close() error {
	if p == nil {
		return nil
	}
	var result error
	for _, artifact := range p.files {
		if artifact.file != nil && artifact.file.Close() != nil {
			result = ErrBoundary
		}
	}
	p.files = nil
	return result
}
