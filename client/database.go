package main

import (
	"asink"
	"code.google.com/p/goconf/conf"
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"strconv"
	"sync"
)

type AsinkDB struct {
	db   *sql.DB
	lock sync.Mutex
}

func GetAndInitDB(config *conf.ConfigFile) (*AsinkDB, error) {
	dbLocation, err := config.GetString("local", "dblocation")
	if err != nil {
		return nil, errors.New("Error: database location not specified in config file.")
	}

	db, err := sql.Open("sqlite3", dbLocation)
	if err != nil {
		return nil, err
	}

	//make sure the events table is created
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='events';")
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		//if this is false, it means no rows were returned
		tx.Exec("CREATE TABLE events (id INTEGER, localid INTEGER PRIMARY KEY ASC, type INTEGER, status INTEGER, path TEXT, hash TEXT, timestamp INTEGER, permissions INTEGER);")
		//		tx.Exec("CREATE INDEX IF NOT EXISTS localididx on events (localid)")
		tx.Exec("CREATE INDEX IF NOT EXISTS ididx on events (id);")
		tx.Exec("CREATE INDEX IF NOT EXISTS pathidx on events (path);")
	}
	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	ret := new(AsinkDB)
	ret.db = db
	return ret, nil
}

func (adb *AsinkDB) DatabaseAddEvent(e *asink.Event) (err error) {
	adb.lock.Lock()
	tx, err := adb.db.Begin()
	if err != nil {
		return err
	}
	//make sure the transaction gets rolled back on error, and the database gets unlocked
	defer func() {
		if err != nil {
			tx.Rollback()
		}
		adb.lock.Unlock()
	}()

	result, err := tx.Exec("INSERT INTO events (id, type, status, path, hash, timestamp, permissions) VALUES (?,?,?,?,?,?,?);", e.Id, e.Type, e.Status, e.Path, e.Hash, e.Timestamp, e.Permissions)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	e.LocalId = id
	e.InDB = true
	return nil
}

func (adb *AsinkDB) DatabaseUpdateEvent(e *asink.Event) (err error) {
	if !e.InDB {
		return errors.New("Attempting to update an event in the database which hasn't been previously added.")
	}

	adb.lock.Lock()
	tx, err := adb.db.Begin()
	if err != nil {
		return err
	}
	//make sure the transaction gets rolled back on error, and the database gets unlocked
	defer func() {
		if err != nil {
			tx.Rollback()
		}
		adb.lock.Unlock()
	}()

	result, err := tx.Exec("UPDATE events SET id=?, type=?, status=?, path=?, hash=?, timestamp=?, permissions=? WHERE localid=?;", e.Id, e.Type, e.Status, e.Path, e.Hash, e.Timestamp, e.Permissions, e.LocalId)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("Updated " + strconv.Itoa(int(rows)) + " row(s) when intending to update 1 event row.")
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

//returns nil if no such event exists
func (adb *AsinkDB) DatabaseLatestRemoteEvent() (event *asink.Event, err error) {
	adb.lock.Lock()
	//make sure the database gets unlocked
	defer adb.lock.Unlock()

	rows, err := adb.db.Query("SELECT id, localid, type, status, path, hash, timestamp, permissions FROM events WHERE id > 0 ORDER BY id DESC LIMIT 1;")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		event = new(asink.Event)
		err = rows.Scan(&event.Id, &event.LocalId, &event.Type, &event.Status, &event.Path, &event.Hash, &event.Timestamp, &event.Permissions)
		if err != nil {
			return nil, err
		}
		return event, nil
	}
	return nil, nil
}
