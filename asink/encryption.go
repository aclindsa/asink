/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"code.google.com/p/go.crypto/openpgp"
	"io"
)

func NewEncrypter(writer io.WriteCloser, key string) (plaintextWriter io.WriteCloser, err error) {
	return openpgp.SymmetricallyEncrypt(writer, []byte(key), nil, nil)
}

type Decrypter struct {
	details *openpgp.MessageDetails
}

func NewDecrypter(ciphertextReader io.ReadCloser, key string) (decrypter io.Reader, err error) {
	prompt := func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		return []byte(key), nil
	}

	details, err := openpgp.ReadMessage(ciphertextReader, nil, prompt, nil)
	if err != nil {
		decrypter = nil
		return
	}

	decrypter = Decrypter{details}

	return
}

func (d Decrypter) Read(p []byte) (n int, err error) {
	return d.details.UnverifiedBody.Read(p)
}
