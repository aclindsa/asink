package asink

const API_VERSION_STRING = "0.1"

type APIStatus uint32

const (
	SUCCESS = 0 + iota
	ERROR
)

type APIResponse struct {
	Status      APIStatus
	Explanation string
	Events      []*Event
}

type EventList struct {
	Events []*Event
}
