package server

import (
	"fmt"
	"net"
	"net/http"
	"net/rpc"
)

type UserModifier int

type UserModifierArgs struct {
	Current        *User
	Updated        *User
	UpdateLogin    bool
	UpdateRole     bool
	UpdatePassword bool
}

func (u *UserModifier) AddUser(user *User, result *int) error {
	fmt.Println("adding user: ", user)
	ret := 0
	result = &ret
	return nil
}

func (u *UserModifier) ModifyUser(args *UserModifierArgs, result *int) error {
	fmt.Println("modifying user: ", args)
	fmt.Println("from: ", args.Current)
	fmt.Println("to: ", args.Updated)
	ret := 0
	result = &ret
	return nil
}

func (u *UserModifier) RemoveUser(user *User, result *int) error {
	fmt.Println("removing user: ", user)
	ret := 0
	result = &ret
	return nil
}

func StartRPC(townDown chan int) {
	defer func() { townDown <- 0 }() //the main thread waits for this to ensure the socket is closed

	usermod := new(UserModifier)
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
