package main

import (
	"asink"
	"asink/util"
	"bytes"
	"code.google.com/p/goconf/conf"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path"
	"strconv"
)

type AsinkGlobals struct {
	configFileName string
	syncDir        string
	cacheDir       string
	tmpDir         string
	db             *sql.DB
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

	fileUpdates := make(chan *asink.Event)
	go StartWatching(globals.syncDir, fileUpdates)

	globals.db, err = GetAndInitDB(config)
	if err != nil {
		panic(err)
	}

	for {
		event := <-fileUpdates
		ProcessEvent(globals, event)
	}
}

func ProcessEvent(globals AsinkGlobals, event *asink.Event) {
	//add to database
	err := DatabaseAddEvent(globals.db, event)
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
		}
		event.Status |= asink.CACHED

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
		event.Status |= asink.UPLOADED

		//update database again
		err = DatabaseUpdateEvent(globals.db, event)
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
	err = DatabaseUpdateEvent(globals.db, event)
	if err != nil {
		panic(err) //TODO probably, definitely, none of these should panic
	}
}

func SendEvent(globals AsinkGlobals, event *asink.Event) error {
	url := "http://" + globals.server + ":" + strconv.Itoa(int(globals.port)) + "/events/"

	//construct json payload
	events := asink.EventList{
		Events: []*asink.Event{event},
	}
	b, err := json.Marshal(events)
	if err != nil {
		return err
	}
	fmt.Println(string(b))

	//actually make the request
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	//check to make sure request succeeded
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var apistatus asink.APIResponse
	err = json.Unmarshal(body, &apistatus)
	if err != nil {
		return err
	}
	if apistatus.Status != asink.SUCCESS {
		return errors.New("API response was not success")
	}

	return nil
}
