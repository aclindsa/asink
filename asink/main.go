package main

import (
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
		fn:          StartClient,
		explanation: "Start the client daemon",
	},
	Command{
		cmd:         "stop",
		fn:          StopClient,
		explanation: "Stop the client daemon",
	},
	/*	Command{
			cmd:         "status",
			fn:          GetStatus,
			explanation: "Get a summary of the client's status",
		},
	*/
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
