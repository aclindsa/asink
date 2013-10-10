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
	"os/user"
	"path"
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
	encrypted      bool
	key            string
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

	//TODO check errors on encryption settings
	globals.encrypted, err = config.GetBool("encryption", "enabled")
	if globals.encrypted {
		globals.key, err = config.GetString("encryption", "key")
	}

	globals.db, err = GetAndInitDB(config)
	if err != nil {
		panic(err)
	}

	//spawn goroutine to handle locking file paths
	go PathLocker(globals.db)

	//spawn goroutines to handle local events
	go SendEvents(&globals)
	localFileUpdates := make(chan *asink.Event)
	initialWalkComplete := make(chan int)
	go StartWatching(globals.syncDir, localFileUpdates, initialWalkComplete)

	//spawn goroutines to receive remote events
	remoteFileUpdates := make(chan *asink.Event)
	go GetEvents(&globals, remoteFileUpdates)

	rpcTornDown := make(chan int)
	go StartRPC(globals.rpcSock, rpcTornDown)
	defer func() { <-rpcTornDown }()

	//make chan with which to wait for exit
	exitChan := make(chan int)
	asink.WaitOnExitChan(exitChan)

	//create all the contexts
	startupContext := NewStartupContext(&globals, localFileUpdates, remoteFileUpdates, initialWalkComplete, exitChan)
	normalContext := NewNormalContext(&globals, localFileUpdates, remoteFileUpdates, exitChan)

	//begin running contexts
	err = startupContext.Run()
	if err != nil && ErrorRequiresExit(err) {
		return
	}

	err = normalContext.Run()
	if err != nil {
		fmt.Println(err)
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
