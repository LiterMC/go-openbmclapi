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

	"github.com/google/uuid"

	"github.com/LiterMC/go-openbmclapi/utils"
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

	GetEmailSubscription(user string, addr string) (*EmailSubscriptionRecord, error)
	AddEmailSubscription(EmailSubscriptionRecord) error
	UpdateEmailSubscription(EmailSubscriptionRecord) error
	RemoveEmailSubscription(user string, addr string) error
	ForEachEmailSubscription(cb func(*EmailSubscriptionRecord) error) error
	ForEachUsersEmailSubscription(user string, cb func(*EmailSubscriptionRecord) error) error
	ForEachEnabledEmailSubscription(cb func(*EmailSubscriptionRecord) error) error

	GetWebhook(user string, id uuid.UUID) (*WebhookRecord, error)
	AddWebhook(WebhookRecord) error
	UpdateWebhook(WebhookRecord) error
	UpdateEnableWebhook(user string, id uuid.UUID, enabled bool) error
	RemoveWebhook(user string, id uuid.UUID) error
	ForEachWebhook(cb func(*WebhookRecord) error) error
	ForEachUsersWebhook(user string, cb func(*WebhookRecord) error) error
	ForEachEnabledWebhook(cb func(*WebhookRecord) error) error
}

type FileRecord struct {
	Path string
	Hash string
	Size int64
}

type SubscribeRecord struct {
	User       string              `json:"user"`
	Client     string              `json:"client"`
	EndPoint   string              `json:"endpoint"`
	Keys       SubscribeRecordKeys `json:"keys"`
	Scopes     NotificationScopes  `json:"scopes"`
	ReportAt   Schedule            `json:"report_at"`
	LastReport sql.NullTime        `json:"-"`
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
	SyncBegin   bool `json:"syncbegin"`
	SyncDone    bool `json:"syncdone"`
	Updates     bool `json:"updates"`
	DailyReport bool `json:"dailyreport"`
}

var (
	_ sql.Scanner   = (*NotificationScopes)(nil)
	_ driver.Valuer = (*NotificationScopes)(nil)
)

//// !!WARN: Do not edit nsFlag's order ////

const (
	nsFlagDisabled = 1 << iota
	nsFlagEnabled
	nsFlagSyncDone
	nsFlagUpdates
	nsFlagDailyReport
	nsFlagSyncBegin
)

func (ns NotificationScopes) ToInt64() (v int64) {
	if ns.Disabled {
		v |= nsFlagDisabled
	}
	if ns.Enabled {
		v |= nsFlagEnabled
	}
	if ns.SyncBegin {
		v |= nsFlagSyncBegin
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
	ns.SyncBegin = v&nsFlagSyncBegin != 0
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
		case "syncbegin":
			ns.SyncBegin = true
		case "syncdone":
			ns.SyncDone = true
		case "updates":
			ns.Updates = true
		case "dailyreport":
			ns.DailyReport = true
		}
	}
}

func (ns *NotificationScopes) UnmarshalJSON(data []byte) (err error) {
	{
		type T NotificationScopes
		if err = json.Unmarshal(data, (*T)(ns)); err == nil {
			return
		}
	}
	var v []string
	if err = json.Unmarshal(data, &v); err != nil {
		return
	}
	ns.FromStrings(v)
	return
}

type Schedule struct {
	Hour   int
	Minute int
}

var (
	_ sql.Scanner   = (*Schedule)(nil)
	_ driver.Valuer = (*Schedule)(nil)
)

func (s Schedule) String() string {
	return fmt.Sprintf("%02d:%02d", s.Hour, s.Minute)
}

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

func (s *Schedule) UnmarshalJSON(buf []byte) (err error) {
	var v string
	if err = json.Unmarshal(buf, &v); err != nil {
		return
	}
	return s.UnmarshalText(([]byte)(v))
}

func (s *Schedule) MarshalJSON() (buf []byte, err error) {
	return json.Marshal(s.String())
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
	return s.String(), nil
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

type EmailSubscriptionRecord struct {
	User    string             `json:"user"`
	Addr    string             `json:"addr"`
	Scopes  NotificationScopes `json:"scopes"`
	Enabled bool               `json:"enabled"`
}

type WebhookRecord struct {
	User     string             `json:"user"`
	Id       uuid.UUID          `json:"id"`
	Name     string             `json:"name"`
	EndPoint string             `json:"endpoint"`
	Auth     *string            `json:"auth,omitempty"`
	AuthHash string             `json:"authHash,omitempty"`
	Scopes   NotificationScopes `json:"scopes"`
	Enabled  bool               `json:"enabled"`
}

func (rec *WebhookRecord) CovertAuthHash() {
	if rec.Auth == nil || *rec.Auth == "" {
		rec.AuthHash = ""
	} else {
		rec.AuthHash = "sha256:" + utils.AsSha256Hex(*rec.Auth)
	}
	rec.Auth = nil
}
