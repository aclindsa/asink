/*
 Copyright (C) 2013 Aaron Lindsay <aaron@aclindsay.com>
*/

package main

import (
	"crypto/sha256"
	"fmt"
)

type UserRole uint32

const (
	//User roles
	ADMIN = 1 << iota
	NORMAL
)

type User struct {
	Id       int64
	Username string
	PWHash   string
	Role     UserRole
}

func HashPassword(pw string) string {
	hashfn := sha256.New()
	hashfn.Write([]byte(pw))
	return fmt.Sprintf("%x", hashfn.Sum(nil))
}

func (u *User) ValidPassword(pw string) bool {
	return HashPassword(pw) == u.PWHash
}

func (u *User) IsAdmin() bool {
	return u.Role&ADMIN == ADMIN
}
