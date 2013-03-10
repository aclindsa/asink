package main

import (
	"asink"
	"sync"
	"time"
)

type LongPollGroup struct {
	channels []*chan *asink.Event
	lock     sync.Mutex
}

type PollingManager struct {
	lock   sync.RWMutex
	groups map[string]*LongPollGroup
}

var pm *PollingManager

func init() {
	pm = new(PollingManager)
	pm.groups = make(map[string]*LongPollGroup)
}

func addPoller(uid string, channel *chan *asink.Event) {
	pm.lock.RLock()

	group := pm.groups[uid]
	if group != nil {
		group.lock.Lock()
		pm.lock.RUnlock()
		group.channels = append(group.channels, channel)
		group.lock.Unlock()
	} else {
		pm.lock.RUnlock()
		pm.lock.Lock()
		group = new(LongPollGroup)
		group.channels = append(group.channels, channel)
		pm.groups[uid] = group
		pm.lock.Unlock()
	}

	//set timer to call function after one minute
	timeout := time.Duration(1)*time.Minute
	time.AfterFunc(timeout, func() {
		group.lock.Lock()
		for i, c := range group.channels {
			if c == channel {
				copy(group.channels[i:], group.channels[i+1:])
				group.channels = group.channels[:len(group.channels)-1]
				break
			}
		}
		group.lock.Unlock()
		close(*channel)
	})
}

func broadcastToPollers(uid string, event *asink.Event) {
	//store off the long polling group we're trying to send to and remove
	//it from PollingManager.groups
	pm.lock.Lock()
	group := pm.groups[uid]
	pm.groups[uid] = nil
	pm.lock.Unlock()

	//send event down each of group's channels
	if group != nil {
		group.lock.Lock()
		for _, c := range group.channels {
			*c <- event
		}
		group.lock.Unlock()
	}
}
