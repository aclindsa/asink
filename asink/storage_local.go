/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"asink/util"
	"code.google.com/p/goconf/conf"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
)

type LocalStorage struct {
	storageDir string
	tmpSubdir  string
}

func NewLocalStorage(config *conf.ConfigFile) (*LocalStorage, error) {
	storageDir, err := config.GetString("storage", "dir")
	if err != nil {
		return nil, errors.New("Error: LocalStorage indicated in config file, but lacking local storage directory ('dir = some/dir').")
	}

	ls := new(LocalStorage)
	ls.storageDir = storageDir
	ls.tmpSubdir = path.Join(storageDir, ".asink-tmpdir")

	//make sure the base directory and tmp subdir exist
	err = util.EnsureDirExists(ls.storageDir)
	if err != nil {
		return nil, err
	}
	err = util.EnsureDirExists(ls.tmpSubdir)
	if err != nil {
		return nil, err
	}

	return ls, nil
}

type putWriteCloser struct {
	outfile  *os.File
	filename string
}

func (wc putWriteCloser) Write(p []byte) (n int, err error) {
	return wc.outfile.Write(p)
}

func (wc putWriteCloser) Close() error {
	tmpfilename := wc.outfile.Name()
	wc.outfile.Close()

	err := os.Rename(tmpfilename, wc.filename)
	if err != nil {
		err := os.Remove(tmpfilename)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ls *LocalStorage) Put(hash string) (w io.WriteCloser, e error) {
	outfile, err := ioutil.TempFile(ls.tmpSubdir, "asink")
	if err != nil {
		return nil, err
	}

	w = putWriteCloser{outfile, path.Join(ls.storageDir, hash)}

	return
}

func (ls *LocalStorage) Get(hash string) (r io.ReadCloser, e error) {
	r, err := os.Open(path.Join(ls.storageDir, hash))
	if err != nil {
		return nil, err
	}
	return
}
