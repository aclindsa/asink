/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"code.google.com/p/goconf/conf"
	"errors"
	"io"
)

type Storage interface {
	// Close() MUST be called on the returned io.WriteCloser
	Put(hash string) (io.WriteCloser, error)
	// Close() MUST be called on the returned io.ReadCloser
	Get(hash string) (io.ReadCloser, error)
}

func GetStorage(config *conf.ConfigFile) (Storage, error) {
	storageMethod, err := config.GetString("storage", "method")
	if err != nil {
		return nil, errors.New("Error: storage method not specified in config file.")
	}

	var storage Storage

	switch storageMethod {
	case "local":
		storage, err = NewLocalStorage(config)
	case "ftp":
		storage, err = NewFTPStorage(config)
	case "gdrive":
		storage, err = NewGDriveStorage(config)
	default:
		return nil, errors.New("Error: storage method '" + storageMethod + "' not found.")
	}

	if err != nil {
		return nil, err
	}

	return storage, nil
}
