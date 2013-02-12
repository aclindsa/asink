package main

import (
	"fmt"
	"flag"
	"path"
	"os/user"
	"code.google.com/p/goconf/conf"
)

var configFileName string

func init() {
	const config_usage = "Config File to use"
	userHomeDir := "~"

	u, err := user.Current()
	if err == nil {
		userHomeDir = u.HomeDir
	} 

	flag.StringVar(&configFileName, "config", path.Join(userHomeDir, ".asink", "config"), config_usage)
	flag.StringVar(&configFileName, "c", path.Join(userHomeDir, ".asink", "config"), config_usage+" (shorthand)")
}

func main() {
	flag.Parse()
	fmt.Println("config file:", configFileName)

	config, err := conf.ReadConfigFile(configFileName)
	if err != nil {
		fmt.Println(err)
		fmt.Println("Error reading config file at ", configFileName, ". Does it exist?")
		return
	}

	storage, err := GetStorage(config)
	if err != nil {
		fmt.Println(err)
		return
	}

	syncdir, err := config.GetString("local", "syncdir")
	cachedir, err := config.GetString("local", "cachedir")

	fmt.Println(syncdir)
	fmt.Println(cachedir)
	fmt.Println(storage)

	fileUpdates := make(chan *Event)
	go StartWatching(syncdir, fileUpdates)

	for {
		event := <- fileUpdates
		ProcessEvent(storage, event)
	}
}

func ProcessEvent(storage Storage, event *Event) {
	fmt.Println(event)

	if event.IsUpdate() {
		err := storage.Put(event.Path, event.Hash)
		if err != nil { panic(err) }
	}
}
