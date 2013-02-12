package main

import (
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

func ensureDirExists(dir string) error {
	_, err := os.Lstat(dir)
	if err != nil {
		fi, err := os.Lstat(path.Dir(dir))
		if err != nil {
			return err
		}
		err = os.Mkdir(dir, fi.Mode().Perm())
		if err != nil {
			return err
		}
	}
	return nil
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
	err = ensureDirExists(ls.storageDir)
	if err != nil {
		return nil, err
	}
	err = ensureDirExists(ls.tmpSubdir)
	if err != nil {
		return nil, err
	}

	return ls, nil
}

func (ls *LocalStorage) copyToTmp(src string) (string, error) {
	infile, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer infile.Close()

	outfile, err := ioutil.TempFile(ls.tmpSubdir, "asink")
	if err != nil {
		return "", err
	}
	defer outfile.Close()

	_, err = io.Copy(outfile, infile)
	if err != nil {
		return "", err
	}

	return outfile.Name(), nil
}

func (ls *LocalStorage) Put(filename string, hash string) (e error) {
	tmpfile, err := ls.copyToTmp(filename)
	if err != nil {
		return err
	}

	err = os.Rename(tmpfile, path.Join(ls.storageDir, hash))
	if err != nil {
		err := os.Remove(tmpfile)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ls *LocalStorage) Get(filename string, hash string) error {
	infile, err := os.Open(path.Join(ls.storageDir, hash))
	if err != nil {
		return err
	}
	defer infile.Close()

	outfile, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer outfile.Close()

	_, err = io.Copy(outfile, infile)

	return err
}
