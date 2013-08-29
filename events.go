package asink

import (
	"os"
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
	//Local event status flags
	DISCARDED = 1 << iota //event is to be discarded because it errored or is duplicate
)

type Event struct {
	Id          int64
	Type        EventType
	Path        string
	Hash        string
	Predecessor string
	Timestamp   int64
	Permissions os.FileMode
	Username    string
	Sharename   string      //TODO start differentiating between a users' different shares
	LocalStatus EventStatus `json:"-"`
	LocalId     int64       `json:"-"`
	InDB        bool        `json:"-"` //defaults to false. Omitted from json marshalling.
}

func (e *Event) IsUpdate() bool {
	return e.Type&UPDATE == UPDATE
}

func (e *Event) IsDelete() bool {
	return e.Type&DELETE == DELETE
}

func (e *Event) IsSameEvent(e2 *Event) bool {
	return (e.Type == e2.Type && e.Path == e2.Path && e.Hash == e2.Hash && e.Predecessor == e2.Predecessor && e.Timestamp == e2.Timestamp && e.Permissions == e2.Permissions)
}
