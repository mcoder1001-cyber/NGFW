package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// interfaceLabels is presentation-only. Keep ifaceState and the JSON/configuration
// renderers unchanged: display aliases must never become configuration identities.
type interfaceLabels struct {
	labels    map[string]string
	names     map[string]string
	ambiguous map[string]bool
}

type interfaceDisplayItem struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Parent string `json:"parent"`
	State  *struct {
		Type    string  `json:"type"`
		VPPName *string `json:"vppName"`
	} `json:"state"`
	Physical *struct {
		PCI string `json:"pci"`
	} `json:"physical"`
	HostInventory *struct {
		PCI    string `json:"pci"`
		Netdev string `json:"netdev"`
	} `json:"hostInventory"`
}

func (item interfaceDisplayItem) pci() string {
	if item.Physical != nil {
		return strings.ToLower(item.Physical.PCI)
	}
	if item.HostInventory != nil {
		return strings.ToLower(item.HostInventory.PCI)
	}
	return ""
}

func (item interfaceDisplayItem) hardware() bool {
	if item.Kind != "interface" {
		return false
	}
	if item.Physical != nil || (item.HostInventory != nil && item.HostInventory.PCI != "") {
		return true
	}
	if item.State != nil {
		switch strings.ToLower(item.State.Type) {
		case "dpdk", "vmxnet3", "avf", "rdma", "af_xdp":
			return true
		}
	}
	// VPP also uses virtio for virtual TAPs; it is not evidence of a physical port.
	return false
}

func (item interfaceDisplayItem) sortName() string {
	if item.State != nil && item.State.VPPName != nil {
		return *item.State.VPPName
	}
	return item.Name
}

// Match apps/web/src/domains/interfaces/display-names.ts. Number the FULL
// inventory before filtering retrieved rows, so hidden host ports reserve their
// position. Negotiated speed is deliberately irrelevant to the port's name.
func newInterfaceLabels(raw json.RawMessage) interfaceLabels {
	var inventory struct {
		Items []interfaceDisplayItem `json:"items"`
	}
	if json.Unmarshal(raw, &inventory) != nil {
		return interfaceLabels{}
	}
	labels := interfaceLabels{labels: make(map[string]string), names: make(map[string]string), ambiguous: make(map[string]bool)}
	var ports []interfaceDisplayItem
	for _, item := range inventory.Items {
		labels.labels[item.Name] = item.Name
		if item.hardware() {
			ports = append(ports, item)
		}
	}
	// English numeric collation also gives native VPP names (...2, ...10) the
	// same ordering as the web localeCompare(..., "en", {numeric: true}).
	nativeOrder := collate.New(language.English, collate.Numeric)
	sort.SliceStable(ports, func(i, j int) bool {
		a, b := ports[i], ports[j]
		pciA, pciB := a.pci(), b.pci()
		if pciA != "" && pciB != "" {
			return pciA < pciB
		}
		if pciA != "" || pciB != "" {
			return pciA != ""
		}
		return nativeOrder.CompareString(a.sortName(), b.sortName()) < 0
	})
	for i, item := range ports {
		labels.labels[item.Name] = fmt.Sprintf("Ethernet%d", i+1)
	}
	for _, item := range inventory.Items {
		parent := labels.labels[item.Parent]
		if item.Kind == "subinterface" && item.Parent != "" && parent != "" && parent != item.Parent && strings.HasPrefix(item.Name, item.Parent+".") {
			labels.labels[item.Name] = parent + strings.TrimPrefix(item.Name, item.Parent)
		}
	}
	// Only canonical logical names enter the inverse map. A host/VPP secondary
	// spelling must not win, and a display alias takes priority over a colliding
	// raw name when reading a human operational command.
	for _, item := range inventory.Items {
		label := labels.labels[item.Name]
		if name, exists := labels.names[label]; exists && name != item.Name {
			labels.ambiguous[label] = true
		} else {
			labels.names[label] = item.Name
		}
	}
	for _, item := range inventory.Items {
		var aliases []string
		if item.State != nil && item.State.VPPName != nil {
			aliases = append(aliases, *item.State.VPPName)
		}
		if item.HostInventory != nil {
			aliases = append(aliases, item.HostInventory.Netdev)
		}
		for _, alias := range aliases {
			if _, exists := labels.labels[alias]; alias != "" && !exists {
				labels.labels[alias] = labels.labels[item.Name]
			}
		}
	}
	return labels
}

// interfaceAlias uses the longest known parent for numeric VLAN suffixes. It
// deliberately leaves unrelated names and unknown interface identities alone.
var interfaceVLANSuffix = regexp.MustCompile(`^(?:\.[0-9]+)+$`)

func interfaceAliasKey(values map[string]string, name string) (string, string, bool) {
	if _, ok := values[name]; ok {
		return name, "", true
	}
	for end := strings.LastIndexByte(name, '.'); end > 0; end = strings.LastIndexByte(name[:end], '.') {
		if !interfaceVLANSuffix.MatchString(name[end:]) {
			return "", "", false
		}
		if _, ok := values[name[:end]]; ok {
			return name[:end], name[end:], true
		}
	}
	return "", "", false
}

func (labels interfaceLabels) label(name string) string {
	if key, suffix, ok := interfaceAliasKey(labels.labels, name); ok {
		return labels.labels[key] + suffix
	}
	return name
}

func (labels interfaceLabels) resolve(label string) (name string, unambiguous bool) {
	if key, suffix, ok := interfaceAliasKey(labels.names, label); ok {
		if labels.ambiguous[key] {
			return "", false
		}
		return labels.names[key] + suffix, true
	}
	return label, true
}
