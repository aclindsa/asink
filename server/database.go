package main

import (
	"asink"
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"strconv"
)

func GetAndInitDB() (*sql.DB, error) {
	dbLocation := "asink-server.db" //TODO make me configurable

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
		tx.Exec("CREATE TABLE events (id INTEGER PRIMARY KEY ASC, localid INTEGER, type INTEGER, status INTEGER, path TEXT, hash TEXT, timestamp INTEGER, permissions INTEGER);")
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
	result, err := tx.Exec("INSERT INTO events (localid, type, status, path, hash, timestamp, permissions) VALUES (?,?,?,?,?,?,?);", e.LocalId, e.Type, e.Status, e.Path, e.Hash, e.Timestamp, e.Permissions)
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

	e.Id = id
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
	result, err := tx.Exec("UPDATE events SET id=?, type=?, status=?, path=?, hash=?, timestamp=?, permissions=? WHERE id=?;", e.Id, e.Type, e.Status, e.Path, e.Hash, e.Timestamp, e.Permissions, e.Id)
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

func DatabaseRetrieveEvents(db *sql.DB, firstId uint64, maxEvents uint) ([]*asink.Event, error) {
	var events []*asink.Event

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query("SELECT id, localid, type, status, path, hash, timestamp, permissions FROM events WHERE id >= ? ORDER BY id ASC LIMIT ?;", firstId, maxEvents)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var event asink.Event
		err = rows.Scan(&event.Id, &event.LocalId, &event.Type, &event.Status, &event.Path, &event.Hash, &event.Timestamp, &event.Permissions)
		if err != nil {
			return nil, err
		}
		events = append(events, &event)
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return events, nil
}
