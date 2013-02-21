package util

import (
	"io"
	"io/ioutil"
	"os"
	"path"
)

func EnsureDirExists(dir string) error {
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

func CopyToTmp(src string, tmpdir string) (string, error) {
	infile, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer infile.Close()

	outfile, err := ioutil.TempFile(tmpdir, "asink")
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
