package main

import (
	"github.com/howeyc/fsnotify"
	"os"
	"path/filepath"
	"time"
)

func StartWatching(watchDir string, fileUpdates chan *Event) {
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
						filepath.Walk(ev.Name, watchDirFn)
					}
					continue
				}

				event := new(Event)
				if ev.IsCreate() || ev.IsModify() {
					event.Type = UPDATE
				} else if ev.IsDelete() || ev.IsRename() {
					event.Type = DELETE
				} else {
					panic("Unknown fsnotify event type")
				}

				event.Status = NOTICED
				event.Path = ev.Name
				event.Timestamp = time.Now()

				fileUpdates <- event

			case err := <-watcher.Error:
				panic(err)
			}
		}
	}()

	//start watching the directory passed in
	filepath.Walk(watchDir, watchDirFn)
}
