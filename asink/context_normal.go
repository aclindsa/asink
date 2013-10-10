/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"asink"
)

type NormalContext struct {
	globals           *AsinkGlobals
	localUpdatesChan  chan *asink.Event
	remoteUpdatesChan chan *asink.Event
	exitChan          chan int
}

func NewNormalContext(globals *AsinkGlobals, localChan chan *asink.Event, remoteChan chan *asink.Event, exitChan chan int) *NormalContext {
	nc := new(NormalContext)
	nc.globals = globals
	nc.localUpdatesChan = localChan
	nc.remoteUpdatesChan = remoteChan
	nc.exitChan = exitChan
	return nc
}

func (nc *NormalContext) Run() error {
	for {
		select {
		case event := <-nc.localUpdatesChan:
			//process top half of local event
			err := ProcessLocalEvent_Upper(nc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					return err
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessLocalEvent_Upper(nc.globals, event)
					if err != nil {
						return err
					}
				}
			}
			if event.LocalStatus&asink.DISCARDED != 0 {
				continue
			}
			//process bottom half of local event
			err = ProcessLocalEvent_Lower(nc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					return err
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessLocalEvent_Lower(nc.globals, event)
					if err != nil {
						return err
					}
				}
			}
		case event := <-nc.remoteUpdatesChan:
			err := ProcessRemoteEvent(nc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					return err
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessRemoteEvent(nc.globals, event)
					if err != nil {
						return err
					}
				}
			}
		case <-nc.exitChan:
			return ProcessingError{EXITED, nil}
		}
	}
}
