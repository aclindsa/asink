/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"github.com/aclindsa/asink"
	"time"
)

type StartupContext struct {
	globals             *AsinkGlobals
	localUpdatesChan    chan *asink.Event
	remoteUpdatesChan   chan *asink.Event
	initialWalkComplete chan int
	exitChan            chan int
}

func NewStartupContext(globals *AsinkGlobals, localChan chan *asink.Event, remoteChan chan *asink.Event, initialWalkComplete chan int, exitChan chan int) *StartupContext {
	sc := new(StartupContext)
	sc.globals = globals
	sc.localUpdatesChan = localChan
	sc.remoteUpdatesChan = remoteChan
	sc.initialWalkComplete = initialWalkComplete
	sc.exitChan = exitChan
	return sc
}

func (sc *StartupContext) Run() error {
	//process top halves of local updates so the files are saved off at least locally
	localEvents := []*asink.Event{}
	initialWalkIncomplete := true
	for initialWalkIncomplete {
		select {
		case event := <-sc.localUpdatesChan:
			//process top half of local event
			err := ProcessLocalEvent_Upper(sc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					return err
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessLocalEvent_Upper(sc.globals, event)
					if err != nil {
						return err
					}
				}
			}
			if event.LocalStatus&asink.DISCARDED == 0 {
				localEvents = append(localEvents, event)
			}
		case <-sc.initialWalkComplete:
			initialWalkIncomplete = false
		case <-sc.exitChan:
			return ProcessingError{EXITED, nil}
		}
	}

	//then process remote events (possibly taking a break whenever a local one comes in to process it)
	timeout := time.NewTimer(1 * time.Second)
	timedOut := false
	for !timedOut {
		select {
		case event := <-sc.localUpdatesChan:
			//process top half of local event
			err := ProcessLocalEvent_Upper(sc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					return err
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessLocalEvent_Upper(sc.globals, event)
					if err != nil {
						return err
					}
				}
			}
			if event.LocalStatus&asink.DISCARDED == 0 {
				localEvents = append(localEvents, event)
			}
			timeout.Reset(1 * time.Second)
		case event := <-sc.remoteUpdatesChan:
			err := ProcessRemoteEvent(sc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					return err
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessRemoteEvent(sc.globals, event)
					if err != nil {
						return err
					}
				}
			}
			timeout.Reset(1 * time.Second)
		case <-timeout.C:
			timedOut = true
		case <-sc.exitChan:
			return ProcessingError{EXITED, nil}
		}
	}

	for _, event := range localEvents {
		//make sure we don't need to exit
		select {
		case <-sc.exitChan:
			return ProcessingError{EXITED, nil}
		default:
		}
		err := ProcessLocalEvent_Lower(sc.globals, event)
		if err != nil {
			if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
				return err
			} else {
				//if error was temporary, retry once
				event.LocalStatus = 0
				err := ProcessLocalEvent_Lower(sc.globals, event)
				if err != nil {
					return err
				}
			}
		}
	}

	//finally, process the bottom halves of local updates
	return nil
}
