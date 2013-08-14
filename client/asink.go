package main

import (
	"asink"
	"asink/util"
	"code.google.com/p/goconf/conf"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
)

type AsinkGlobals struct {
	configFileName string
	syncDir        string
	cacheDir       string
	tmpDir         string
	db             *AsinkDB
	storage        Storage
	server         string
	port           int
}

var globals AsinkGlobals

func init() {
	const config_usage = "Config File to use"
	userHomeDir := "~"

	u, err := user.Current()
	if err == nil {
		userHomeDir = u.HomeDir
	}

	flag.StringVar(&globals.configFileName, "config", path.Join(userHomeDir, ".asink", "config"), config_usage)
	flag.StringVar(&globals.configFileName, "c", path.Join(userHomeDir, ".asink", "config"), config_usage+" (shorthand)")
}

func main() {
	flag.Parse()

	config, err := conf.ReadConfigFile(globals.configFileName)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Error reading config file at ", globals.configFileName, ". Does it exist?")
		return
	}

	globals.storage, err = GetStorage(config)
	if err != nil {
		fmt.Println(err)
		return
	}

	globals.syncDir, err = config.GetString("local", "syncdir")
	globals.cacheDir, err = config.GetString("local", "cachedir")
	globals.tmpDir, err = config.GetString("local", "tmpdir")

	//make sure all the necessary directories exist
	err = util.EnsureDirExists(globals.syncDir)
	if err != nil {
		panic(err)
	}
	err = util.EnsureDirExists(globals.cacheDir)
	if err != nil {
		panic(err)
	}
	err = util.EnsureDirExists(globals.tmpDir)
	if err != nil {
		panic(err)
	}

	globals.server, err = config.GetString("server", "host")
	globals.port, err = config.GetInt("server", "port")

	globals.db, err = GetAndInitDB(config)
	if err != nil {
		panic(err)
	}

	//spawn goroutine to handle locking file paths
	go PathLocker(globals.db)

	//spawn goroutines to handle local events
	localFileUpdates := make(chan *asink.Event)
	go StartWatching(globals.syncDir, localFileUpdates)

	//spawn goroutines to receive remote events
	remoteFileUpdates := make(chan *asink.Event)
	go GetEvents(globals, remoteFileUpdates)
	go ProcessRemoteEvents(globals, remoteFileUpdates)

	for {
		event := <-localFileUpdates
		go ProcessLocalEvent(globals, event)
	}
}

func ProcessLocalEvent(globals AsinkGlobals, event *asink.Event) {
	latestLocal := LockPath(event.Path, true)
	defer UnlockPath(event)
	if latestLocal != nil {
		event.Predecessor = latestLocal.Hash
	}

	if event.IsUpdate() {
		//copy to tmp
		//TODO upload in chunks and check modification times to make sure it hasn't been changed instead of copying the whole thing off
		tmpfilename, err := util.CopyToTmp(event.Path, globals.tmpDir)
		if err != nil {
			//bail out if the file we are trying to upload already got deleted
			if util.ErrorFileNotFound(err) {
				event.Status |= asink.DISCARDED
				return
			}
			panic(err)
		}

		//try to collect the file's permissions
		fileinfo, err := os.Stat(event.Path)
		if err != nil {
			//bail out if the file we are trying to upload already got deleted
			if util.ErrorFileNotFound(err) {
				event.Status |= asink.DISCARDED
				return
			}
			panic(err)
		} else {
			event.Permissions = fileinfo.Mode()
		}

		//get the file's hash
		hash, err := HashFile(tmpfilename)
		event.Hash = hash
		if err != nil {
			panic(err)
		}

		//If the file didn't actually change, squash this event
		if latestLocal != nil && event.Hash == latestLocal.Hash {
			os.Remove(tmpfilename)
			event.Status |= asink.DISCARDED
			return
		}

		//rename to local cache w/ filename=hash
		cachedFilename := path.Join(globals.cacheDir, event.Hash)
		err = os.Rename(tmpfilename, cachedFilename)
		if err != nil {
			err := os.Remove(tmpfilename)
			if err != nil {
				panic(err)
			}
			panic(err)
		}

		//upload file to remote storage
		err = globals.storage.Put(cachedFilename, event.Hash)
		if err != nil {
			panic(err)
		}
	} else {
		//if we're trying to delete a file that we thought was already deleted, there's no need to delete it again
		if latestLocal != nil && latestLocal.IsDelete() {
			event.Status |= asink.DISCARDED
			return
		}
	}

	//finally, send it off to the server
	err := SendEvent(globals, event)
	if err != nil {
		panic(err) //TODO handle sensibly
	}
}

func ProcessRemoteEvent(globals AsinkGlobals, event *asink.Event) {
	latestLocal := LockPath(event.Path, true)
	defer UnlockPath(event)

	//if we already have this event, or if it is older than our most recent event, bail out
	if latestLocal != nil {
		if event.Timestamp < latestLocal.Timestamp || event.IsSameEvent(latestLocal) {
			event.Status |= asink.DISCARDED
			return
		}

		if latestLocal.Hash != event.Predecessor && latestLocal.Hash != event.Hash {
			panic("conflict")
			//TODO handle conflict
		}
	}

	//Download event
	if event.IsUpdate() {
		if latestLocal == nil || event.Hash != latestLocal.Hash {

			outfile, err := ioutil.TempFile(globals.tmpDir, "asink")
			if err != nil {
				panic(err) //TODO handle sensibly
			}
			tmpfilename := outfile.Name()
			outfile.Close()
			err = globals.storage.Get(tmpfilename, event.Hash)
			if err != nil {
				panic(err) //TODO handle sensibly
			}

			//rename to local hashed filename
			hashedFilename := path.Join(globals.cacheDir, event.Hash)
			err = os.Rename(tmpfilename, hashedFilename)
			if err != nil {
				err := os.Remove(tmpfilename)
				if err != nil {
					panic(err)
				}
				panic(err)
			}

			//TODO copy hashed file to another tmp, then rename it to the actual file.
			tmpfilename, err = util.CopyToTmp(hashedFilename, globals.tmpDir)
			if err != nil {
				panic(err)
			}
			err = os.Rename(tmpfilename, event.Path)
			if err != nil {
				err := os.Remove(tmpfilename)
				if err != nil {
					panic(err)
				}
				panic(err)
			}
		}
		if latestLocal == nil || event.Permissions != latestLocal.Permissions {
			err := os.Chmod(event.Path, event.Permissions)
			if err != nil && !util.ErrorFileNotFound(err) {
				panic(err)
			}
		}
	} else {
		//intentionally ignore errors in case this file has been deleted out from under us
		os.Remove(event.Path)
		//TODO delete file hierarchy beneath this file if its the last one in its directory?
	}

	//TODO make sure file being overwritten is either unchanged or already copied off and hashed
}

func ProcessRemoteEvents(globals AsinkGlobals, eventChan chan *asink.Event) {
	for event := range eventChan {
		ProcessRemoteEvent(globals, event)
	}
}
