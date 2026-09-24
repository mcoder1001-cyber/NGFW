module ngfw/test/topology/interfaces

go 1.26

require (
	go.fd.io/govpp v0.13.0
	golang.org/x/net v0.57.0
	ngfw/agent v0.0.0
)

replace ngfw/agent => ../../../apps/agent
