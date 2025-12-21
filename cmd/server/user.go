package main

import (
	"errors"
	"time"

	"github.com/rexlx/squall/internal"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Rooms    []string          `json:"rooms"`
	History  []string          `json:"history"`
	ID       string            `json:"id"`
	Email    string            `json:"email"`
	Password string            `json:"password"`
	Name     string            `json:"name"`
	Created  time.Time         `json:"created"`
	Updated  time.Time         `json:"updated"`
	Stats    internal.AppStats `json:"stats"`
	Posts    []internal.Post   `json:"posts"`
}

// SetPassword hashes the input password and stores it in the User struct
func (u *User) SetPassword(input string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(input), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hash)
	return nil
}

func (u *User) PasswordMatches(input string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(input))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			//invalid password
			return false, nil
		default:
			//unknown error
			return false, err
		}
	}
	return true, nil
}

func (u *User) GetUserStats() internal.AppStats {
	return u.Stats
}
