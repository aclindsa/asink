package main

import (
	"errors"
	"code.google.com/p/goconf/conf"
)

type Storage interface {
	Put(filename string, hash string) error
	Get(filename string, hash string) error
}

func GetStorage(config *conf.ConfigFile) (*Storage, error) {
	storageMethod, err := config.GetString("storage", "method")
	if err != nil {
		return nil, errors.New("Error: storage method not specified in config file.")
	}

	var storage Storage

	switch storageMethod {
		case "local":
			storage, err = NewLocalStorage(config)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("Error: storage method '" + storageMethod + "' not implemented.")
	}

	return &storage, nil
}
