/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package util

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"syscall"
)

func EnsureDirExists(dir string) error {
	_, err := os.Lstat(dir)
	if err != nil {
		var fi os.FileInfo
		curDir := dir
		for dir != "" && err != nil {
			curDir = path.Dir(curDir)
			fi, err = os.Lstat(curDir)
		}
		if err != nil {
			return err
		}
		err = os.MkdirAll(dir, fi.Mode().Perm())
		if err != nil {
			return err
		}
	}
	return nil
}

func FileExistsAndHasPermissions(file string, mode os.FileMode) bool {
	info, err := os.Stat(file)
	if err != nil {
		return false
	}
	return info.Mode().Perm() == mode
}

//TODO maybe this shouldn't fail silently?
func RecursiveRemoveEmptyDirs(dir string) {
	var err error = nil
	curDir := dir
	for err == nil {
		err = os.Remove(curDir)
		curDir = path.Dir(curDir)
	}
}

func CopyReaderToTmp(src io.Reader, tmpdir string) (string, error) {
	outfile, err := ioutil.TempFile(tmpdir, "asink")
	if err != nil {
		return "", err
	}
	defer outfile.Close()

	_, err = io.Copy(outfile, src)
	if err != nil {
		return "", err
	}

	return outfile.Name(), nil
}

func CopyToTmp(src string, tmpdir string) (string, error) {
	infile, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer infile.Close()

	return CopyReaderToTmp(infile, tmpdir)
}

func ErrorFileNotFound(err error) bool {
	if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOENT {
		return true
	}
	return false
}
