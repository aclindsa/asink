/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"fmt"
	"github.com/aclindsa/asink"
	"github.com/aclindsa/asink/util"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"
)

//Event-processing errors
const (
	STORAGE = iota
	TEMPORARY
	PERMANENT
	NETWORK
	CONFIG
	EXITED
)

type ProcessingError struct {
	ErrorType     int
	originalError error
}

func (pe ProcessingError) Error() string {
	typeString := ""
	origErrorString := ""

	switch pe.ErrorType {
	case STORAGE:
		typeString = "Storage"
	case TEMPORARY:
		typeString = "Temporary"
	case PERMANENT:
		typeString = "Permanent"
	case NETWORK:
		typeString = "Network"
	case CONFIG:
		typeString = "Configuration"
	case EXITED:
		typeString = "Exited"
	}
	if pe.originalError != nil {
		origErrorString = pe.originalError.Error()
	}
	return fmt.Sprintf("%s: %s", typeString, origErrorString)
}

func ErrorRequiresExit(err error) bool {
	if e, ok := err.(ProcessingError); ok {
		if e.ErrorType == TEMPORARY || e.ErrorType == NETWORK {
			return false
		}
		return true
	}
	return true //if the error wasn't even a processing error, something went wrong, so we should definitely exit
}

//handle a conflict by copying the loser event to another file
func handleConflict(globals *AsinkGlobals, loser *asink.Event, copyFrom string) error {
	if loser.IsUpdate() {
		//come up with new file name
		conflictedPath := path.Join(globals.syncDir, loser.Path) + "_conflicted_copy_" + time.Now().Format("2006-01-02_15:04:05.000000")

		//copy file to new filename
		src, err := os.Open(copyFrom)
		if err != nil {
			return err
		}
		defer src.Close()
		sink, err := os.Create(conflictedPath)
		if err != nil {
			return err
		}
		defer sink.Close()

		_, err = io.Copy(sink, src)
		return err
	}
	return nil
}
func ProcessLocalEvent(globals *AsinkGlobals, event *asink.Event) error {
	var err error

	StatStartLocalUpdate()
	defer StatStopLocalUpdate()

	//make the path relative before we save/send it anywhere
	absolutePath := event.Path
	event.Path, err = filepath.Rel(globals.syncDir, event.Path)
	if err != nil {
		return ProcessingError{TEMPORARY, err}
	}

	latestLocal := LockPath(event.Path, true)
	defer func() {
		if err != nil {
			event.LocalStatus |= asink.DISCARDED
		}
		UnlockPath(event)
	}()

	err = processLocalEvent_Upper(globals, event, latestLocal, absolutePath)
	if err != nil {
		return err
	}
	//don't process the second half if the first half discarded it
	if event.LocalStatus&asink.DISCARDED != 0 {
		return nil
	}
	err = processLocalEvent_Lower(globals, event, latestLocal)
	return err
}

func ProcessLocalEvent_Upper(globals *AsinkGlobals, event *asink.Event) error {
	var err error

	StatStartLocalUpdate()
	defer StatStopLocalUpdate()

	//make the path relative before we save/send it anywhere
	absolutePath := event.Path
	event.Path, err = filepath.Rel(globals.syncDir, event.Path)
	if err != nil {
		return ProcessingError{TEMPORARY, err}
	}

	latestLocal := LockPath(event.Path, true)

	defer func() {
		if err != nil {
			event.LocalStatus |= asink.DISCARDED
		}
		event.LocalStatus |= asink.NOSAVE //make sure event doesn't get saved back until lower half
		UnlockPath(event)
	}()

	err = processLocalEvent_Upper(globals, event, latestLocal, absolutePath)
	return err
}

func processLocalEvent_Upper(globals *AsinkGlobals, event *asink.Event, latestLocal *asink.Event, absolutePath string) error {
	if latestLocal != nil {
		event.Predecessor = latestLocal.Hash

		if event.Timestamp < latestLocal.Timestamp {
			fmt.Printf("trying to send event older than latestLocal:\n")
			fmt.Printf("OLD %+v\n", latestLocal)
			fmt.Printf("NEW %+v\n", event)
		}
	}

	if event.IsUpdate() {
		//copy to tmp
		//TODO upload in chunks and check modification times to make sure it hasn't been changed instead of copying the whole thing off
		tmpfilename, err := util.CopyToTmp(absolutePath, globals.tmpDir)
		if err != nil {
			//bail out if the file we are trying to upload already got deleted
			if util.ErrorFileNotFound(err) {
				event.LocalStatus |= asink.DISCARDED
				return nil
			}
			return err
		}

		//try to collect the file's permissions
		fileinfo, err := os.Stat(absolutePath)
		if err != nil {
			//bail out if the file we are trying to upload already got deleted
			if util.ErrorFileNotFound(err) {
				event.LocalStatus |= asink.DISCARDED
				return nil
			}
			return ProcessingError{PERMANENT, err}
		} else {
			event.Permissions = fileinfo.Mode()
		}

		//get the file's hash
		hash, err := HashFile(tmpfilename)
		if err != nil {
			return ProcessingError{TEMPORARY, err}
		}
		event.Hash = hash

		//If the hash is the same, don't try to upload the event again
		if latestLocal != nil && event.Hash == latestLocal.Hash {
			os.Remove(tmpfilename)
			//If neither the file contents nor permissions changed, squash this event completely
			if event.Permissions == latestLocal.Permissions {
				event.LocalStatus |= asink.DISCARDED
				return nil
			}
		} else {
			//rename to local cache w/ filename=hash
			cachedFilename := path.Join(globals.cacheDir, event.Hash)
			err = os.Rename(tmpfilename, cachedFilename)
			if err != nil {
				err = os.Remove(tmpfilename)
				if err != nil {
					return ProcessingError{PERMANENT, err}
				}
				return ProcessingError{PERMANENT, err}
			}
		}
	} else {
		//if we're trying to delete a file that we thought was already deleted, there's no need to delete it again
		if latestLocal != nil && latestLocal.IsDelete() {
			event.LocalStatus |= asink.DISCARDED
			return nil
		}
	}
	return nil
}

func ProcessLocalEvent_Lower(globals *AsinkGlobals, event *asink.Event) error {
	var err error

	StatStartLocalUpdate()
	defer StatStopLocalUpdate()

	latestLocal := LockPath(event.Path, true)
	defer func() {
		if err != nil {
			event.LocalStatus |= asink.DISCARDED
		}
		event.LocalStatus &= ^asink.NOSAVE //clear NOSAVE set in upper half
		UnlockPath(event)
	}()

	err = processLocalEvent_Lower(globals, event, latestLocal)
	return err
}

func processLocalEvent_Lower(globals *AsinkGlobals, event *asink.Event, latestLocal *asink.Event) error {
	var err error

	//if we already have this event, or if it is older than our most recent event, bail out
	if latestLocal != nil {
		if event.Timestamp < latestLocal.Timestamp {
			event.LocalStatus |= asink.DISCARDED
			return nil
		}

		//if the remote side snuck in an event that has the same hash
		//as we do, disregard our event
		if event.Hash == latestLocal.Hash {
			event.LocalStatus |= asink.DISCARDED
			return nil
		}

		//if our predecessor has changed, it means we have received a
		//remote event for this file since the top half of processing
		//this local event. If this is true, we have a conflict we
		//can't resolve without user intervention.
		if latestLocal.Hash != event.Predecessor {
			err = handleConflict(globals, event, path.Join(globals.cacheDir, event.Hash))
			event.LocalStatus |= asink.DISCARDED
			if err != nil {
				return ProcessingError{PERMANENT, err}
			}
			return nil
		}
	}

	if event.IsUpdate() {
		//upload file to remote storage
		StatStartUpload()
		uploadWriteCloser, err := globals.storage.Put(event.Hash)
		if err != nil {
			return ProcessingError{STORAGE, err}
		}

		cachedFilename := path.Join(globals.cacheDir, event.Hash)
		uploadFile, err := os.Open(cachedFilename)
		if err != nil {
			uploadWriteCloser.Close()
			return ProcessingError{STORAGE, err}
		}

		if globals.encrypted {
			encrypter, err := NewEncrypter(uploadWriteCloser, globals.key)
			if err != nil {
				uploadWriteCloser.Close()
				uploadFile.Close()
				return ProcessingError{STORAGE, err}
			}
			_, err = io.Copy(encrypter, uploadFile)
			encrypter.Close()
		} else {
			_, err = io.Copy(uploadWriteCloser, uploadFile)
		}
		uploadFile.Close()
		uploadWriteCloser.Close()

		StatStopUpload()
		if err != nil {
			return ProcessingError{STORAGE, err}
		}
	}

	//finally, send it off to the server
	StatStartSending()
	err = SendEvent(globals, event)
	StatStopSending()
	if err != nil {
		return ProcessingError{NETWORK, err}
	}
	return nil
}

func ProcessRemoteEvent(globals *AsinkGlobals, event *asink.Event) error {
	var err error

	StatStartRemoteUpdate()
	defer StatStopRemoteUpdate()
	latestLocal := LockPath(event.Path, false)
	defer func() {
		if err != nil {
			event.LocalStatus |= asink.DISCARDED
		}
		UnlockPath(event)
	}()

	//get the absolute path because we may need it later
	absolutePath := path.Join(globals.syncDir, event.Path)

	//if we already have this event, or if it is older than our most recent event, bail out
	if latestLocal != nil {
		if event.Timestamp < latestLocal.Timestamp {
			event.LocalStatus |= asink.DISCARDED
			return nil
		}
		if event.IsSameEvent(latestLocal) {
			return nil
		}

		if latestLocal.Hash != event.Predecessor && latestLocal.Hash != event.Hash {
			err = handleConflict(globals, latestLocal, path.Join(globals.cacheDir, latestLocal.Hash))
			if err != nil {
				return ProcessingError{PERMANENT, err}
			}
		}
	}

	//Download event
	if event.IsUpdate() {
		if latestLocal == nil || event.Hash != latestLocal.Hash {

			outfile, err := ioutil.TempFile(globals.tmpDir, "asink")
			if err != nil {
				return ProcessingError{CONFIG, err}
			}
			tmpfilename := outfile.Name()
			StatStartDownload()
			downloadReadCloser, err := globals.storage.Get(event.Hash)
			if err != nil {
				StatStopDownload()
				return ProcessingError{STORAGE, err}
			}
			defer downloadReadCloser.Close()
			if globals.encrypted {
				decrypter, err := NewDecrypter(downloadReadCloser, globals.key)
				if err != nil {
					StatStopDownload()
					return ProcessingError{STORAGE, err}
				}
				_, err = io.Copy(outfile, decrypter)
			} else {
				_, err = io.Copy(outfile, downloadReadCloser)
			}

			outfile.Close()
			StatStopDownload()
			if err != nil {
				return ProcessingError{STORAGE, err}
			}

			//rename to local hashed filename
			hashedFilename := path.Join(globals.cacheDir, event.Hash)
			err = os.Rename(tmpfilename, hashedFilename)
			if err != nil {
				err = os.Remove(tmpfilename)
				if err != nil {
					return ProcessingError{PERMANENT, err}
				}
				return ProcessingError{PERMANENT, err}
			}

			//copy hashed file to another tmp, then rename it to the actual file.
			tmpfilename, err = util.CopyToTmp(hashedFilename, globals.tmpDir)
			if err != nil {
				return ProcessingError{PERMANENT, err}
			}

			//make sure containing directory exists
			err = util.EnsureDirExists(path.Dir(absolutePath))
			if err != nil {
				return ProcessingError{PERMANENT, err}
			}

			err = os.Rename(tmpfilename, absolutePath)
			if err != nil {
				err2 := os.Remove(tmpfilename)
				if err2 != nil {
					return ProcessingError{PERMANENT, err2}
				}
				return ProcessingError{PERMANENT, err}
			}
		}
		if latestLocal == nil || event.Hash != latestLocal.Hash || event.Permissions != latestLocal.Permissions {
			err = os.Chmod(absolutePath, event.Permissions)
			if err != nil && !util.ErrorFileNotFound(err) {
				return ProcessingError{PERMANENT, err}
			}
		}
	} else {
		//intentionally ignore errors in case this file has been deleted out from under us
		os.Remove(absolutePath)
		//delete the directory previously containing this file if its the last file
		util.RecursiveRemoveEmptyDirs(path.Dir(absolutePath))
	}

	return nil
}
