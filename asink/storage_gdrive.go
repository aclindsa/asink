/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goconf/conf"
	"code.google.com/p/google-api-go-client/drive/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const GDRIVE_CLIENT_ID = "1006560298028.apps.googleusercontent.com"
const GDRIVE_CLIENT_SECRET = "2iTpEeN76RQK5KKF6ut1TCpV"

type GDriveStorage struct {
	cachefile string
	directory string
	auth_code string
	folderid  string
	transport *oauth.Transport
	service   *drive.Service
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
	return gs, nil
}

func (gs *GDriveStorage) Put(hash string) (w io.WriteCloser, e error) {

	//TODO detect duplicates and don't re-upload this file if it already exists

	f := &drive.File{Title: hash, Description: hash, MimeType: "application/pgp-encrypted"}
	p := &drive.ParentReference{Id: gs.folderid}
	f.Parents = []*drive.ParentReference{p}

	reader, writer := io.Pipe()

	go func() {
		_, err := gs.service.Files.Insert(f).Media(reader).Do()
		if err != nil {
			reader.CloseWithError(err)
		}
	}()

	return writer, nil
}

func (gs *GDriveStorage) Get(hash string) (io.ReadCloser, error) {
	folderlist, err := gs.service.Files.List().Q("mimeType = 'application/pgp-encrypted' and title = '" + hash + "' and '" + gs.folderid + "' in parents").Do()
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

	request, err := http.NewRequest("GET", downloadUrl, nil)
	if err != nil {
		return nil, err
	}
	response, err := gs.transport.RoundTrip(request)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}
