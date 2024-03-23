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
	"encoding/json"
	"errors"
	"fmt"
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
	User       string
	Client     string
	EndPoint   string
	Keys       SubscribeRecordKeys
	Scopes     NotificationScopes
	ReportAt   Schedule
	LastReport sql.NullTime
}

type SubscribeRecordKeys struct {
	Auth   string `json:"auth"`
	P256dh string `json:"p256dh"`
}

var (
	_ sql.Scanner   = (*SubscribeRecordKeys)(nil)
	_ driver.Valuer = (*SubscribeRecordKeys)(nil)
)

func (sk *SubscribeRecordKeys) Scan(src any) error {
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = ([]byte)(v)
	default:
		return errors.New("Source is not a string")
	}
	return json.Unmarshal(data, sk)
}

func (sk SubscribeRecordKeys) Value() (driver.Value, error) {
	return json.Marshal(sk)
}

type NotificationScopes struct {
	Disabled    bool `json:"disabled"`
	Enabled     bool `json:"enabled"`
	SyncDone    bool `json:"syncdone"`
	Updates     bool `json:"updates"`
	DailyReport bool `json:"dailyreport"`
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
	nsFlagDailyReport
)

func (ns NotificationScopes) ToInt64() (v int64) {
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
	if ns.DailyReport {
		v |= nsFlagDailyReport
	}
	return
}

func (ns *NotificationScopes) FromInt64(v int64) {
	ns.Disabled = v&nsFlagDisabled != 0
	ns.Enabled = v&nsFlagEnabled != 0
	ns.SyncDone = v&nsFlagSyncDone != 0
	ns.Updates = v&nsFlagUpdates != 0
	ns.DailyReport = v&nsFlagDailyReport != 0
}

func (ns *NotificationScopes) Scan(src any) error {
	v, ok := src.(int64)
	if !ok {
		return errors.New("Source is not a integer")
	}
	ns.FromInt64(v)
	return nil
}

func (ns NotificationScopes) Value() (driver.Value, error) {
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
		case "dailyreport":
			ns.DailyReport = true
		}
	}
}

type Schedule struct {
	Hour   int
	Minute int
}

var (
	_ sql.Scanner   = (*Schedule)(nil)
	_ driver.Valuer = (*Schedule)(nil)
)

func (s *Schedule) UnmarshalText(buf []byte) (err error) {
	if _, err = fmt.Sscanf((string)(buf), "%02d:%02d", &s.Hour, &s.Minute); err != nil {
		return
	}
	if s.Hour < 0 || s.Hour >= 24 {
		return fmt.Errorf("Hour %d out of range [0, 24)", s.Hour)
	}
	if s.Minute < 0 || s.Minute >= 60 {
		return fmt.Errorf("Minute %d out of range [0, 60)", s.Minute)
	}
	return
}

func (s *Schedule) Scan(src any) error {
	var v []byte
	switch w := src.(type) {
	case []byte:
		v = w
	case string:
		v = ([]byte)(w)
	default:
		return fmt.Errorf("Unexpected type %T", src)
	}
	return s.UnmarshalText(v)
}

func (s Schedule) Value() (driver.Value, error) {
	return fmt.Sprintf("%02d:%02d", s.Hour, s.Minute), nil
}

func (s Schedule) ReadySince(last, now time.Time) bool {
	if last.IsZero() {
		last = now.Add(-time.Hour*24 + 1)
	}
	mustAfter := last.Add(time.Hour * 12)
	if now.Before(mustAfter) {
		return false
	}
	if !now.Before(last.Add(time.Hour * 24)) {
		return true
	}
	hour, min := now.Hour(), now.Minute()
	if s.Hour < hour && s.Hour+3 > hour || s.Hour == hour && s.Minute <= min {
		return true
	}
	return false
}
