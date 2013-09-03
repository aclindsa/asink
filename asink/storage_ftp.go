package main

import (
	"code.google.com/p/goconf/conf"
	"errors"
	"github.com/jlaffaye/goftp"
	"io"
	"os"
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

func (fs *FTPStorage) Put(filename string, hash string) (e error) {
	//make sure we don't flood the FTP server
	fs.connectionsChan <- 0
	defer func() { <-fs.connectionsChan }()

	infile, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer infile.Close()

	connection, err := ftp.Connect(fs.server + ":" + strconv.Itoa(fs.port))
	if err != nil {
		return err
	}
	defer connection.Quit()

	err = connection.Login(fs.username, fs.password)
	if err != nil {
		return err
	}

	err = connection.ChangeDir(fs.directory)
	if err != nil {
		return err
	}

	return connection.Stor(hash, infile)
}

func (fs *FTPStorage) Get(filename string, hash string) error {
	fs.connectionsChan <- 0
	defer func() { <-fs.connectionsChan }()

	connection, err := ftp.Connect(fs.server + ":" + strconv.Itoa(fs.port))
	if err != nil {
		return err
	}
	defer connection.Quit()

	err = connection.Login(fs.username, fs.password)
	if err != nil {
		return err
	}

	err = connection.ChangeDir(fs.directory)
	if err != nil {
		return err
	}

	downloadedFile, err := connection.Retr(hash)
	if err != nil {
		return err
	}
	defer downloadedFile.Close()

	outfile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer outfile.Close()

	_, err = io.Copy(outfile, downloadedFile)
	return err
}
