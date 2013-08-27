package main

import (
	"asink/server"
	"code.google.com/p/gopass"
	"flag"
	"fmt"
	"os"
	"strconv"
	"net/rpc"
)

type boolIsSetFlag struct {
	Value bool
	IsSet bool //true if explicitly set from the command-line, false otherwise
}

func newBoolIsSetFlag(defaultValue bool) *boolIsSetFlag {
	b := new(boolIsSetFlag)
	b.Value = defaultValue
	return b
}

func (b *boolIsSetFlag) Set(value string) error {
	v, err := strconv.ParseBool(value)
	b.Value = v
	b.IsSet = true
	return err
}

func (b *boolIsSetFlag) String() string { return fmt.Sprintf("%v", *b) }

func (b *boolIsSetFlag) IsBoolFlag() bool { return true }

func UserAdd(args []string) {
	flags := flag.NewFlagSet("useradd", flag.ExitOnError)
	admin := flags.Bool("admin", false, "User should be an administrator")
	flags.Parse(args)

	if flags.NArg() != 1 {
		fmt.Println("Error: please supply a username (and only one)")
		os.Exit(1)
	}

	passwordOne, err := gopass.GetPass("Enter password for new user: ")
	if err != nil {
		panic(err)
	}
	passwordTwo, err := gopass.GetPass("Enter the same password again: ")
	if err != nil {
		panic(err)
	}

	if passwordOne != passwordTwo {
		fmt.Println("Error: Passwords do not match. Please try again.")
		os.Exit(1)
	}

	user := new(server.User)

	if *admin {
		user.Role = server.ADMIN
	} else {
		user.Role = server.NORMAL
	}
	user.Username = flags.Arg(0)
	user.PWHash = server.HashPassword(passwordOne)

	i := 99
	err = RPCCall("UserModifier.AddUser", user, &i)
	if err != nil {
		if _, ok := err.(rpc.ServerError); ok && err.Error() == server.DuplicateUsernameErr.Error() {
			fmt.Println("Error: "+err.Error())
			return
		}
		panic(err)
	}
}

func UserDel(args []string) {
	if len(args) != 1 {
		fmt.Println("Error: please supply a username (and only one)")
		os.Exit(1)
	}

	user := new(server.User)
	user.Username = args[0]

	i := 99
	err := RPCCall("UserModifier.RemoveUser", user, &i)
	if err != nil {
		if _, ok := err.(rpc.ServerError); ok && err.Error() == server.NoUserErr.Error() {
			fmt.Println("Error: "+err.Error())
			return
		}
		panic(err)
	}
}

func UserMod(args []string) {
	rpcargs := new(server.UserModifierArgs)
	rpcargs.Current = new(server.User)
	rpcargs.Updated = new(server.User)

	admin := newBoolIsSetFlag(false)

	flags := flag.NewFlagSet("usermod", flag.ExitOnError)
	flags.Var(admin, "admin", "User should be an administrator")
	flags.BoolVar(&rpcargs.UpdatePassword, "password", false, "Change the user's password")
	flags.BoolVar(&rpcargs.UpdatePassword, "p", false, "Change the user's password (short version)")
	flags.BoolVar(&rpcargs.UpdateLogin, "login", false, "Change the user's username")
	flags.BoolVar(&rpcargs.UpdateLogin, "l", false, "Change the user's username (short version)")
	flags.Parse(args)

	if flags.NArg() != 1 {
		fmt.Println("Error: please supply a username (and only one)")
		os.Exit(1)
	}
	rpcargs.Current.Username = flags.Arg(0)

	if rpcargs.UpdateLogin == true {
		fmt.Print("New login: ")
		fmt.Scanf("%s", &rpcargs.Updated.Username)
	}

	if rpcargs.UpdatePassword {
		passwordOne, err := gopass.GetPass("Enter new password for user: ")
		if err != nil {
			panic(err)
		}
		passwordTwo, err := gopass.GetPass("Enter the same password again: ")
		if err != nil {
			panic(err)
		}

		if passwordOne != passwordTwo {
			fmt.Println("Error: Passwords do not match. Please try again.")
			os.Exit(1)
		}
		rpcargs.Updated.PWHash = server.HashPassword(passwordOne)
	}

	//set the UpdateRole flag based on whether it was present on the command-line
	rpcargs.UpdateRole = admin.IsSet
	if admin.Value {
		rpcargs.Updated.Role = server.ADMIN
	} else {
		rpcargs.Updated.Role = server.NORMAL
	}

	if !rpcargs.UpdateRole && !rpcargs.UpdateLogin && !rpcargs.UpdatePassword {
		fmt.Println("What exactly are you modifying again?")
		return
	}

	i := 99
	err := RPCCall("UserModifier.ModifyUser", rpcargs, &i)
	if err != nil {
		if _, ok := err.(rpc.ServerError); ok && err.Error() == server.NoUserErr.Error() {
			fmt.Println("Error: "+err.Error())
			return
		}
		panic(err)
	}
}
