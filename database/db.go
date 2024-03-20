/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
 * All rights reserved
 *
 *  This program is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU Affero General Public License as published
 *  by the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU Affero General Public License for more details.
 *
 *  You should have received a copy of the GNU Affero General Public License
 *  along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package database

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"
)

var (
	ErrStopIter = errors.New("stop iteration")
	ErrNotFound = errors.New("no record was found")
	ErrExists   = errors.New("record's key was already exists")
)

type DB interface {
	// Cleanup will release any release that the database created
	// No operation should be executed during or after cleanup
	Cleanup() (err error)

	ValidJTI(jti string) (bool, error)
	AddJTI(jti string, expire time.Time) error
	RemoveJTI(jti string) error

	// You should not edit the record pointer
	GetFileRecord(path string) (*FileRecord, error)
	SetFileRecord(FileRecord) error
	RemoveFileRecord(path string) error
	// if the callback returns ErrStopIter, ForEach must immediately stop and returns a nil error
	// the callback should not edit the record pointer
	ForEachFileRecord(cb func(*FileRecord) error) error

	GetSubscribe(user string, client string) (*SubscribeRecord, error)
	SetSubscribe(SubscribeRecord) error
	RemoveSubscribe(user string, client string) error
	ForEachSubscribe(cb func(*SubscribeRecord) error) error
}

type FileRecord struct {
	Path string
	Hash string
	Size int64
}

type SubscribeRecord struct {
	User     string
	Client   string
	EndPoint string
	Scopes   NotificationScopes
}

type NotificationScopes struct {
	Disabled bool `json:"disabled"`
	Enabled  bool `json:"enabled"`
	SyncDone bool `json:"syncdone"`
	Updates  bool `json:"updates"`
}

var (
	_ sql.Scanner   = (*NotificationScopes)(nil)
	_ driver.Valuer = (*NotificationScopes)(nil)
)

const (
	nsFlagDisabled = 1 << iota
	nsFlagEnabled
	nsFlagSyncDone
	nsFlagUpdates
)

func (ns *NotificationScopes) ToInt64() (v int64) {
	if ns.Disabled {
		v |= nsFlagDisabled
	}
	if ns.Enabled {
		v |= nsFlagEnabled
	}
	if ns.SyncDone {
		v |= nsFlagSyncDone
	}
	if ns.Updates {
		v |= nsFlagUpdates
	}
	return
}

func (ns *NotificationScopes) FromInt64(v int64) {
	ns.Disabled = v&nsFlagDisabled != 0
	ns.Enabled = v&nsFlagEnabled != 0
	ns.SyncDone = v&nsFlagSyncDone != 0
	ns.Updates = v&nsFlagUpdates != 0
}

func (ns *NotificationScopes) Scan(src any) error {
	v, ok := src.(int64)
	if !ok {
		return errors.New("Source is not a integer")
	}
	ns.FromInt64(v)
	return nil
}

func (ns *NotificationScopes) Value() (driver.Value, error) {
	return ns.ToInt64(), nil
}

func (ns *NotificationScopes) FromStrings(scopes []string) {
	for _, s := range scopes {
		switch s {
		case "disabled":
			ns.Disabled = true
		case "enabled":
			ns.Enabled = true
		case "syncdone":
			ns.SyncDone = true
		case "updates":
			ns.Updates = true
		}
	}
}
