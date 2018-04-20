package main

import (
	"io/ioutil"
	"os"
	"strconv"

	"github.com/boltdb/bolt"
)

// localDB stores values atomically on the local server for fast lookup
type localDB struct {
	BoltDBConfig struct {
		DSN             string `toml:"dsn"`
		BucketName      string `toml:"bucket"`
		DelDBOnShutdown bool   `toml:"cleanup_db_file"` // this has a default in setup (change the keyname there if it changes here)
	} `toml:"bolt"`

	db *bolt.DB
}

// Get returns the value for a given key (namespace)
func (l localDB) Get(key []byte) (val []byte) {
	l.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(l.BoltDBConfig.BucketName))
		val = b.Get(key)
		return nil
	})
	return val
}

// Set sets the value for a given key (namespace)
func (l localDB) Set(key, val []byte) error {
	return l.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(l.BoltDBConfig.BucketName))
		err := b.Put(key, val)
		return err
	})
}

// Incr sets the value for a given key (namespace), and
// makes sure that it is larger than the previous value
func (l localDB) Incr(key, val []byte) error {
	v := l.Get(key)
	if len(v) == 0 {
		return l.Set(key, val)
	}
	num, err := strconv.ParseUint(string(val), 10, 64)
	if err != nil {
		return ErrParseUint64(err)
	}
	n, err := strconv.ParseUint(string(v), 10, 64)
	if err != nil {
		return ErrParseUint64(err)
	}

	if num > n {
		return l.Set(key, val)
	}

	return errNumLessThan
}

// Shutdown is a hook that can be called on server shutdown
func (l *localDB) Shutdown() error {
	if l.BoltDBConfig.DelDBOnShutdown {
		return os.Remove(l.db.Path())
	}
	return nil
}

// configDisplay shows the configuration for the local database
func (l *localDB) configDisplay(padd int, config *configuration) {
	display.Printf(leftpad(padd, "[config] LocalDB Bolt DSN:", "%v"), l.BoltDBConfig.DSN)
	display.Printf(leftpad(padd, "[config] LocalDB Bolt Bucket:", "%v"), l.BoltDBConfig.BucketName)
	display.Printf(leftpad(padd, "[config] LocalDB Bolt Shutdown:", "%v"), l.BoltDBConfig.DelDBOnShutdown)
}

// setupLocalDB sets up the object for the local database
func setupLocalDB(config *configuration) *localDB {
	l := config.Datastore.LocalDB
	if l == nil {
		l = new(localDB)
	}

	if len(l.BoltDBConfig.DSN) == 0 {
		tmpfile, err := ioutil.TempDir("", "db-")
		if err != nil {
			log.Fatalf("[localDB] temp file: %v", err)
		}
		l.BoltDBConfig.DSN = "file://" + tmpfile + "/default.db"
	}

	if len(l.BoltDBConfig.BucketName) == 0 {
		l.BoltDBConfig.BucketName = defaultBucketName
	}

	// // set the default to delete the DB on shutdown to true
	// // for the dev environment
	// if (config.Environment == "" || config.Environment == "dev") && !metadata.IsDefined("local_datastore", "boltdb_cleanup_db_file") {
	// 	l.BoltDBConfig.DelDBOnShutdown = true
	// }

	dsn, err := parseFileDSN(l.BoltDBConfig.DSN)
	if err != nil {
		log.Fatalf("[localDB] parse dsn: %v", err)
	}

	// Open the database.
	l.db, err = bolt.Open(dsn.filename, 0666, nil)
	if err != nil {
		log.Fatal(err)
	}

	config.internal.shutdown = append(config.internal.shutdown, l)

	// Start a write transaction.
	if err := l.db.Update(func(tx *bolt.Tx) error {
		// Create a bucket.
		_, err = tx.CreateBucketIfNotExists([]byte(l.BoltDBConfig.BucketName))
		if err != nil {
			return err
		}
		return nil

	}); err != nil {
		log.Fatal(err)
	}

	return l
}
