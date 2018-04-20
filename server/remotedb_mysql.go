package main

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLIdentifier is the identifier used for DB registration
const mysqlIdentifier = "mysql"

func init() {
	remoteDBRegister[mysqlIdentifier] = func() remoteDBSetup { return new(mysqlDB) }
}

// mysqlDB is the object interface for logging values to
// a remote database for storage
type mysqlDB struct {
	// MysqlConfig is the config for using mysql databases
	// as the backend storage for the remoteDB
	mysqlConfig struct {
		DSN string `toml:"dsn"`
	} `toml:"mysql"`

	keys map[string]struct{}

	*sql.DB
}

const mysqlDatabaseCreate = `CREATE DATABASE IF NOT EXISTS incrr;`

// mysqlTableCreate is the SQL for setting up a
// database table on startup
const mysqlTableCreate = `
CREATE TABLE IF NOT EXISTS keys (
	id INT UNSIGNED NOT NULL AUTO_INCREMENT,
	namespace TINYTEXT NOT NULL,
	value BIGINT NOT NULL,
	created DATETIME NOT NULL,
	PRIMARY KEY (id),
	INDEX (namespace(255))
)
ENGINE=InnoDB;`

// Setup does the setup of the remoteDB
func (m *mysqlDB) Setup(config *configuration) remoteDB {

	if v, ok := config.Datastore.RemoteOptions[mysqlIdentifier]; ok {
		if dsn, ok := v["dsn"].(string); ok {
			m.mysqlConfig.DSN = strings.TrimSpace(dsn)
		}
	}

	m.keys = make(map[string]struct{})

	var err error
	m.DB, err = sql.Open("mysql", m.mysqlConfig.DSN)
	log.OnErr(err).Fatalf("[mysql] connect: %v", err)

	err = m.DB.Ping()
	log.OnErr(err).Fatalf("[mysql] ping: %v", err)

	stmt, err := m.DB.Prepare(strings.TrimSpace(mysqlTableCreate))
	log.OnErr(err).Fatalf("[mysql] create prep: %v", err)

	_, err = stmt.Exec()
	log.OnErr(err).Fatalf("[mysql] create exec: %v", err)

	return m
}

// configDisplay shows the configuration for the MySQL database
func (m *mysqlDB) configDisplay(padd int, config *configuration) {
	display.Printf(leftpad(padd, "[config] Use RemoteDB:", "%v"), config.Datastore.UseRemoteDB)
	display.Printf(leftpad(padd, "[config] RemoteDB DSN:", "%v"), m.mysqlConfig.DSN)

	k, _ := m.Keys()
	display.Printf(leftpad(padd, "[config] RemoteDB Keys:", "%v"), k)
}

// HashKey returns if the key is in the DB
func (m *mysqlDB) HasKey(key string) bool {
	_, ok := m.keys[key]
	return ok
}

// Keys is the list of unique keys (namespaces) found within the database
func (m *mysqlDB) Keys() (out []string, err error) {
	sql := "SELECT DISTINCT `namespace` FROM `keys`"
	rows, err := m.DB.Query(sql)
	if err != nil {
		return nil, fmt.Errorf("[mysql] keys: %v", err)
	}
	for rows.Next() {
		var ns string
		err = rows.Scan(&ns)
		if err != nil {
			return nil, fmt.Errorf("[mysql] keys row: %v", err)
		}
		out = append(out, ns)
		m.keys[ns] = struct{}{}
	}
	return out, err
}

// Get returns the value for a given key (namespace)
func (m *mysqlDB) Get(key []byte) ([]byte, error) {
	sql := "SELECT MAX(`value`) AS `current` FROM `keys` WHERE `namespace`=?"
	rows, err := m.DB.Query(sql, key)
	if err != nil {
		return nil, fmt.Errorf("[mysql] get: %v", err)
	}
	rows.Next() // have to pull in the value to be scanned

	var val string
	rows.Scan(&val)
	return []byte(val), nil
}

// Set sets the value for a given key (namespace)
func (m *mysqlDB) Set(key, val []byte) error {
	sql := "INSERT INTO `keys` (`namespace`, `value`, `created`) VALUES (?, ?, ?)"
	stmt, err := m.DB.Prepare(sql)
	if err != nil {
		return fmt.Errorf("[mysql] set: %v", err)
	}
	v, err := strconv.ParseUint(string(val), 10, 64)
	if err != nil {
		return fmt.Errorf("[mysql] set parse: %v", err)
	}
	_, err = stmt.Exec(key, v, time.Now())
	if err != nil {
		return fmt.Errorf("[mysql] set result: %v", err)
	}
	return nil
}
