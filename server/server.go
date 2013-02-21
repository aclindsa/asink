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

var eventsRegexp *regexp.Regexp

var port int = 8080
func init() {
	const port_usage = "Port on which to serve HTTP API"

	flag.IntVar(&port, "port", 8080, port_usage)
	flag.IntVar(&port, "p", 8080, port_usage+" (shorthand)")

	eventsRegexp = regexp.MustCompile("^/events/([0-9]+)$")
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
	fmt.Fprintf(w, strconv.FormatUint(nextEvent, 10))
}

func putEvents(w http.ResponseWriter, r *http.Request) {
	var events asink.EventList
	var error_occurred bool = false
	var error_message string = ""
	defer func() {
		var apiresponse asink.APIResponse
		if error_occurred {
			apiresponse = asink.APIResponse{
				Status: "error",
				Explanation: error_message,
			}
		} else {
			apiresponse = asink.APIResponse{
				Status: "success",
			}
		}
		b, err := json.Marshal(apiresponse)
		if err != nil {
			panic(err)
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
		fmt.Println(event)
	}
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
		fmt.Fprintf(w, "You f-ed up.")
	}
}
