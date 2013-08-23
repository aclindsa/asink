package main

import (
	"fmt"
	"os"
)

type AdminCommand struct {
	cmd         string
	fn          func(args []string)
	explanation string
}

var commands []AdminCommand = []AdminCommand{
	AdminCommand{
		cmd:         "useradd",
		fn:          UserAdd,
		explanation: "Add a user",
	},
	AdminCommand{
		cmd:         "userdel",
		fn:          UserDel,
		explanation: "Remove a user",
	},
	AdminCommand{
		cmd:         "usermod",
		fn:          UserMod,
		explanation: "Modify a user",
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
