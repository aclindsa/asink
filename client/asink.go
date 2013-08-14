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
	"syscall"
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
			if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOENT {
				//if the file doesn't exist, it must've been deleted out from under us, disregard this event
				return
			} else {
				panic(err)
			}
		}
		event.Status |= asink.COPIED_TO_TMP

		//get the file's hash
		hash, err := HashFile(tmpfilename)
		event.Hash = hash
		if err != nil {
			panic(err)
		}
		event.Status |= asink.HASHED

		//If the file didn't actually change, squash this event
		if latestLocal != nil && event.Hash == latestLocal.Hash {
			os.Remove(tmpfilename)
			return
		}

		//rename to local cache w/ filename=hash
		err = os.Rename(tmpfilename, path.Join(globals.cacheDir, event.Hash))
		if err != nil {
			err := os.Remove(tmpfilename)
			if err != nil {
				panic(err)
			}
			panic(err)
		}
		event.Status |= asink.CACHED

		//upload file to remote storage
		err = globals.storage.Put(event.Path, event.Hash)
		if err != nil {
			panic(err)
		}
		event.Status |= asink.UPLOADED
	}

	//finally, send it off to the server
	err := SendEvent(globals, event)
	if err != nil {
		panic(err) //TODO handle sensibly
	}

	event.Status |= asink.ON_SERVER
}

func ProcessRemoteEvent(globals AsinkGlobals, event *asink.Event) {
	latestLocal := LockPath(event.Path, true)
	defer UnlockPath(event)
	//if we already have this event, or if it is older than our most recent event, bail out
	if latestLocal != nil {
		if event.Timestamp < latestLocal.Timestamp || event.IsSameEvent(latestLocal) {
			UnlockPath(event)
			return
		}

		if latestLocal.Hash != event.Predecessor && latestLocal.Hash != event.Hash {
			panic("conflict")
			//TODO handle conflict
		}
	}

	//Download event
	if event.IsUpdate() && (latestLocal == nil || event.Hash != latestLocal.Hash) {
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
		err = os.Rename(tmpfilename, path.Join(globals.cacheDir, event.Hash))
		if err != nil {
			err := os.Remove(tmpfilename)
			if err != nil {
				panic(err)
			}
			panic(err)
		}
	} else {
		//intentionally ignore errors in case this file has been deleted out from under us
		os.Remove(event.Path)
		//TODO delete file hierarchy beneath this file if its the last one in its directory?
	}

	fmt.Println(event)
	//TODO make sure file being overwritten is either unchanged or already copied off and hashed
}

func ProcessRemoteEvents(globals AsinkGlobals, eventChan chan *asink.Event) {
	for event := range eventChan {
		ProcessRemoteEvent(globals, event)
	}
}
