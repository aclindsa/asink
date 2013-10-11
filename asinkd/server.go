/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/aclindsa/asink"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

//global variables
var eventsRegexp *regexp.Regexp
var port int = 8080
var rpcSock string
var adb *AsinkDB

func init() {
	var err error

	eventsRegexp = regexp.MustCompile("^/events/([0-9]+)$")

	adb, err = GetAndInitDB()
	if err != nil {
		panic(err)
	}

	asink.SetupCleanExitOnSignals()
}

const sock_usage = "Socket to use to connect to the Asink server."
const sock_default = "/var/run/asink/asinkd.sock"

func StartServer(args []string) {
	const port_usage = "Port on which to serve HTTP API"

	flags := flag.NewFlagSet("start", flag.ExitOnError)
	flags.IntVar(&port, "port", 8080, port_usage)
	flags.IntVar(&port, "p", 8080, port_usage+" (shorthand)")
	flags.StringVar(&rpcSock, "sock", sock_default, sock_usage)
	flags.StringVar(&rpcSock, "s", sock_default, sock_usage+" (shorthand)")
	flags.Parse(args)

	rpcTornDown := make(chan int)
	go StartRPC(rpcSock, rpcTornDown, adb)

	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/events", eventHandler)
	http.HandleFunc("/events/", eventHandler)

	//TODO add HTTPS, something like http://golang.org/pkg/net/http/#ListenAndServeTLS
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}
	defer l.Close()
	go http.Serve(l, nil)
	//TODO handle errors from http.Serve?

	asink.WaitOnExit()
	<-rpcTornDown
}

func StopServer(args []string) {
	flags := flag.NewFlagSet("stop", flag.ExitOnError)
	flags.StringVar(&rpcSock, "sock", sock_default, sock_usage)
	flags.StringVar(&rpcSock, "s", sock_default, sock_usage+" (shorthand)")
	flags.Parse(args)

	i := 99
	returnCode := 0
	err := asink.RPCCall(rpcSock, "ServerStopper.StopServer", &returnCode, &i)
	if err != nil {
		panic(err)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "You're probably looking for /events/")
}

func getEvents(w http.ResponseWriter, r *http.Request, user *User, nextEvent uint64) {
	var events []*asink.Event
	var error_message string = ""
	defer func() {
		var apiresponse asink.APIResponse
		if error_message != "" {
			apiresponse = asink.APIResponse{
				Status:      asink.ERROR,
				Explanation: error_message,
			}
		} else {
			apiresponse = asink.APIResponse{
				Status: asink.SUCCESS,
				Events: events,
			}
		}
		b, err := json.Marshal(apiresponse)
		if err != nil {
			error_message = err.Error()
			return
		}
		w.Write(b)
	}()

	events, err := adb.DatabaseRetrieveEvents(nextEvent, 50, user)
	if err != nil {
		error_message = err.Error()
		return
	}

	//long-poll if events is empty
	if len(events) == 0 {
		c := make(chan *asink.Event)
		addPoller(user.Id, &c) //TODO support more than one share per user
		e, ok := <-c
		if ok {
			events = append(events, e)
		}
	}
}

func putEvents(w http.ResponseWriter, r *http.Request, user *User) {
	var events asink.EventList
	var error_message string = ""
	defer func() {
		var apiresponse asink.APIResponse
		if error_message != "" {
			apiresponse = asink.APIResponse{
				Status:      asink.ERROR,
				Explanation: error_message,
			}
		} else {
			apiresponse = asink.APIResponse{
				Status: asink.SUCCESS,
			}
		}
		b, err := json.Marshal(apiresponse)
		if err != nil {
			error_message = err.Error()
			return
		}
		w.Write(b)
	}()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		error_message = err.Error()
		return
	}
	err = json.Unmarshal(body, &events)
	if err != nil {
		error_message = err.Error()
		return
	}
	err = adb.DatabaseAddEvents(user, events.Events)
	if err != nil {
		error_message = err.Error()
		return
	}

	broadcastToPollers(user.Id, events.Events[0])
}

func eventHandler(w http.ResponseWriter, r *http.Request) {
	user := AuthenticateUser(r)
	if user == nil {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"Asink Server\"")
		apiresponse := asink.APIResponse{
			Status:      asink.ERROR,
			Explanation: "This operation requires user authentication",
		}
		b, err := json.Marshal(apiresponse)
		if err != nil {
			b = []byte(err.Error())
		}
		w.WriteHeader(401)
		w.Write(b)
		return
	}
	if r.Method == "GET" {
		//if GET, return any events later than (and including) the event id passed in
		if sm := eventsRegexp.FindStringSubmatch(r.RequestURI); sm != nil {
			i, err := strconv.ParseUint(sm[1], 10, 64)
			if err != nil {
				//TODO display error message here instead
				fmt.Printf("ERROR parsing " + sm[1] + "\n")
				getEvents(w, r, user, 0)
			} else {
				getEvents(w, r, user, i)
			}
		} else {
			getEvents(w, r, user, 0)
		}
	} else if r.Method == "POST" {
		putEvents(w, r, user)
	} else {
		apiresponse := asink.APIResponse{
			Status:      asink.ERROR,
			Explanation: "Invalid HTTP method - only GET and POST are supported on this endpoint.",
		}
		b, _ := json.Marshal(apiresponse)
		w.Write(b)
	}
}

func AuthenticateUser(r *http.Request) (user *User) {
	h, ok := r.Header["Authorization"]
	if !ok {
		return nil
	}
	authparts := strings.Split(h[0], " ")
	if len(authparts) != 2 || authparts[0] != "Basic" {
		return nil
	}

	userpass, err := base64.StdEncoding.DecodeString(authparts[1])
	if err != nil {
		return nil
	}
	splituserpass := strings.Split(string(userpass), ":")
	if len(splituserpass) != 2 {
		return nil
	}

	user, err = adb.DatabaseGetUser(splituserpass[0])
	if err != nil || user == nil {
		return nil
	}

	if user.ValidPassword(splituserpass[1]) {
		return user
	} else {
		return nil
	}
}
