package main

import (
	"asink"
	"asink/server"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"sync"
)

type AsinkDB struct {
	db   *sql.DB
	lock sync.Mutex
}

func GetAndInitDB() (*AsinkDB, error) {
	dbLocation := "asink-server.db" //TODO make me configurable

	db, err := sql.Open("sqlite3", "file:"+dbLocation+"?cache=shared&mode=rwc")
	if err != nil {
		return nil, err
	}

	//make sure all the tables are created
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
		tx.Exec("CREATE TABLE events (id INTEGER PRIMARY KEY ASC, userid INTEGER, type INTEGER, path TEXT, hash TEXT, predecessor TEXT, timestamp INTEGER, permissions INTEGER);")
		tx.Exec("CREATE INDEX IF NOT EXISTS pathidx on events (path);")
		tx.Exec("CREATE INDEX IF NOT EXISTS timestampidx on events (timestamp);")
	} else {
		rows.Close()
	}

	rows, err = tx.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='users';")
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		//if this is false, it means no rows were returned
		tx.Exec("CREATE TABLE user (id INTEGER PRIMARY KEY ASC, username TEXT, pwhash TEXT, role INTEGER);")
	} else {
		rows.Close()
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

	result, err := tx.Exec("INSERT INTO events (userid, type, path, hash, predecessor, timestamp, permissions) VALUES (?,?,?,?,?,?,?,?);", e.Type, e.Path, e.Hash, e.Predecessor, e.Timestamp, e.Permissions)
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

func (adb *AsinkDB) DatabaseRetrieveEvents(firstId uint64, maxEvents uint) (events []*asink.Event, err error) {
	adb.lock.Lock()
	//make sure the database gets unlocked on return
	defer func() {
		adb.lock.Unlock()
	}()
	rows, err := adb.db.Query("SELECT id, type, path, hash, predecessor, timestamp, permissions FROM events WHERE id >= ? ORDER BY id ASC LIMIT ?;", firstId, maxEvents)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var event asink.Event
		err = rows.Scan(&event.Id, &event.Type, &event.Path, &event.Hash, &event.Predecessor, &event.Timestamp, &event.Permissions)
		if err != nil {
			return nil, err
		}
		events = append(events, &event)
	}

	return events, nil
}

func (adb *AsinkDB) DatabaseAddUser(u *server.User) (err error) {
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

	result, err := tx.Exec("INSERT INTO users (username, pwhash, role) VALUES (?,?);", u.Username, u.PWHash, u.Role)
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

	u.Id = id
	return nil
}

func (adb *AsinkDB) DatabaseGetUser(username string) (user *server.User, err error) {
	adb.lock.Lock()
	//make sure the database gets unlocked
	defer adb.lock.Unlock()

	row := adb.db.QueryRow("SELECT id, username, pwhash, role FROM users WHERE username == ?;", username)

	user = new(server.User)
	err = row.Scan(&user.Id, &user.Username, &user.PWHash, &user.Role)

	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return user, nil
	}
}


