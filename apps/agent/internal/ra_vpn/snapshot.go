package ravpn

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var credentialName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// PrivateSnapshot has no generic serialization of passwords or key material.
// It is rendered from sealed-cache material before any daemon starts.
type PrivateSnapshot struct {
	Daemon             []byte      `json:"-"`
	Connection         []byte      `json:"-"`
	Secrets            []byte      `json:"-"`
	Credentials        Credentials `json:"-"`
	CertificateName    string
	ClientCAName       string
	Identity           string
	CertificateClients bool
}

func (PrivateSnapshot) String() string     { return "remote-access private snapshot <redacted>" }
func (s PrivateSnapshot) GoString() string { return s.String() }

func (s PrivateSnapshot) Validate(now time.Time) error {
	if !credentialName.MatchString(s.CertificateName) || len(s.Daemon) == 0 || len(s.Daemon) > 1<<20 || len(s.Connection) == 0 || len(s.Connection) > 1<<20 || len(s.Secrets) > 1<<20 {
		return ErrBoundary
	}
	if s.CertificateClients && !credentialName.MatchString(s.ClientCAName) || !s.CertificateClients && s.ClientCAName != "" {
		return ErrBoundary
	}
	return VerifyCredentials(s.Credentials, s.Identity, s.CertificateClients, now)
}

// WriteSnapshot creates an exclusive generation. Existing credentials are
// never overwritten while a daemon might still hold a previous trust policy.
// Controller must stop/delete the old generation before creating its replacement.
func WriteSnapshot(instance string, snapshot PrivateSnapshot, now time.Time) error {
	if snapshot.Validate(now) != nil {
		return ErrBoundary
	}
	if _, err := ReadAgentPlan(instance); err != nil {
		return err
	}
	dir := filepath.Join(InstanceRoot, instance)
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrBoundary
	}
	defer unix.Close(fd)
	var createdDirs, createdFiles []string
	rollback := func() {
		for i := len(createdFiles) - 1; i >= 0; i-- {
			unix.Unlinkat(fd, createdFiles[i], 0)
		}
		for i := len(createdDirs) - 1; i >= 0; i-- {
			unix.Unlinkat(fd, createdDirs[i], unix.AT_REMOVEDIR)
		}
	}
	for _, name := range []string{"private", "x509", "x509ca", "x509crl", "daemon"} {
		if unix.Mkdirat(fd, name, 0700) != nil {
			rollback()
			return ErrBoundary
		}
		createdDirs = append(createdDirs, name)
	}
	files := map[string][]byte{"strongswan.conf": snapshot.Daemon, "swanctl.conf": append(append([]byte{}, snapshot.Connection...), snapshot.Secrets...), "x509/" + snapshot.CertificateName + ".pem": snapshot.Credentials.Certificate, "private/" + snapshot.CertificateName + ".pem": snapshot.Credentials.PrivateKey}
	if snapshot.CertificateClients {
		files["x509ca/"+snapshot.ClientCAName+".pem"] = snapshot.Credentials.ClientCA
		files["x509crl/"+snapshot.ClientCAName+".pem"] = snapshot.Credentials.ClientCRL
	}
	defer clear(files["swanctl.conf"])
	for name, data := range files {
		child, err := unix.Openat(fd, name, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if err != nil {
			rollback()
			return ErrBoundary
		}
		createdFiles = append(createdFiles, name)
		file := os.NewFile(uintptr(child), name)
		_, err = file.Write(data)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			rollback()
			return ErrBoundary
		}
	}
	if unix.Fsync(fd) != nil {
		rollback()
		return ErrBoundary
	}
	return nil
}
