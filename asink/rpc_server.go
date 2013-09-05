/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"asink"
	"net"
	"net/http"
	"net/rpc"
)

type ClientAdmin int

func (c *ClientAdmin) StopClient(code *int, result *int) error {
	asink.Exit(*code)
	*result = 0
	return nil
}

func (c *ClientAdmin) GetClientStatus(code *int, result *string) error {
	*result = GetStats()
	return nil
}

func StartRPC(sock string, tornDown chan int) {
	defer func() { tornDown <- 0 }() //the main thread waits for this to ensure the socket is closed

	clientadmin := new(ClientAdmin)
	rpc.Register(clientadmin)

	rpc.HandleHTTP()
	l, err := net.Listen("unix", sock)
	if err != nil {
		panic(err)
	}
	defer l.Close()

	go http.Serve(l, nil)

	asink.WaitOnExit()
}
