package main

import "github.com/rexlx/squall/internal"

type Database interface {
	GetMessage(roomid, messageid string) (internal.Message, error)
	StoreMessage(roomid string, message internal.Message) error
	GetUser(userid string) (User, error)
	StoreUser(user User) error
	GetRoom(roomid string) (Room, error)
	StoreRoom(room Room) error
}

type LiteDB struct {
	ID string
}

func (db *LiteDB) CreateTables() error {
	return nil
}

func (db *LiteDB) GetMessage(roomid, messageid string) (internal.Message, error) {
	return internal.Message{}, nil
}

func (db *LiteDB) StoreMessage(roomid string, message internal.Message) error {
	return nil
}

func (db *LiteDB) GetUser(userid string) (User, error) {
	return User{}, nil
}

func (db *LiteDB) StoreUser(user User) error {
	return nil
}

func (db *LiteDB) GetRoom(roomid string) (Room, error) {
	return Room{}, nil
}

func (db *LiteDB) StoreRoom(room Room) error {
	return nil
}
