package main

import (
	"github.com/howeyc/fsnotify"
)

func StartWatching(watchDir string, fileUpdates chan *Event) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic("Failed to create fsnotify watcher")
	}

	err = watcher.Watch(watchDir)
	if err != nil {
		panic("Failed to watch " + watchDir)
	}

	for {
		select {
		case ev := <-watcher.Event:
			event := new(Event)
			if ev.IsCreate() || ev.IsModify() {
				event.Type = UPDATE
			} else if ev.IsDelete() || ev.IsRename() {
				event.Type = DELETE
			} else {
				panic("Unknown fsnotify event type")
			}

			event.Path = ev.Name
			if event.IsUpdate() {
				event.Hash, err = HashFile(ev.Name)
				if err != nil { continue }
			} else {
				event.Hash = ""
			}

			fileUpdates <- event

			//TODO if creating a directory, start watching it (and then initiate a full scan of it so we're sure nothing slipped through the cracks)

		case err := <-watcher.Error:
			panic(err)
		}
	}
}
