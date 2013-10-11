/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"github.com/aclindsa/asink"
)

const NUM_NORMAL_WORKERS = 50

type NormalContext struct {
	globals           *AsinkGlobals
	localUpdatesChan  chan *asink.Event
	remoteUpdatesChan chan *asink.Event
	exitChan          chan int
	workerError       chan error
	workerExit        chan int
	workerExited      chan int
}

func NewNormalContext(globals *AsinkGlobals, localChan chan *asink.Event, remoteChan chan *asink.Event, exitChan chan int) *NormalContext {
	nc := new(NormalContext)
	nc.globals = globals
	nc.localUpdatesChan = localChan
	nc.remoteUpdatesChan = remoteChan
	nc.exitChan = exitChan
	return nc
}

func (nc *NormalContext) runWorker() {
	defer func() { nc.workerExited <- 0 }()
	for {
		select {
		case event := <-nc.localUpdatesChan:
			//process top half of local event
			err := ProcessLocalEvent(nc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					nc.workerError <- err
					continue
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessLocalEvent(nc.globals, event)
					if err != nil {
						nc.workerError <- err
						continue
					}
				}
			}
		case event := <-nc.remoteUpdatesChan:
			err := ProcessRemoteEvent(nc.globals, event)
			if err != nil {
				if e, ok := err.(ProcessingError); !ok || e.ErrorType != TEMPORARY {
					nc.workerError <- err
					continue
				} else {
					//if error was temporary, retry once
					event.LocalStatus = 0
					err := ProcessRemoteEvent(nc.globals, event)
					if err != nil {
						nc.workerError <- err
						continue
					}
				}
			}
		case <-nc.workerExit:
			return
		}
	}
}
func (nc *NormalContext) Run() error {
	nc.workerExit = make(chan int)
	nc.workerExited = make(chan int)
	nc.workerError = make(chan error)

	//start all the goroutines
	for i := 0; i < NUM_NORMAL_WORKERS; i++ {
		go nc.runWorker()
	}

	var err error
	//wait until an error or we exit
	select {
	case <-nc.exitChan:
		err = ProcessingError{EXITED, nil}
	case err = <-nc.workerError:
	}

	//notify all the goroutines we're exiting
	for i := 0; i < NUM_NORMAL_WORKERS; i++ {
		nc.workerExit <- 0
	}

	//wait until they're finished
	for i := 0; i < NUM_NORMAL_WORKERS; i++ {
		<-nc.workerExited
	}

	return err
}
