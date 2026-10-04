package isis

import "ngfw/agent/internal/renderers/frr"

const DatabaseReader = "isisDatabase"
const ShowDatabase frr.ShowCommand = "show isis vrf all database json"

func init() {
	frr.RegisterStateReader(frr.StateReader{Key: DatabaseReader, Command: ShowDatabase, OnDemand: true})
}
