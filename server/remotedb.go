package main

import "sort"

// remoteDBRegister is the global registry for remoteDB implementations
var remoteDBRegister = map[string]func() remoteDBSetup{}

// remoteDB is the basic interface that a
// remoteDB implementation requires
type remoteDB interface {
	Keys() ([]string, error)
	HasKey(string) bool

	Get([]byte) ([]byte, error)
	Set([]byte, []byte) error

	remoteDBSetup
}

// remoteDBSetup is the interface for setting
// up a remoteDB object
type remoteDBSetup interface {
	Setup(*configuration) remoteDB
}

// configDisplay is for showing the remoteDB configuration
type configDisplay interface {
	configDisplay(int, *configuration)
}

// setupRemoteDB sets up the object for the remote database
func setupRemoteDB(config *configuration) remoteDB {
	m := config.Datastore.RemoteDB
	if m == nil {
		if len(config.Datastore.UseRemoteDB) == 0 {
			switch len(remoteDBRegister) {
			case 0:
				log.Fatalf("no remoteDB registered")
			case 1:
				for k := range remoteDBRegister {
					config.Datastore.UseRemoteDB = k
				}
			default:
				// fire the first one in lex order
				var order = make([]string, 0, len(remoteDBRegister))
				for k := range remoteDBRegister {
					order = append(order, k)
				}
				sort.Strings(order)
				config.Datastore.UseRemoteDB = order[0]
			}
		}

		registeredDB, ok := remoteDBRegister[config.Datastore.UseRemoteDB]
		if !ok {
			log.Fatalf("no remoteDB registered by the name: %s", config.Datastore.UseRemoteDB)
		}
		m = registeredDB().Setup(config)
	}
	return m
}
