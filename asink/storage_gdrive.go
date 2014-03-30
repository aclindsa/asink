/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goconf/conf"
	"code.google.com/p/google-api-go-client/drive/v2"
	"code.google.com/p/google-api-go-client/googleapi"
	"code.google.com/p/mxk/go1/flowcontrol"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"
	"reflect"
)

const GDRIVE_CLIENT_ID = "1006560298028.apps.googleusercontent.com"
const GDRIVE_CLIENT_SECRET = "2iTpEeN76RQK5KKF6ut1TCpV"
const GDRIVE_MAX_CONNECTIONS = 1 //should this be configurable?

type GDriveStorage struct {
	connectionsChan chan int
	cachefile string
	directory string
	auth_code string
	folderid  string
	transport *oauth.Transport
	service   *drive.Service
	max_kbps int64
}

func NewGDriveStorage(config *conf.ConfigFile) (*GDriveStorage, error) {
	cachefile, err := config.GetString("storage", "cachefile")
	if err != nil {
		return nil, errors.New("Error: GDriveStorage indicated in config file, but 'cachefile' not specified.")
	}

	code, err := config.GetString("storage", "oauth_code")
	if err != nil {
		code = ""
	}

	directory, err := config.GetString("storage", "directory")
	if err != nil {
		return nil, errors.New("Error: GDriveStorage indicated in config file, but 'directory' not specified.")
	}

	oauth_config := &oauth.Config{
		ClientId:     GDRIVE_CLIENT_ID,
		ClientSecret: GDRIVE_CLIENT_SECRET,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scope:        "https://www.googleapis.com/auth/drive",
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://accounts.google.com/o/oauth2/token",
		TokenCache:   oauth.CacheFile(cachefile),
	}

	transport := &oauth.Transport{Config: oauth_config}
	token, err := oauth_config.TokenCache.Token()
	if err != nil {
		//if a code wasn't specified in the config file, ask the user to do that
		if code == "" {
			url := oauth_config.AuthCodeURL("")
			return nil, errors.New(fmt.Sprintf("Visit the following URL and sign in using your Google account to get an authorization code allowing Asink to access your GDrive files. Be sure to add this code to your Asink config file as 'oauth_code = your_code_here' before re-starting Asink:\n%s\n", url))
		}

		//attempt to fetch a token using the user-supplied code (this
		//has the effect of caching the token in the specified cache
		//file)
		token, err = transport.Exchange(code)
		if err != nil {
			url := oauth_config.AuthCodeURL("")
			return nil, errors.New(fmt.Sprintf("Error exchanging user-supplied GDrive code for an authentication token. Please check your auth code supplied in the Asink config file, or consider obtaining another by visiting %s\n(%s)", url, err.Error()))
		}
	}

	//Now, actually initialize the GDrive part of the API
	transport.Token = token
	s, err := drive.New(transport.Client())
	if err != nil {
		return nil, err
	}

	folderlist, err := s.Files.List().Q("mimeType = 'application/vnd.google-apps.folder' and title = '" + directory + "'").Do()
	if err != nil {
		return nil, err
	}

	if len(folderlist.Items) < 1 {
		//try to create a folder named 'directory'
		f := &drive.File{Title: directory, Description: "Asink client folder", MimeType: "application/vnd.google-apps.folder"}
		f, err := s.Files.Insert(f).Do()

		folderlist, err = s.Files.List().Q("mimeType = 'application/vnd.google-apps.folder' and title = '" + directory + "'").Do()
		if err != nil {
			return nil, err
		} else if len(folderlist.Items) < 1 {
			return nil, errors.New("I was unable to create a new folder in your GDrive, but I'm not sure why")
		}
	} else if len(folderlist.Items) > 1 {
		return nil, errors.New(fmt.Sprintf("Error: Your GDrive has more than one directory named '%s'. You are a barbarian. Fix that and we'll talk. (check your trash if you can't find it)\n", directory))
	}

	folderid := folderlist.Items[0].Id

	gs := new(GDriveStorage)
	gs.cachefile = cachefile
	gs.directory = directory
	gs.auth_code = code
	gs.service = s
	gs.transport = transport
	gs.folderid = folderid
	gs.connectionsChan = make(chan int, GDRIVE_MAX_CONNECTIONS)
	gs.max_kbps = 512 //TODO make configurable
	return gs, nil
}

func (gs *GDriveStorage) fileExists(hash string) (bool, error) {
	folderlist, err := gs.service.Files.List().Q("mimeType = 'application/pgp-encrypted' and title = '" + hash + "' and '" + gs.folderid + "' in parents").Do()
	if err != nil {
		return false, err
	}
	return len(folderlist.Items) > 0, nil
}

func (gs *GDriveStorage) put(f *drive.File, reader *io.PipeReader, done chan error) {
	var err error
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	gs.connectionsChan <- 0

	for n := 0; n < 5; n++ {
		_, err = gs.service.Files.Insert(f).Media(reader).Do()
		if err != nil {
			if e, ok := err.(*googleapi.Error); ok && e.Code == 403 && strings.EqualFold(e.Message, "Rate Limit Exceeded") {
				wait_ms := time.Duration(2^n)*time.Second + time.Duration(rnd.Int31n(1000))*time.Millisecond
				time.Sleep(wait_ms)
				continue
			}
			fmt.Println("closing")
			fmt.Println(reflect.TypeOf(err).Name())
			fmt.Println(err)
			reader.CloseWithError(err)
			<- gs.connectionsChan
			done <- err
			return
		} else {
			reader.Close()
			<- gs.connectionsChan
			done <- nil
			return
		}
	}
	fmt.Println("closing")
	fmt.Println(reflect.TypeOf(err).Name())
	reader.CloseWithError(err)
	<- gs.connectionsChan
	done <- err
}

func (gs *GDriveStorage) Put(hash string, done chan error) (w io.WriteCloser, e error) {

	//TODO detect duplicates and don't re-upload this file if it already exists

	f := &drive.File{Title: hash, Description: hash, MimeType: "application/pgp-encrypted"}
	p := &drive.ParentReference{Id: gs.folderid}
	f.Parents = []*drive.ParentReference{p}

	reader, writer := io.Pipe()

	go gs.put(f, reader, done)

	limitedWriter := flowcontrol.NewWriter(writer, 1024/16*gs.max_kbps)
	limitedWriter.SetBlocking(true)

	return limitedWriter, nil
}

//should only be called if you have acquired the(a) lock for the GDrive storage
func (gs *GDriveStorage) getFolderList(hash string) (*drive.FileList, error) {
	var err error
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for n := 0; n < 5; n++ {
		folderlist, err := gs.service.Files.List().Q("mimeType = 'application/pgp-encrypted' and title = '" + hash + "' and '" + gs.folderid + "' in parents").Do()
		if err != nil {
			if e, ok := err.(*net.DNSError); ok && strings.EqualFold(e.Err, "no such host") {
				wait_ms := time.Duration(2^n)*time.Second + time.Duration(rnd.Int31n(1000))*time.Millisecond
				time.Sleep(wait_ms)
				continue
			}
			return nil, err
		} else {
			return folderlist, nil
		}
	}
	return nil, err
}

//should only be called if you have acquired the(a) lock for the GDrive storage
func (gs *GDriveStorage) getFile(url string) (*http.Response, error) {
	var err error
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for n := 0; n < 5; n++ {
		request, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		response, err := gs.transport.RoundTrip(request)
		if err != nil {
			if e, ok := err.(*net.DNSError); ok && strings.EqualFold(e.Err, "no such host") {
				wait_ms := time.Duration(2^n)*time.Second + time.Duration(rnd.Int31n(1000))*time.Millisecond
				time.Sleep(wait_ms)
				continue
			}
			return nil, err
		} else {
			return response, nil
		}
	}
	return nil, err
}

func (gs *GDriveStorage) Get(hash string) (io.ReadCloser, error) {
	gs.connectionsChan <- 0
	folderlist, err := gs.getFolderList(hash)
	if err != nil {
		return nil, err
	}
	if len(folderlist.Items) < 1 {
		return nil, errors.New(fmt.Sprintf("Error: '%s' not found", hash))
	}

	downloadUrl := folderlist.Items[0].DownloadUrl
	if downloadUrl == "" {
		return nil, errors.New(fmt.Sprintf("Error: content not found for '%s'", hash))
	}

	response, err := gs.getFile(downloadUrl)

	<- gs.connectionsChan

	return response.Body, nil
}
