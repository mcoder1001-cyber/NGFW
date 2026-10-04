package isis

import "ngfw/agent/internal/renderers/frr"

// DatabaseReader identifies the on-demand IS-IS database state reader.
const DatabaseReader = "isisDatabase"

// ShowDatabase reads the IS-IS database across VRFs as JSON.
const ShowDatabase frr.ShowCommand = "show isis vrf all database json"

func init() {
	frr.RegisterStateReader(frr.StateReader{Key: DatabaseReader, Command: ShowDatabase, OnDemand: true})
}
