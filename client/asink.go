package main

import (
	"asink"
	"asink/util"
	"code.google.com/p/goconf/conf"
	"flag"
	"fmt"
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
	//add to database
	err := globals.db.DatabaseAddEvent(event)
	if err != nil {
		panic(err)
	}

	if event.IsUpdate() {
		//copy to tmp
		tmpfilename, err := util.CopyToTmp(event.Path, globals.tmpDir)
		if err != nil {
			panic(err)
		}
		event.Status |= asink.COPIED_TO_TMP

		//get the file's hash
		hash, err := HashFile(tmpfilename)
		event.Hash = hash
		if err != nil {
			panic(err)
		}
		event.Status |= asink.HASHED

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

		//update database
		err = globals.db.DatabaseUpdateEvent(event)
		if err != nil {
			panic(err)
		}

		//upload file to remote storage
		err = globals.storage.Put(event.Path, event.Hash)
		if err != nil {
			panic(err)
		}
		event.Status |= asink.UPLOADED

		//update database again
		err = globals.db.DatabaseUpdateEvent(event)
		if err != nil {
			panic(err)
		}

	}

	//finally, send it off to the server
	err = SendEvent(globals, event)
	if err != nil {
		panic(err) //TODO handle sensibly
	}

	event.Status |= asink.ON_SERVER
	err = globals.db.DatabaseUpdateEvent(event)
	if err != nil {
		panic(err) //TODO probably, definitely, none of these should panic
	}
}

func ProcessRemoteEvents(globals AsinkGlobals, eventChan chan *asink.Event) {
	for event := range eventChan {
		fmt.Println(event)
		//TODO actually download event, add it to the local database, and populate the local directory
	}
}
