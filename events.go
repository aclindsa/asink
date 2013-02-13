package main

import (
	"time"
)

//event type
type EventType uint32

const (
	UPDATE = 1 << iota
	DELETE
)

//event status
type EventStatus uint32

const (
	NOTICED       = 1 << iota //watcher.go has been notified that a file changed
	COPIED_TO_TMP             //temporary version saved off
	HASHED                    //hash taken of tmp file
	CACHED                    //tmp file renamed to its hash
	UPLOADED                  //tmp file has been successfully uploaded to storage
	ON_SERVER                 //server has been successfully notified of event
)

type Event struct {
	Type      EventType
	Status    EventStatus
	Path      string
	Hash      string
	Timestamp time.Time
}

func (e Event) IsUpdate() bool {
	return e.Type&UPDATE == UPDATE
}

func (e Event) IsDelete() bool {
	return e.Type&DELETE == DELETE
}
