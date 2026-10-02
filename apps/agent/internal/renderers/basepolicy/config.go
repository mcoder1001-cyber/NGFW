package basepolicy

import (
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const ProductConfig = "/etc/vrx/base-policy.env"
const maxConfig = 4096

// Config contains only immutable nonsecret bootstrap admission inputs.
type Config struct {
	Management string
	Permanent  []string
}

// LoadConfig never follows a final symlink or shell-sources EnvironmentFile
// syntax. Callers opt into the product explicitly; file absence is not disable.
func LoadConfig(path string) (Config, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return Config{}, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		return Config{}, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != 0 || stat.Mode&0777 != 0600 || stat.Size > maxConfig {
		return Config{}, errors.New("basepolicy: configuration must be bounded root-owned regular 0600 file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxConfig+1))
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(data)
}
func ParseConfig(data []byte) (Config, error) {
	if len(data) > maxConfig {
		return Config{}, errors.New("basepolicy: configuration too large")
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "VRX_BOOTSTRAP_MGMT_IF" && key != "VRX_BOOTSTRAP_PUNT_IFS") {
			return Config{}, errors.New("basepolicy: unexpected configuration assignment")
		}
		if _, seen := values[key]; seen {
			return Config{}, errors.New("basepolicy: duplicate configuration assignment")
		}
		values[key] = value
	}
	management, ok := values["VRX_BOOTSTRAP_MGMT_IF"]
	if !ok {
		return Config{}, errors.New("basepolicy: missing management interface")
	}
	punts, ok := values["VRX_BOOTSTRAP_PUNT_IFS"]
	if !ok {
		return Config{}, errors.New("basepolicy: missing permanent punt inputs")
	}
	var permanent []string
	if punts != "" {
		permanent = strings.Split(punts, ",")
	}
	names, err := Members(management, permanent, nil)
	if err != nil {
		return Config{}, err
	}
	return Config{Management: management, Permanent: names}, nil
}
