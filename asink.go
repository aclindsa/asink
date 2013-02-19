package main

import (
	"code.google.com/p/goconf/conf"
	"database/sql"
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
	db             *sql.DB
	storage        Storage
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
	fmt.Println("config file:", globals.configFileName)

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
	err = ensureDirExists(globals.syncDir)
	if err != nil {
		panic(err)
	}
	err = ensureDirExists(globals.cacheDir)
	if err != nil {
		panic(err)
	}
	err = ensureDirExists(globals.tmpDir)
	if err != nil {
		panic(err)
	}

	//TODO FIXME REMOVEME
	fmt.Println(globals.syncDir)
	fmt.Println(globals.cacheDir)
	fmt.Println(globals.tmpDir)
	fmt.Println(globals.storage)
	//TODO FIXME REMOVEME

	fileUpdates := make(chan *Event)
	go StartWatching(globals.syncDir, fileUpdates)

	globals.db, err = GetAndInitDB(config)
	if err != nil {
		panic(err)
		return
	}

	for {
		event := <-fileUpdates
		ProcessEvent(globals, event)
	}
}

func ProcessEvent(globals AsinkGlobals, event *Event) {
	//add to database
	err := DatabaseAddEvent(globals.db, event)
	if err != nil {
		panic(err)
	}

	if event.IsUpdate() {
		//copy to tmp
		tmpfilename, err := copyToTmp(event.Path, globals.tmpDir)
		if err != nil {
			panic(err)
		}
		event.Status |= COPIED_TO_TMP

		//get the file's hash
		hash, err := HashFile(tmpfilename)
		event.Hash = hash
		if err != nil {
			panic(err)
		}
		event.Status |= HASHED

		//rename to local cache w/ filename=hash
		err = os.Rename(tmpfilename, path.Join(globals.cacheDir, event.Hash))
		if err != nil {
			err := os.Remove(tmpfilename)
			if err != nil {
				panic(err)
			}
		}
		event.Status |= CACHED

		//update database
		err = DatabaseUpdateEvent(globals.db, event)
		if err != nil {
			panic(err)
		}

		//upload file to remote storage
		err = globals.storage.Put(event.Path, event.Hash)
		if err != nil {
			panic(err)
		}
		event.Status |= UPLOADED

		//update database again
		err = DatabaseUpdateEvent(globals.db, event)
		if err != nil {
			panic(err)
		}

	}
	fmt.Println(event)

	//TODO notify server of new file
}
