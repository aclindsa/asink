package main

import (
	"asink"
	"net"
	"net/http"
	"net/rpc"
)

type ClientStopper int

func (c *ClientStopper) StopClient(code *int, result *int) error {
	asink.Exit(*code)
	*result = 0
	return nil
}

func StartRPC(sock string, tornDown chan int) {
	defer func() { tornDown <- 0 }() //the main thread waits for this to ensure the socket is closed

	clientstop := new(ClientStopper)
	rpc.Register(clientstop)

	rpc.HandleHTTP()
	l, err := net.Listen("unix", sock)
	if err != nil {
		panic(err)
	}
	defer l.Close()

	go http.Serve(l, nil)

	asink.WaitOnExit()
}
