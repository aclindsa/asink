package main

import (
	"asink"
	"code.google.com/p/goconf/conf"
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"strconv"
)

func GetAndInitDB(config *conf.ConfigFile) (*sql.DB, error) {
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

	return db, nil
}

func DatabaseAddEvent(db *sql.DB, e *asink.Event) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	result, err := tx.Exec("INSERT INTO events (id, type, status, path, hash, timestamp, permissions) VALUES (?,?,?,?,?,?,?);", e.Id, e.Type, e.Status, e.Path, e.Hash, e.Timestamp, 0)
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

func DatabaseUpdateEvent(db *sql.DB, e *asink.Event) error {
	if !e.InDB {
		return errors.New("Attempting to update an event in the database which hasn't been previously added.")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	result, err := tx.Exec("UPDATE events SET id=?, type=?, status=?, path=?, hash=?, timestamp=?, permissions=? WHERE localid=?;", e.Id, e.Type, e.Status, e.Path, e.Hash, e.Timestamp, 0, e.LocalId)
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
