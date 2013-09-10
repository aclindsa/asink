/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"asink"
	"asink/util"
	"code.google.com/p/goconf/conf"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
)

type AsinkGlobals struct {
	configFileName string
	syncDir        string
	cacheDir       string
	tmpDir         string
	rpcSock        string
	db             *AsinkDB
	storage        Storage
	server         string
	port           int
	username       string
	password       string
}

var globals AsinkGlobals

var flags *flag.FlagSet

func init() {
	asink.SetupCleanExitOnSignals()
}

func StartClient(args []string) {
	const config_usage = "Config File to use"
	userHomeDir := "~"

	u, err := user.Current()
	if err == nil {
		userHomeDir = u.HomeDir
	}

	flags := flag.NewFlagSet("start", flag.ExitOnError)
	flags.StringVar(&globals.configFileName, "config", path.Join(userHomeDir, ".asink", "config"), config_usage)
	flags.StringVar(&globals.configFileName, "c", path.Join(userHomeDir, ".asink", "config"), config_usage+" (shorthand)")
	flags.Parse(args)

	//make sure config file's permissions are read-write only for the current user
	if !util.FileExistsAndHasPermissions(globals.configFileName, 384 /*0b110000000*/) {
		fmt.Println("Error: Either the file at " + globals.configFileName + " doesn't exist, or it doesn't have permissions such that the current user is the only one allowed to read and write.")
		return
	}

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
	globals.rpcSock, err = config.GetString("local", "socket") //TODO make sure this exists

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

	//TODO check errors on server settings
	globals.server, err = config.GetString("server", "host")
	globals.port, err = config.GetInt("server", "port")
	globals.username, err = config.GetString("server", "username")
	globals.password, err = config.GetString("server", "password")

	globals.db, err = GetAndInitDB(config)
	if err != nil {
		panic(err)
	}

	//spawn goroutine to handle locking file paths
	go PathLocker(globals.db)

	//spawn goroutines to handle local events
	go SendEvents(globals)
	localFileUpdates := make(chan *asink.Event)
	go StartWatching(globals.syncDir, localFileUpdates)

	//spawn goroutines to receive remote events
	remoteFileUpdates := make(chan *asink.Event)
	go GetEvents(globals, remoteFileUpdates)
	go ProcessLocalEvents(globals, localFileUpdates)
	//TODO ensure remote updates wait until all local changes are saved off?
	go ProcessRemoteEvents(globals, remoteFileUpdates)

	rpcTornDown := make(chan int)
	go StartRPC(globals.rpcSock, rpcTornDown)

	asink.WaitOnExit()
	<-rpcTornDown
}

func ProcessLocalEvent(globals AsinkGlobals, event *asink.Event) {
	StatStartLocalUpdate()
	defer StatStopLocalUpdate()

	//make the path relative before we save/send it anywhere
	var err error
	absolutePath := event.Path
	event.Path, err = filepath.Rel(globals.syncDir, event.Path)
	if err != nil {
		panic(err)
	}

	latestLocal := LockPath(event.Path, true)
	defer UnlockPath(event)
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
				return
			}
			panic(err)
		}

		//try to collect the file's permissions
		fileinfo, err := os.Stat(absolutePath)
		if err != nil {
			//bail out if the file we are trying to upload already got deleted
			if util.ErrorFileNotFound(err) {
				event.LocalStatus |= asink.DISCARDED
				return
			}
			panic(err)
		} else {
			event.Permissions = fileinfo.Mode()
		}

		//get the file's hash
		hash, err := HashFile(tmpfilename)
		if err != nil {
			panic(err)
		}
		event.Hash = hash

		//If the hash is the same, don't try to upload the event again
		if latestLocal != nil && event.Hash == latestLocal.Hash {
			os.Remove(tmpfilename)
			//If neither the file contents nor permissions changed, squash this event completely
			if event.Permissions == latestLocal.Permissions {
				event.LocalStatus |= asink.DISCARDED
				return
			}
		} else {
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
			StatStartUpload()
			uploadWriteCloser, err := globals.storage.Put(event.Hash)
			if err != nil {
				panic(err)
			}
			defer uploadWriteCloser.Close()

			uploadFile, err := os.Open(cachedFilename)
			if err != nil {
				panic(err)
			}
			defer uploadFile.Close()

			_, err = io.Copy(uploadWriteCloser, uploadFile)
			StatStopUpload()
			if err != nil {
				panic(err)
			}
		}
	} else {
		//if we're trying to delete a file that we thought was already deleted, there's no need to delete it again
		if latestLocal != nil && latestLocal.IsDelete() {
			event.LocalStatus |= asink.DISCARDED
			return
		}
	}

	//finally, send it off to the server
	StatStartSending()
	err = SendEvent(globals, event)
	StatStopSending()
	if err != nil {
		panic(err) //TODO handle sensibly
	}
}

func ProcessLocalEvents(globals AsinkGlobals, eventChan chan *asink.Event) {
	for {
		event := <-eventChan
		go ProcessLocalEvent(globals, event)
	}
}

func ProcessRemoteEvent(globals AsinkGlobals, event *asink.Event) {
	StatStartRemoteUpdate()
	defer StatStopRemoteUpdate()
	latestLocal := LockPath(event.Path, true)
	defer UnlockPath(event)

	//get the absolute path because we may need it later
	absolutePath := path.Join(globals.syncDir, event.Path)

	//if we already have this event, or if it is older than our most recent event, bail out
	if latestLocal != nil {
		if event.Timestamp < latestLocal.Timestamp {
			event.LocalStatus |= asink.DISCARDED
			return
		}
		if event.IsSameEvent(latestLocal) {
			return
		}

		if latestLocal.Hash != event.Predecessor && latestLocal.Hash != event.Hash {
			fmt.Printf("conflict:\n")
			fmt.Printf("OLD %+v\n", latestLocal)
			fmt.Printf("NEW %+v\n", event)
			//TODO handle conflict?
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
			StatStartDownload()
			downloadReadCloser, err := globals.storage.Get(event.Hash)
			if err != nil {
				panic(err)
			}
			defer downloadReadCloser.Close()
			_, err = io.Copy(outfile, downloadReadCloser)

			outfile.Close()
			StatStopDownload()
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

			//copy hashed file to another tmp, then rename it to the actual file.
			tmpfilename, err = util.CopyToTmp(hashedFilename, globals.tmpDir)
			if err != nil {
				panic(err)
			}

			//make sure containing directory exists
			err = util.EnsureDirExists(path.Dir(absolutePath))
			if err != nil {
				panic(err)
			}

			err = os.Rename(tmpfilename, absolutePath)
			if err != nil {
				err2 := os.Remove(tmpfilename)
				if err2 != nil {
					panic(err2)
				}
				panic(err)
			}
		}
		if latestLocal == nil || event.Hash != latestLocal.Hash || event.Permissions != latestLocal.Permissions {
			err := os.Chmod(absolutePath, event.Permissions)
			if err != nil && !util.ErrorFileNotFound(err) {
				panic(err)
			}
		}
	} else {
		//intentionally ignore errors in case this file has been deleted out from under us
		os.Remove(absolutePath)
		//delete the directory previously containing this file if its the last file
		util.RecursiveRemoveEmptyDirs(path.Dir(absolutePath))
	}

	//TODO make sure file being overwritten is either unchanged or already copied off and hashed
}

func ProcessRemoteEvents(globals AsinkGlobals, eventChan chan *asink.Event) {
	for event := range eventChan {
		go ProcessRemoteEvent(globals, event)
	}
}

func getSocketFromArgs(args []string) (string, error) {
	const config_usage = "Config File to use"
	userHomeDir := "~"

	u, err := user.Current()
	if err == nil {
		userHomeDir = u.HomeDir
	}

	flags := flag.NewFlagSet("stop", flag.ExitOnError)
	flags.StringVar(&globals.configFileName, "config", path.Join(userHomeDir, ".asink", "config"), config_usage)
	flags.StringVar(&globals.configFileName, "c", path.Join(userHomeDir, ".asink", "config"), config_usage+" (shorthand)")
	flags.Parse(args)

	config, err := conf.ReadConfigFile(globals.configFileName)
	if err != nil {
		return "", err
	}

	rpcSock, err := config.GetString("local", "socket")
	if err != nil {
		return "", errors.New("Error reading local.socket from config file at " + globals.configFileName)
	}

	return rpcSock, nil
}

func StopClient(args []string) {
	rpcSock, err := getSocketFromArgs(args)
	if err != nil {
		fmt.Println(err)
		return
	}

	i := 99
	returnCode := 0
	err = asink.RPCCall(rpcSock, "ClientAdmin.StopClient", &i, &returnCode)
	if err != nil {
		panic(err)
	}
}

func GetStatus(args []string) {
	var status string

	rpcSock, err := getSocketFromArgs(args)
	if err != nil {
		fmt.Println(err)
		return
	}

	i := 99
	err = asink.RPCCall(rpcSock, "ClientAdmin.GetClientStatus", &i, &status)
	if err != nil {
		panic(err)
	}

	fmt.Println(status)
}
