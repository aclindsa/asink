package server

import (
	"net"
	"net/http"
	"net/rpc"
)

type UserModifier struct {
	adb *AsinkDB
}

type UserModifierArgs struct {
	Current        *User
	Updated        *User
	UpdateLogin    bool
	UpdateRole     bool
	UpdatePassword bool
}

func (u *UserModifier) AddUser(user *User, result *int) error {
	err := u.adb.DatabaseAddUser(user)
	if err != nil {
		*result = 1
	} else {
		*result = 0
	}
	return err
}

func (u *UserModifier) ModifyUser(args *UserModifierArgs, result *int) error {
	currentUser, err := u.adb.DatabaseGetUser(args.Current.Username)
	if err != nil {
		*result = 1
		return err
	}

	if args.UpdateLogin {
		currentUser.Username = args.Updated.Username
	}
	if args.UpdateRole {
		currentUser.Role = args.Updated.Role
	}
	if args.UpdatePassword {
		currentUser.PWHash = args.Updated.PWHash
	}

	err = u.adb.DatabaseUpdateUser(currentUser)
	if err != nil {
		*result = 1
		return err
	}

	*result = 0
	return nil
}

func (u *UserModifier) RemoveUser(user *User, result *int) error {
	err := u.adb.DatabaseDeleteUser(user)
	if err != nil {
		*result = 1
	} else {
		*result = 0
	}
	return err
}

func StartRPC(tornDown chan int, adb *AsinkDB) {
	defer func() { tornDown <- 0 }() //the main thread waits for this to ensure the socket is closed

	usermod := new(UserModifier)
	usermod.adb = adb

	rpc.Register(usermod)
	rpc.HandleHTTP()
	l, err := net.Listen("unix", "/tmp/asink.sock")
	if err != nil {
		panic(err)
	}
	defer l.Close()

	go http.Serve(l, nil)

	WaitOnExit()
}
