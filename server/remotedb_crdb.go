package main

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/stdlib"
)

// crDBIdentifier is the identifier used for DB registration
// of the CoachroachDB
const crDBIdentifier = "crdb"

func init() {
	remoteDBRegister[crDBIdentifier] = func() remoteDBSetup { return new(crDB) }
}

// crDB is the object interface for logging values to
// a remote database for storage
type crDB struct {
	// CRDBConfig is the config for using cockroachDB databases
	// as the backend storage for the remoteDB
	crDBConfig struct {
		DSN string `toml:"dsn"`
	} `toml:"crdb"`

	keys map[string]struct{}

	*sql.DB
}

const crdbDatabaseCreate = `CREATE DATABASE IF NOT EXISTS incrr;`

// crDBTableCreate is the SQL for setting up a
// database table on startup
const crdbTableCreate = `
CREATE TABLE IF NOT EXISTS keys (
	id SERIAL PRIMARY KEY,
	namespace STRING NOT NULL,
	value INT NOT NULL,
	created TIMESTAMP NOT NULL,
	INDEX ns_idx (namespace)
);`

// Setup does the setup of the remoteDB
func (c *crDB) Setup(config *configuration) remoteDB {

	if v, ok := config.Datastore.RemoteOptions[crDBIdentifier]; ok {
		if dsn, ok := v["dsn"].(string); ok {
			c.crDBConfig.DSN = strings.TrimSpace(dsn)
		}
	}

	c.keys = make(map[string]struct{})

	var err error
	c.DB, err = sql.Open("pgx", c.crDBConfig.DSN)
	log.OnErr(err).Fatalf("[crdb] connect: %v", err)

	err = c.DB.Ping()
	log.OnErr(err).Fatalf("[crdb] ping: %v", err)

	stmtD, err := c.DB.Prepare(strings.TrimSpace(crdbDatabaseCreate))
	log.OnErr(err).Fatalf("[crdb] create db prep: %v", err)

	_, err = stmtD.Exec()
	log.OnErr(err).Fatalf("[crdb] create db exec: %v", err)

	stmtT, err := c.DB.Prepare(strings.TrimSpace(crdbTableCreate))
	log.OnErr(err).Fatalf("[crdb] create table prep: %v", err)

	_, err = stmtT.Exec()
	log.OnErr(err).Fatalf("[crdb] create table exec: %v", err)

	return c
}

// configDisplay shows the configuration for the CockraochDB database
func (c *crDB) configDisplay(padd int, config *configuration) {
	display.Printf(leftpad(padd, "[config] Use RemoteDB:", "%v"), config.Datastore.UseRemoteDB)
	display.Printf(leftpad(padd, "[config] RemoteDB DSN:", "%v"), c.crDBConfig.DSN)

	k, _ := c.Keys()
	display.Printf(leftpad(padd, "[config] RemoteDB Keys:", "%v"), k)
}

// HashKey returns if the key is in the DB
func (c *crDB) HasKey(key string) bool {
	_, ok := c.keys[key]
	return ok
}

// Keys is the list of unique keys (namespaces) found within the database
func (c *crDB) Keys() (out []string, err error) {
	sql := "SELECT DISTINCT namespace FROM keys"
	rows, err := c.DB.Query(sql)
	if err != nil {
		return nil, fmt.Errorf("[crdb] keys: %v", err)
	}
	for rows.Next() {
		var ns string
		err = rows.Scan(&ns)
		if err != nil {
			return nil, fmt.Errorf("[crdb] keys row: %v", err)
		}
		out = append(out, ns)
		c.keys[ns] = struct{}{}
	}
	return out, err
}

// Get returns the value for a given key (namespace)
func (c *crDB) Get(key []byte) ([]byte, error) {
	sql := "SELECT MAX(value) AS current FROM keys WHERE namespace=$1"
	rows, err := c.DB.Query(sql, key)
	if err != nil {
		return nil, fmt.Errorf("[crdb] get: %v", err)
	}
	rows.Next() // have to pull in the value to be scanned

	var val string
	rows.Scan(&val)
	return []byte(val), nil
}

// Set sets the value for a given key (namespace)
func (c *crDB) Set(key, val []byte) error {
	sql := "INSERT INTO keys (namespace, value, created) VALUES ($1, $2, $3)"
	stmt, err := c.DB.Prepare(sql)
	if err != nil {
		return fmt.Errorf("[crdb] set: %v", err)
	}
	v, err := strconv.ParseUint(string(val), 10, 64)
	if err != nil {
		return fmt.Errorf("[crdb] set parse: %v", err)
	}
	_, err = stmt.Exec(key, v, time.Now())
	if err != nil {
		return fmt.Errorf("[crdb] set result: %v", err)
	}
	return nil
}
