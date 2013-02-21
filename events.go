package asink

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
	Id        int64
	LocalId   int64
	Type      EventType
	Status    EventStatus
	Path      string
	Hash      string
	Timestamp int64
	InDB      bool `json:"-"` //defaults to false. Omitted from json marshalling.
}

func (e Event) IsUpdate() bool {
	return e.Type&UPDATE == UPDATE
}

func (e Event) IsDelete() bool {
	return e.Type&DELETE == DELETE
}

type EventList struct {
	Events []*Event
}
