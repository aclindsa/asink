package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

func HashFile(filename string) (string, error) {
	hashfn := sha256.New()

	infile, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer infile.Close()

	_, err = io.Copy(hashfn, infile)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hashfn.Sum(nil)), nil
}
