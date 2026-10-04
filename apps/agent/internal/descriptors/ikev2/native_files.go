package ikev2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	ikev2api "ngfw/agent/binapi/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/vpp"
)

// NativePaths derives immutable owner-private snapshot paths from verified
// keyed fingerprints. Including the local-key generation in the peer path forces
// profile recreation on a shared-key rotation, terminating old trust sessions.
func NativePaths(root, keyRef, peerRef string) (string, string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || vpn.CheckRef(keyRef) != nil || vpn.CheckRef(peerRef) != nil || !strings.HasPrefix(keyRef, vpn.RefHMAC) || !strings.HasPrefix(peerRef, vpn.RefHMAC) {
		return "", "", errors.New("native certificate snapshot references are invalid")
	}
	key := filepath.Join(root, strings.TrimPrefix(keyRef, vpn.RefHMAC)+".key.pem")
	peer := filepath.Join(root, strings.TrimPrefix(peerRef, vpn.RefHMAC)+"-"+strings.TrimPrefix(keyRef, vpn.RefHMAC)+".cert.pem")
	if len(key) > 255 || len(peer) > 1023 {
		return "", "", errors.New("native certificate state path is too long")
	}
	return key, peer, nil
}

// snapshot never overwrites generations; rollback and confirmed-commit revert
// may need historical files after the ordinary PKI file set has changed.
func (c Config) snapshot(ctx context.Context, path string, key bool) (resultErr error) {
	if c.NativeRoot == "" {
		return nil
	} // descriptor-only tooling owns its explicit files
	if filepath.Dir(path) != c.NativeRoot {
		return errors.New("native certificate snapshot must be inside the owner state directory")
	}
	name := filepath.Base(path)
	var ref string
	if key {
		if len(name) != 72 || !strings.HasSuffix(name, ".key.pem") {
			return errors.New("invalid native key snapshot path")
		}
		ref = vpn.RefHMAC + name[:64]
	} else {
		if len(name) != 138 || name[64] != '-' || !strings.HasSuffix(name, ".cert.pem") {
			return errors.New("invalid native peer snapshot path")
		}
		if vpn.CheckRef(vpn.RefHMAC+name[65:129]) != nil {
			return vpn.ErrBadRef
		}
		ref = vpn.RefHMAC + name[:64]
	}
	if vpn.CheckRef(ref) != nil {
		return vpn.ErrBadRef
	}
	material, err := vpn.Resolve(ctx, c.Secrets, c.Keys, ref)
	if err != nil {
		return fmt.Errorf("native certificate snapshot material unavailable: %w", err)
	}
	defer vpn.Zero(material)
	materialLimit := 64 << 10
	if key {
		materialLimit = 16 << 10
	}
	if len(material) == 0 || len(material) > materialLimit {
		return errors.New("native certificate snapshot material exceeds its size bound")
	}
	// The configured state directory is trusted, but refuse a symlink anywhere in
	// this path rather than following one into a system or another owner's tree.
	for p := c.NativeRoot; p != "/"; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) && p == c.NativeRoot {
			continue
		}
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("native certificate snapshot parent is not a real directory")
		}
	}
	if err = os.Mkdir(c.NativeRoot, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	st, err := os.Lstat(c.NativeRoot)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm() != 0700 {
		return errors.New("native certificate snapshot directory must have mode 0700")
	}
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if errors.Is(err, syscall.EEXIST) {
		fd, err = syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		f := os.NewFile(uintptr(fd), path)
		defer func() {
			if closeErr := f.Close(); resultErr == nil {
				resultErr = closeErr
			}
		}()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() > int64(materialLimit) {
			return errors.New("native certificate snapshot has unsafe metadata")
		}
		existing, err := io.ReadAll(io.LimitReader(f, int64(materialLimit)+1))
		defer vpn.Zero(existing)
		if err != nil {
			return err
		}
		if c.Keys.Ref(existing) != ref {
			return vpn.ErrSecretMismatch
		}
		return nil
	}
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	if _, err = f.Write(material); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// CheckCertificateOwnership rejects foreign RSA profiles during dry-run, before
// PKI or native immutable snapshots are written. Repeated at the key setter to
// guard against an ownership change between projection and application.
func CheckCertificateOwnership(ctx context.Context, client vpp.Client, owner string) error {
	stream, err := ikev2api.NewServiceClient(client).Ikev2ProfileDump(ctx, &ikev2api.Ikev2ProfileDump{})
	if err != nil {
		return err
	}
	for {
		det, e := stream.Recv()
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return e
		}
		foreign := det.Profile.Auth.Method == authRSASig && !strings.HasPrefix(det.Profile.Name, owner+"-")
		vpn.Zero(det.Profile.Auth.Data)
		if foreign {
			return errors.New("native local key is shared with a foreign RSA profile")
		}
	}
}
