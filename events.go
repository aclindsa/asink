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
	//the state of the event on the local asink instance on which it originated:
	NOTICED       = 1 << iota //watcher.go has been notified that a file changed
	COPIED_TO_TMP             //temporary version saved off
	HASHED                    //hash taken of tmp file
	CACHED                    //tmp file renamed to its hash
	UPLOADED                  //tmp file has been successfully uploaded to storage
	ON_SERVER                 //server has been successfully notified of event
	//the state of the event on the asink instance notified that it occurred elsewhere
	NOTIFIED   //we've been told a file has been changed remotely
	DOWNLOADED //event has been downloaded and stored in the local file cache
	SYNCED     //everything has been done to ensure the affected file is up-to-date
)

type Event struct {
	Id          int64
	LocalId     int64
	Type        EventType
	Status      EventStatus
	Path        string
	Hash        string
	Predecessor string
	Timestamp   int64
	Permissions uint32
	InDB        bool `json:"-"` //defaults to false. Omitted from json marshalling.
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
