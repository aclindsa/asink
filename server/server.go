package main

import (
	"asink"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
)

//global variables
var eventsRegexp *regexp.Regexp
var port int = 8080
var adb *AsinkDB

func init() {
	var err error
	const port_usage = "Port on which to serve HTTP API"

	flag.IntVar(&port, "port", 8080, port_usage)
	flag.IntVar(&port, "p", 8080, port_usage+" (shorthand)")

	eventsRegexp = regexp.MustCompile("^/events/([0-9]+)$")

	adb, err = GetAndInitDB()
	if err != nil {
		panic(err)
	}
}

func main() {
	flag.Parse()

	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/events", eventHandler)
	http.HandleFunc("/events/", eventHandler)

	//TODO replace with http://golang.org/pkg/net/http/#ListenAndServeTLS
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		fmt.Println(err)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "You're probably looking for /events/")
}

func getEvents(w http.ResponseWriter, r *http.Request, nextEvent uint64) {
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

	events, err := adb.DatabaseRetrieveEvents(nextEvent, 50)
	if err != nil {
		panic(err)
		error_message = err.Error()
		return
	}

	//long-poll if events is empty
	if len(events) == 0 {
		c := make(chan *asink.Event)
		addPoller("aclindsa", &c) //TODO support more than one user
		e, ok := <-c
		if ok {
			events = append(events, e)
		}
	}
}

func putEvents(w http.ResponseWriter, r *http.Request) {
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
	for _, event := range events.Events {
		err = adb.DatabaseAddEvent(event)
		if err != nil {
			//TODO should probably do this in a way that the caller knows how many of these have failed and doesn't re-try sending ones that succeeded
			//i.e. add this to the return codes or something
			//OR put all the DatabaseAddEvent's inside a SQL transaction, and rollback on any failure
			error_message = err.Error()
			return
		}
	}

	broadcastToPollers("aclindsa", events.Events[0]) //TODO support more than one user
}

func eventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		//if GET, return any events later than (and including) the event id passed in
		if sm := eventsRegexp.FindStringSubmatch(r.RequestURI); sm != nil {
			i, err := strconv.ParseUint(sm[1], 10, 64)
			if err != nil {
				//TODO display error message here instead
				fmt.Printf("ERROR parsing " + sm[1] + "\n")
				getEvents(w, r, 0)
			} else {
				getEvents(w, r, i)
			}
		} else {
			getEvents(w, r, 0)
		}
	} else if r.Method == "POST" {
		putEvents(w, r)
	} else {
		apiresponse := asink.APIResponse{
			Status:      asink.ERROR,
			Explanation: "Invalid HTTP method - only GET and POST are supported on this endpoint.",
		}
		b, _ := json.Marshal(apiresponse)
		w.Write(b)
	}
}
