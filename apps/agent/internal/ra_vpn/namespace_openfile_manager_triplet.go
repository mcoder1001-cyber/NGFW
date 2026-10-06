package ravpn

import (
	"context"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
	"strings"
	"unicode/utf8"
)

// numericPublisherTrustSnapshot is a fresh, private, single-boundary readback.
// Each consumer still verifies full actual Source/server process identities and
// the held installation proof. It is never retained across IPC or mutations.
// Role maps are selected only by exact Id, never by unit order or caller input.
type numericPublisherTrustSnapshot struct {
	publisher                 numericPublisherManagerSnapshot
	source                    map[string]string
	proof                     *numericPublisherInstallationProof
	identity                  bootid.Identity
	sourceUsed, publisherUsed bool
}

func parseNumericPublisherManagerTriplet(output []byte) (*numericPublisherTrustSnapshot, error) {
	if len(output) == 0 || len(output) > 16384 || !utf8.Valid(output) || strings.ContainsAny(string(output), "\x00\r") {
		return nil, ErrBoundary
	}
	blocks := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n\n")
	if len(blocks) != 3 {
		return nil, ErrBoundary
	}
	snapshot := &numericPublisherTrustSnapshot{}
	var publisherBlocks []string
	allowed := make(map[string]bool)
	for _, key := range strings.Split(numericPublisherManagerProperties, ",") {
		allowed[key] = true
	}
	for _, block := range blocks {
		fields := make(map[string]string)
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok || !allowed[key] {
				return nil, ErrBoundary
			}
			if _, duplicate := fields[key]; duplicate {
				return nil, ErrBoundary
			}
			fields[key] = value
		}
		switch fields["Id"] {
		case "ngfw-agent.service":
			if snapshot.source != nil {
				return nil, ErrBoundary
			}
			snapshot.source = fields
			for _, key := range strings.Split("Id,MainPID,ControlGroup,FragmentPath,DropInPaths,User,Group,ExecStart", ",") {
				if _, ok := fields[key]; !ok {
					return nil, ErrBoundary
				}
			}
		case "ngfw-ra-openfile.socket", "ngfw-ra-openfile.service":
			publisherBlocks = append(publisherBlocks, block)
		default:
			return nil, ErrBoundary
		}
	}
	if snapshot.source == nil || len(publisherBlocks) != 2 {
		return nil, ErrBoundary
	}
	publisher, err := parseNumericPublisherManagerPair([]byte(strings.Join(publisherBlocks, "\n\n")))
	if err != nil {
		return nil, ErrBoundary
	}
	snapshot.publisher = publisher
	return snapshot, nil
}

// loadNumericPublisherSourceTrust protects the Source-first query before the
// original manager verifier is entered.
func loadNumericPublisherSourceTrust(ctx context.Context, proof *numericPublisherInstallationProof, source bootid.Identity) (*numericPublisherTrustSnapshot, error) {
	if proof.Verify(ctx) != nil {
		return nil, ErrBoundary
	}
	snapshot, err := loadNumericPublisherManagerBracketedTrust(ctx, proof, source)
	if err != nil || proof.Verify(ctx) != nil {
		return nil, ErrBoundary
	}
	return snapshot, nil
}

// loadNumericPublisherManagerBracketedTrust is called within the original
// manager verifier's proof bracket, or the explicit Source-first bracket above.
func loadNumericPublisherManagerBracketedTrust(ctx context.Context, proof *numericPublisherInstallationProof, source bootid.Identity) (*numericPublisherTrustSnapshot, error) {
	if ctx.Err() != nil || proof == nil || !proof.source.Complete() || !source.Complete() || source.PID <= 1 || !(bootid.Reader{}).ForPID(source.PID).Equal(source) {
		return nil, ErrBoundary
	}
	roles := append([]managerDBusRole(nil), managerDBusPublisherRoles...)
	roles = append(roles, managerDBusSourceRole)
	values, err := readManagerDBusRoles(ctx, roles)
	if err != nil || len(values) != 3 || !(bootid.Reader{}).ForPID(source.PID).Equal(source) {
		return nil, ErrBoundary
	}
	snapshot := &numericPublisherTrustSnapshot{
		publisher: numericPublisherManagerSnapshot{socket: values[0], service: values[1]},
		source:    values[2],
	}
	snapshot.proof = proof
	snapshot.identity = source
	return snapshot, nil
}

func (snapshot *numericPublisherTrustSnapshot) guard(ctx context.Context) error {
	if ctx.Err() != nil || snapshot == nil || snapshot.proof == nil || !snapshot.proof.source.Complete() || !snapshot.identity.Complete() || snapshot.identity.PID <= 1 || !(bootid.Reader{}).ForPID(snapshot.identity.PID).Equal(snapshot.identity) {
		return ErrBoundary
	}
	return nil
}

func (snapshot *numericPublisherTrustSnapshot) sourceProperties(ctx context.Context) (map[string]string, error) {
	if snapshot.guard(ctx) != nil || snapshot.sourceUsed {
		return nil, ErrBoundary
	}
	snapshot.sourceUsed = true
	return snapshot.source, nil
}

func (snapshot *numericPublisherTrustSnapshot) publisherProperties(ctx context.Context) (numericPublisherManagerSnapshot, error) {
	if snapshot.guard(ctx) != nil || snapshot.publisherUsed {
		return numericPublisherManagerSnapshot{}, ErrBoundary
	}
	snapshot.publisherUsed = true
	return snapshot.publisher, nil
}

func numericPublisherSourceTrust(ctx context.Context, peer *unix.Ucred, source bootid.Identity, proof *numericPublisherInstallationProof) (*numericPublisherTrustSnapshot, error) {
	var snapshot *numericPublisherTrustSnapshot
	err := verifyFixedAgentPeerUsing(ctx, peer, source, func(read context.Context) (map[string]string, error) {
		var err error
		snapshot, err = loadNumericPublisherSourceTrust(read, proof, source)
		if err != nil {
			return nil, err
		}
		return snapshot.sourceProperties(read)
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func numericPublisherManagerTrust(ctx context.Context, server bootid.Identity, proof *numericPublisherInstallationProof, source bootid.Identity) (*numericPublisherTrustSnapshot, error) {
	var snapshot *numericPublisherTrustSnapshot
	err := numericPublisherManagerUsing(ctx, server, proof, func(read context.Context) (numericPublisherManagerSnapshot, error) {
		var err error
		snapshot, err = loadNumericPublisherManagerBracketedTrust(read, proof, source)
		if err != nil {
			return numericPublisherManagerSnapshot{}, err
		}
		return snapshot.publisherProperties(read)
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func numericPublisherPeerFromTrust(ctx context.Context, peer *unix.Ucred, source bootid.Identity, snapshot *numericPublisherTrustSnapshot) error {
	if snapshot == nil || !snapshot.identity.Equal(source) {
		return ErrBoundary
	}
	return verifyFixedAgentPeerUsing(ctx, peer, source, snapshot.sourceProperties)
}

func numericPublisherManagerFromTrust(ctx context.Context, server bootid.Identity, proof *numericPublisherInstallationProof, snapshot *numericPublisherTrustSnapshot) error {
	if snapshot == nil || snapshot.proof != proof {
		return numericPublisherFailure(ctx, 6)
	}
	return numericPublisherManagerUsing(ctx, server, proof, snapshot.publisherProperties)
}
