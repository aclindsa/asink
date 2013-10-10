/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"asink"
	"github.com/howeyc/fsnotify"
	"os"
	"path/filepath"
	"time"
)

func StartWatching(watchDir string, fileUpdates chan *asink.Event, initialWalkComplete chan int) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic("Failed to create fsnotify watcher")
	}

	//function called by filepath.Walk to start watching a directory and all subdirectories
	watchDirFn := func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			err = watcher.Watch(path)
			if err != nil {
				panic("Failed to watch " + path)
			}
		} else if info.Mode().IsRegular() {
			event := new(asink.Event)
			event.Path = path
			event.Type = asink.UPDATE
			event.Timestamp = time.Now().UnixNano()
			fileUpdates <- event
		}
		return nil
	}

	//processes all the fsnotify events into asink events
	go func() {
		for {
			select {
			case ev := <-watcher.Event:
				//if a directory was created, begin recursively watching all its subdirectories
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					if ev.IsCreate() {
						//Note: even though filepath.Walk will visit root, we must watch root first so we catch files/directories created after the walk begins but before this directory begins being watched
						err = watcher.Watch(ev.Name)
						if err != nil {
							panic("Failed to watch " + ev.Name)
						}
						//scan this directory to ensure any file events we missed before starting to watch this directory are caught
						filepath.Walk(ev.Name, watchDirFn)
					}
					continue
				}

				event := new(asink.Event)
				if ev.IsCreate() || ev.IsModify() {
					event.Type = asink.UPDATE
				} else if ev.IsDelete() || ev.IsRename() {
					event.Type = asink.DELETE
				} else {
					panic("Unknown fsnotify event type")
				}

				event.Path = ev.Name
				event.Timestamp = time.Now().UnixNano()

				fileUpdates <- event

			case err := <-watcher.Error:
				panic(err)
			}
		}
	}()

	//start watching the directory passed in
	filepath.Walk(watchDir, watchDirFn)
	initialWalkComplete <- 0
}
