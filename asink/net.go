/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"asink"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const MIN_ERROR_WAIT = 100   // 1/10 of a second
const MAX_ERROR_WAIT = 10000 // 10 seconds

func AuthenticatedRequest(method, url, bodyType string, body io.Reader, username, password string) (*http.Response, error) {
	client := &http.Client{}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if bodyType != "" {
		req.Header.Set("Content-Type", bodyType)
	}
	req.SetBasicAuth(username, password)
	return client.Do(req)
}
func AuthenticatedGet(url string, username, password string) (*http.Response, error) {
	return AuthenticatedRequest("GET", url, "", nil, username, password)
}
func AuthenticatedPost(url, bodyType string, body io.Reader, username, password string) (*http.Response, error) {
	return AuthenticatedRequest("POST", url, bodyType, body, username, password)
}

func SendEvent(globals AsinkGlobals, event *asink.Event) error {
	url := "http://" + globals.server + ":" + strconv.Itoa(int(globals.port)) + "/events/"

	//construct json payload
	events := asink.EventList{
		Events: []*asink.Event{event},
	}
	b, err := json.Marshal(events)
	if err != nil {
		return err
	}

	//actually make the request
	resp, err := AuthenticatedPost(url, "application/json", bytes.NewReader(b), globals.username, globals.password)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	//check to make sure request succeeded
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var apistatus asink.APIResponse
	err = json.Unmarshal(body, &apistatus)
	if err != nil {
		return err
	}
	if apistatus.Status != asink.SUCCESS {
		return errors.New("API response was not success: " + apistatus.Explanation)
	}

	return nil
}

func GetEvents(globals AsinkGlobals, events chan *asink.Event) {
	url := "http://" + globals.server + ":" + strconv.Itoa(int(globals.port)) + "/events/"
	var successiveErrors uint = 0

	errorWait := func(err error) {
		fmt.Println(err)
		var waitMilliseconds time.Duration = MIN_ERROR_WAIT << successiveErrors
		if waitMilliseconds > MAX_ERROR_WAIT {
			waitMilliseconds = MAX_ERROR_WAIT
		}
		time.Sleep(waitMilliseconds * time.Millisecond)
		successiveErrors++
	}

	//query DB for latest remote event version number that we've seen locally
	latestEvent, err := globals.db.DatabaseLatestRemoteEvent()
	if err != nil {
		panic(err)
	}

	for {
		//query for events after latest_event
		var fullUrl string
		if latestEvent != nil {
			fullUrl = url + strconv.FormatInt(latestEvent.Id+1, 10)
		} else {
			fullUrl = url + "0"
		}
		resp, err := AuthenticatedGet(fullUrl, globals.username, globals.password)

		//if error, perform exponential backoff (with maximum timeout)
		if err != nil {
			if resp != nil {
				resp.Body.Close()
			}
			errorWait(err)
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close() //must be done after the last time resp is used
		if err != nil {
			errorWait(err)
			continue
		}

		var apistatus asink.APIResponse
		err = json.Unmarshal(body, &apistatus)
		if err != nil {
			errorWait(err)
			continue
		}
		if apistatus.Status != asink.SUCCESS {
			errorWait(err)
			continue
		}

		for _, event := range apistatus.Events {
			if latestEvent != nil && event.Id != latestEvent.Id+1 {
				break
			}
			events <- event
			latestEvent = event
		}
		successiveErrors = 0
	}
}
