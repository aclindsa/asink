package main

type EventType uint32

const (
	UPDATE  = 1 << iota
	DELETE
)

type Event struct {
	Type EventType
	Path string
	Hash string
}

func (e Event) IsUpdate() bool {
	return e.Type & UPDATE == UPDATE
}

func (e Event) IsDelete() bool {
	return e.Type & DELETE == DELETE
}
