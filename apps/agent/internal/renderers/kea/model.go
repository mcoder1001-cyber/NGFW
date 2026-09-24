package kea

// Typed Kea 3.0 configuration. The renderer builds these structs and marshals them with
// encoding/json (native escaping, HTML escaping off): no user string is ever spliced into
// JSON text by hand. Field order is the struct order, slices are sorted by the builder and
// the only map (user-context) is marshalled with sorted keys, so the output is deterministic.

type dhcp4Root struct {
	Dhcp4 serverConfig `json:"Dhcp4"`
}

type dhcp6Root struct {
	Dhcp6 serverConfig `json:"Dhcp6"`
}

// serverConfig is the common shape of Dhcp4 / Dhcp6; family-specific fields are omitempty.
type serverConfig struct {
	InterfacesConfig interfacesConfig `json:"interfaces-config"`
	ControlSockets   []controlSocket  `json:"control-sockets"`
	LeaseDatabase    leaseDatabase    `json:"lease-database"`
	HooksLibraries   []hookLibrary    `json:"hooks-libraries,omitempty"`
	OptionDef        []optionDef      `json:"option-def,omitempty"`
	Subnet4          *[]subnet        `json:"subnet4,omitempty"`
	Subnet6          *[]subnet        `json:"subnet6,omitempty"`
	Loggers          []logger         `json:"loggers"`
	// UserContext carries the render input (input.go); absent in an idle configuration.
	UserContext *topContext `json:"user-context,omitempty"`
}

type interfacesConfig struct {
	Interfaces     []string `json:"interfaces"`
	DHCPSocketType string   `json:"dhcp-socket-type,omitempty"`
	ReDetect       bool     `json:"re-detect"`
}

type controlSocket struct {
	SocketType string `json:"socket-type"`
	SocketName string `json:"socket-name"`
}

type leaseDatabase struct {
	Type        string `json:"type"`
	Persist     bool   `json:"persist"`
	Name        string `json:"name"`
	LFCInterval uint32 `json:"lfc-interval"`
}

type hookLibrary struct {
	Library string `json:"library"`
}

type optionDef struct {
	Name  string `json:"name"`
	Code  uint32 `json:"code"`
	Type  string `json:"type"`
	Space string `json:"space"`
}

type optionData struct {
	Name       string `json:"name,omitempty"`
	Code       uint32 `json:"code,omitempty"`
	Space      string `json:"space,omitempty"`
	CSVFormat  *bool  `json:"csv-format,omitempty"`
	Data       string `json:"data"`
	AlwaysSend bool   `json:"always-send,omitempty"`
}

type pool struct {
	Pool string `json:"pool"`
}

type reservation struct {
	HWAddress   string       `json:"hw-address,omitempty"`
	DUID        string       `json:"duid,omitempty"`
	IPAddress   string       `json:"ip-address,omitempty"`
	IPAddresses []string     `json:"ip-addresses,omitempty"`
	Hostname    string       `json:"hostname,omitempty"`
	OptionData  []optionData `json:"option-data,omitempty"`
	UserContext *userContext `json:"user-context,omitempty"`
}

type subnet struct {
	ID                uint32        `json:"id"`
	Subnet            string        `json:"subnet"`
	Interface         string        `json:"interface,omitempty"`
	Pools             []pool        `json:"pools"`
	OptionData        []optionData  `json:"option-data,omitempty"`
	Reservations      []reservation `json:"reservations,omitempty"`
	ValidLifetime     uint32        `json:"valid-lifetime"`
	PreferredLifetime uint32        `json:"preferred-lifetime,omitempty"`
	RenewTimer        uint32        `json:"renew-timer,omitempty"`
	RebindTimer       uint32        `json:"rebind-timer,omitempty"`
	Authoritative     *bool         `json:"authoritative,omitempty"`
	UserContext       *userContext  `json:"user-context,omitempty"`
}

// userContext carries the document names back through config-get (Retrieve maps Kea's
// subnets to services.dhcp.servers.<server>.subnets.<subnet> with it).
type userContext struct {
	VRX vrxContext `json:"vrx"`
}

type vrxContext struct {
	Server      string `json:"server,omitempty"`
	Subnet      string `json:"subnet,omitempty"`
	Reservation string `json:"reservation,omitempty"`
	Description string `json:"description,omitempty"`
	// ServerDescription is the owning server's description (Kea has no server object).
	ServerDescription string `json:"server-description,omitempty"`
}

type logger struct {
	Name          string         `json:"name"`
	OutputOptions []outputOption `json:"output-options"`
	Severity      string         `json:"severity"`
}

type outputOption struct {
	Output  string `json:"output"`
	MaxSize uint64 `json:"maxsize"`
	MaxVer  uint32 `json:"maxver"`
}
