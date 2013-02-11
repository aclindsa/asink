package main

import (
	"fmt"
	"flag"
	"path"
	"os/user"
	"github.com/howeyc/fsnotify"
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

	setup_watchers()
}

func setup_watchers() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Failed to create fsnotify watcher")
		return
	}
	fmt.Println("Created new fsnotify watcher!")

	err = watcher.Watch("/home/aclindsa/.asink")
	if err != nil {
		fmt.Println("Failed to watch /home/aclindsa/.asink")
		return
	}

	for {
		select {
		case ev := <-watcher.Event:
			fmt.Println("event:", ev)
			hash, err := HashFile(ev.Name)
			//TODO if creating a directory, start watching it (and then initiate a full scan of it so we're sure nothing slipped through the cracks)
			if err != nil {
				fmt.Println(err)
			} else  {
				fmt.Println(hash)
			}
		case err := <-watcher.Error:
			fmt.Println("error:", err)
		}
	}
}
