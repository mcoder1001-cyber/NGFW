package basepolicy

import (
	"errors"
	"fmt"
	"slices"

	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/scheduler"
)

// Admission is internal state only, keyed by host identity for revoke-before-
// pair-delete ordering. Pair is empty only for orphan kernel readback.
type Admission struct {
	Host string `json:"host"`
	Pair string `json:"pair"`
}

// Project requires an explicitly known effective default namespace from the
// transaction/VPP owner. Empty desired Netns means that default, not root.
func Project(management string, permanent []string, pairs []lcp.ItfPair, defaultNamespace string, namespaceKnown bool) ([]Admission, error) {
	if !namespaceKnown {
		return nil, errors.New("basepolicy: default LCP namespace unknown")
	}
	seen := map[string]bool{}
	static := map[string]bool{}
	for _, name := range permanent {
		static[name] = true
	}
	var admissions []Admission
	var dynamic []string
	for _, pair := range pairs {
		if err := pair.Validate(); err != nil {
			return nil, err
		}
		effective := pair.Netns
		if effective == "" {
			effective = defaultNamespace
		}
		if effective != "" {
			continue
		}
		if seen[pair.HostIfName] {
			return nil, fmt.Errorf("basepolicy: ambiguous root host %q", pair.HostIfName)
		}
		seen[pair.HostIfName] = true
		if static[pair.HostIfName] {
			return nil, errors.New("basepolicy: permanent admission overlaps dynamic host")
		}
		dynamic = append(dynamic, pair.HostIfName)
		admissions = append(admissions, Admission{Host: pair.HostIfName, Pair: string(scheduler.Join(lcp.NameItfPair, pair.Interface))})
	}
	if _, err := Members(management, permanent, dynamic); err != nil {
		return nil, err
	}
	slices.SortFunc(admissions, func(a, b Admission) int {
		if a.Host < b.Host {
			return -1
		}
		if a.Host > b.Host {
			return 1
		}
		return 0
	})
	return admissions, nil
}
