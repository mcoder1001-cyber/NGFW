package ravpn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
)

// NumericPublisherValidationBudget bounds fresh whole-artifact installation
// validation independently of the IPC phase. Caller cancellation still applies.
const NumericPublisherValidationBudget = 20 * time.Second

// NumericPublisherIPCBudget bounds each manager exchange after installation
// validation. It does not permit an unbounded socket or process lifetime.
const NumericPublisherIPCBudget = 5 * time.Second

// NumericOpenFilePublicationBudget bounds the complete installation, identity,
// two fresh manager captures and publication operation. It is not a cache TTL.
const NumericOpenFilePublicationBudget = 40 * time.Second

// Fixed publisher diagnostics use journal stderr; stdout remains discarded.
const numericPublisherServiceDigest = "24226ed4e32a1880d031f5222a8ea8c201dfdeb4a6b44fb1fd72ce3bac621712"

// A proof belongs to exactly one call/process and holds the actual validated
// files. Every trust boundary must verify full file stamps and fresh canonical
// paths plus the complete creating process boot identity. No proof crosses a
// call, agent restart or helper execution. Close releases every held descriptor.
type numericPublisherInstallationProof struct {
	source bootid.Identity
	files  []numericPublisherHeldArtifact
}

type numericPublisherHeldArtifact struct {
	path       string
	file       *os.File
	stat       unix.Stat_t
	limit      int64
	executable bool
}

func newNumericPublisherInstallationProof(ctx context.Context) (*numericPublisherInstallationProof, error) {
	bounded, cancel := context.WithTimeout(ctx, NumericPublisherValidationBudget)
	defer cancel()
	proof := &numericPublisherInstallationProof{source: (bootid.Reader{}).ForPID(os.Getpid())}
	if !proof.source.Complete() {
		return nil, ErrBoundary
	}
	success := false
	defer func() {
		if !success {
			_ = proof.Close()
		}
	}()
	items := []struct {
		path       string
		limit      int64
		executable bool
		digest     string
	}{
		{unitObserverExecutable, 32 << 20, true, ""},
		{unitObserverExecutable + ".sha256", 128, false, ""},
		{numericPublisherService, 16384, false, numericPublisherServiceDigest},
		{numericPublisherSocket, 16384, false, "e548b49466216b1254bd41cf9cb04d9d2ae342d1b8b228848a2362ba48b08147"},
	}
	var executableDigest string
	for index, item := range items {
		artifact, err := openNumericPublisherArtifact(item.path, item.limit, item.executable)
		if err != nil {
			return nil, numericPublisherFailure(bounded, 4)
		}
		proof.files = append(proof.files, artifact)
		digest, header, err := hashNumericPublisherArtifact(bounded, artifact)
		if err != nil {
			return nil, numericPublisherFailure(bounded, 4)
		}
		if index == 0 {
			if !bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}) {
				return nil, numericPublisherFailure(bounded, 4)
			}
			executableDigest = digest
		} else if index == 1 {
			if _, err := artifact.file.Seek(0, io.SeekStart); err != nil {
				return nil, numericPublisherFailure(bounded, 4)
			}
			receipt, err := io.ReadAll(io.LimitReader(artifact.file, 129))
			if err != nil || len(receipt) > 128 || strings.TrimSpace(string(receipt)) != executableDigest {
				return nil, numericPublisherFailure(bounded, 4)
			}
		} else if digest != item.digest {
			return nil, numericPublisherFailure(bounded, 5)
		}
	}
	if proof.Verify(bounded) != nil {
		return nil, numericPublisherFailure(bounded, 4)
	}
	success = true
	return proof, nil
}

func openNumericPublisherArtifact(path string, limit int64, executable bool) (numericPublisherHeldArtifact, error) {
	empty := numericPublisherHeldArtifact{}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || limit <= 0 || limit > 32<<20 {
		return empty, ErrBoundary
	}
	parent, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return empty, ErrBoundary
	}
	defer func() { _ = unix.Close(parent) }()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts[:len(parts)-1] {
		next, err := unix.Openat(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return empty, ErrBoundary
		}
		if unix.Close(parent) != nil {
			_ = unix.Close(next)
			return empty, ErrBoundary
		}
		parent = next
		var stat unix.Stat_t
		if unix.Fstat(parent, &stat) != nil || stat.Uid != 0 || stat.Mode&0022 != 0 && stat.Mode&unix.S_ISVTX == 0 {
			return empty, ErrBoundary
		}
	}
	fd, err := unix.Openat(parent, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return empty, ErrBoundary
	}
	file := os.NewFile(uintptr(fd), "held publisher installation")
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Uid != 0 || stat.Nlink != 1 || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0022 != 0 || stat.Size < 0 || stat.Size > limit || executable && stat.Mode&0111 == 0 {
		_ = file.Close()
		return empty, ErrBoundary
	}
	return numericPublisherHeldArtifact{path: path, file: file, stat: stat, limit: limit, executable: executable}, nil
}

func sameNumericPublisherArtifact(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Size == b.Size && a.Uid == b.Uid && a.Gid == b.Gid && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Ctim == b.Ctim && a.Mtim == b.Mtim
}

func hashNumericPublisherArtifact(ctx context.Context, artifact numericPublisherHeldArtifact) (string, []byte, error) {
	if ctx.Err() != nil || artifact.file == nil {
		return "", nil, ErrBoundary
	}
	if _, err := artifact.file.Seek(0, io.SeekStart); err != nil {
		return "", nil, ErrBoundary
	}
	hash := sha256.New()
	buffer := make([]byte, 256<<10)
	var header []byte
	var total int64
	for {
		if ctx.Err() != nil {
			return "", nil, ErrBoundary
		}
		count, err := artifact.file.Read(buffer)
		if count > 0 {
			if len(header) < 4 {
				header = append(header, buffer[:min(count, 4-len(header))]...)
			}
			total += int64(count)
			if total > artifact.limit {
				return "", nil, ErrBoundary
			}
			if _, writeErr := hash.Write(buffer[:count]); writeErr != nil {
				return "", nil, ErrBoundary
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, ErrBoundary
		}
	}
	var after unix.Stat_t
	if ctx.Err() != nil || total != artifact.stat.Size || unix.Fstat(int(artifact.file.Fd()), &after) != nil || !sameNumericPublisherArtifact(artifact.stat, after) {
		return "", nil, ErrBoundary
	}
	return hex.EncodeToString(hash.Sum(nil)), header, nil
}

func (p *numericPublisherInstallationProof) Verify(ctx context.Context) error {
	if ctx.Err() != nil || p == nil || !p.source.Complete() || !(bootid.Reader{}).ForPID(os.Getpid()).Equal(p.source) || len(p.files) != 4 {
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

func (p *numericPublisherInstallationProof) Close() error {
	if p == nil {
		return nil
	}
	var failure error
	for _, artifact := range p.files {
		if artifact.file != nil {
			failure = errors.Join(failure, artifact.file.Close())
		}
	}
	p.files = nil
	if failure != nil {
		return ErrBoundary
	}
	return nil
}
