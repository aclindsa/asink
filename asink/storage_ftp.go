/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"code.google.com/p/goconf/conf"
	"errors"
	"github.com/jlaffaye/goftp"
	"io"
	"strconv"
)

const FTP_MAX_CONNECTIONS = 10 //should this be configurable?

type FTPStorage struct {
	connectionsChan chan int
	server          string
	port            int
	directory       string
	username        string
	password        string
}

func NewFTPStorage(config *conf.ConfigFile) (*FTPStorage, error) {
	server, err := config.GetString("storage", "server")
	if err != nil {
		return nil, errors.New("Error: FTPStorage indicated in config file, but 'server' not specified.")
	}
	port, err := config.GetInt("storage", "port")
	if err != nil {
		return nil, errors.New("Error: FTPStorage indicated in config file, but 'port' not specified.")
	}
	directory, err := config.GetString("storage", "directory")
	if err != nil {
		return nil, errors.New("Error: FTPStorage indicated in config file, but 'directory' not specified.")
	}
	username, err := config.GetString("storage", "username")
	if err != nil {
		return nil, errors.New("Error: FTPStorage indicated in config file, but 'username' not specified.")
	}
	password, err := config.GetString("storage", "password")
	if err != nil {
		return nil, errors.New("Error: FTPStorage indicated in config file, but 'password' not specified.")
	}

	fs := new(FTPStorage)
	fs.server = server
	fs.port = port
	fs.directory = directory
	fs.username = username
	fs.password = password

	fs.connectionsChan = make(chan int, FTP_MAX_CONNECTIONS)

	return fs, nil
}

func (fs *FTPStorage) Put(hash string, done chan error) (w io.WriteCloser, e error) {
	returningNormally := false
	//make sure we don't flood the FTP server
	fs.connectionsChan <- 0
	defer func() {
		if !returningNormally {
			<-fs.connectionsChan
		}
	}()

	connection, err := ftp.Connect(fs.server + ":" + strconv.Itoa(fs.port))
	if err != nil {
		return nil, err
	}
	defer func() {
		if !returningNormally {
			connection.Quit()
		}
	}()

	err = connection.Login(fs.username, fs.password)
	if err != nil {
		return nil, err
	}

	err = connection.ChangeDir(fs.directory)
	if err != nil {
		return nil, err
	}

	reader, writer := io.Pipe()

	go func() {
		err := connection.Stor(hash, reader)
		if err != nil {
			reader.CloseWithError(err)
		}
		<-fs.connectionsChan
		connection.Quit()
		done <- err
	}()

	returningNormally = true
	return writer, nil
}

func (fs *FTPStorage) Get(hash string) (io.ReadCloser, error) {
	fs.connectionsChan <- 0
	defer func() { <-fs.connectionsChan }()

	connection, err := ftp.Connect(fs.server + ":" + strconv.Itoa(fs.port))
	if err != nil {
		return nil, err
	}
	defer connection.Quit()

	err = connection.Login(fs.username, fs.password)
	if err != nil {
		return nil, err
	}

	err = connection.ChangeDir(fs.directory)
	if err != nil {
		return nil, err
	}

	return connection.Retr(hash)
}
