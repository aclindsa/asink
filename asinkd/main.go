/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"asink"
	"fmt"
	"os"
)

type Command struct {
	cmd         string
	fn          func(args []string)
	explanation string
}

var commands []Command = []Command{
	Command{
		cmd:         "start",
		fn:          StartServer,
		explanation: "Start the server daemon",
	},
	Command{
		cmd:         "stop",
		fn:          StopServer,
		explanation: "Stop the server daemon",
	},
	Command{
		cmd:         "useradd",
		fn:          UserAdd,
		explanation: "Add a user",
	},
	Command{
		cmd:         "userdel",
		fn:          UserDel,
		explanation: "Remove a user",
	},
	Command{
		cmd:         "usermod",
		fn:          UserMod,
		explanation: "Modify a user",
	},
	Command{
		cmd:         "version",
		fn:          PrintVersion,
		explanation: "Display the current version",
	},
}

func main() {
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		for _, c := range commands {
			if c.cmd == cmd {
				c.fn(os.Args[2:])
				return
			}
		}
		fmt.Println("Invalid subcommand specified, please pick from the following:")
	} else {
		fmt.Println("No subcommand specified, please pick one from the following:")
	}
	for _, c := range commands {
		fmt.Printf("\t%s\t\t%s\n", c.cmd, c.explanation)
	}
}

func PrintVersion(args []string) {
	fmt.Println("Asink server version " + asink.VERSION_STRING + ", using version " + asink.API_VERSION_STRING + " of the Asink API.")
}
