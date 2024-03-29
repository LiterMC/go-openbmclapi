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
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/LiterMC/go-openbmclapi/utils"
)

type webhookMemKey struct {
	User string
	Id   uuid.UUID
}

type MemoryDB struct {
	fileRecMux  sync.RWMutex
	fileRecords map[string]*FileRecord

	tokenMux sync.RWMutex
	tokens   map[string]time.Time

	subscribeMux     sync.RWMutex
	subscribeRecords map[[2]string]*SubscribeRecord

	emailSubMux     sync.RWMutex
	emailSubRecords map[[2]string]*EmailSubscriptionRecord

	webhookMux     sync.RWMutex
	webhookRecords map[webhookMemKey]*WebhookRecord
}

var _ DB = (*MemoryDB)(nil)

func NewMemoryDB() *MemoryDB {
	return &MemoryDB{
		fileRecords:      make(map[string]*FileRecord),
		tokens:           make(map[string]time.Time),
		subscribeRecords: make(map[[2]string]*SubscribeRecord),
	}
}

func (m *MemoryDB) Cleanup() (err error) {
	m.fileRecords = nil
	m.tokens = nil
	return
}

func (m *MemoryDB) ValidJTI(jti string) (bool, error) {
	m.tokenMux.RLock()
	defer m.tokenMux.RUnlock()

	expire, ok := m.tokens[jti]
	if !ok {
		return false, ErrNotFound
	}
	if time.Now().After(expire) {
		return false, nil
	}
	return true, nil
}

func (m *MemoryDB) AddJTI(jti string, expire time.Time) error {
	m.tokenMux.Lock()
	defer m.tokenMux.Unlock()
	if _, ok := m.tokens[jti]; ok {
		return ErrExists
	}
	m.tokens[jti] = expire
	return nil
}

func (m *MemoryDB) RemoveJTI(jti string) error {
	m.tokenMux.RLock()
	_, ok := m.tokens[jti]
	m.tokenMux.RUnlock()
	if !ok {
		return ErrNotFound
	}

	m.tokenMux.Lock()
	defer m.tokenMux.Unlock()
	if _, ok := m.tokens[jti]; !ok {
		return ErrNotFound
	}
	delete(m.tokens, jti)
	return nil
}

func (m *MemoryDB) GetFileRecord(path string) (*FileRecord, error) {
	m.fileRecMux.RLock()
	defer m.fileRecMux.RUnlock()

	record, ok := m.fileRecords[path]
	if !ok {
		return nil, ErrNotFound
	}
	return record, nil
}

func (m *MemoryDB) SetFileRecord(record FileRecord) error {
	m.fileRecMux.Lock()
	defer m.fileRecMux.Unlock()

	old, ok := m.fileRecords[record.Path]
	if ok && *old == record {
		return nil
	}
	m.fileRecords[record.Path] = &record
	return nil
}

func (m *MemoryDB) RemoveFileRecord(path string) error {
	m.fileRecMux.Lock()
	defer m.fileRecMux.Unlock()

	if _, ok := m.fileRecords[path]; !ok {
		return ErrNotFound
	}
	delete(m.fileRecords, path)
	return nil
}

func (m *MemoryDB) ForEachFileRecord(cb func(*FileRecord) error) error {
	m.fileRecMux.RLock()
	defer m.fileRecMux.RUnlock()

	for _, v := range m.fileRecords {
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}

func (m *MemoryDB) GetSubscribe(user string, client string) (*SubscribeRecord, error) {
	m.subscribeMux.RLock()
	defer m.subscribeMux.RUnlock()

	record, ok := m.subscribeRecords[[2]string{user, client}]
	if !ok {
		return nil, ErrNotFound
	}
	return record, nil
}

func (m *MemoryDB) SetSubscribe(record SubscribeRecord) error {
	m.subscribeMux.Lock()
	defer m.subscribeMux.Unlock()

	key := [2]string{record.User, record.Client}
	if record.EndPoint == "" {
		old, ok := m.subscribeRecords[key]
		if !ok {
			return ErrNotFound
		}
		record.EndPoint = old.EndPoint
	}
	m.subscribeRecords[key] = &record
	return nil
}

func (m *MemoryDB) RemoveSubscribe(user string, client string) error {
	m.subscribeMux.Lock()
	defer m.subscribeMux.Unlock()

	key := [2]string{user, client}
	_, ok := m.subscribeRecords[key]
	if !ok {
		return ErrNotFound
	}
	delete(m.subscribeRecords, key)
	return nil
}

func (m *MemoryDB) ForEachSubscribe(cb func(*SubscribeRecord) error) error {
	m.subscribeMux.RLock()
	defer m.subscribeMux.RUnlock()

	for _, v := range m.subscribeRecords {
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}

func (m *MemoryDB) GetEmailSubscription(user string, addr string) (*EmailSubscriptionRecord, error) {
	m.emailSubMux.RLock()
	defer m.emailSubMux.RUnlock()

	record, ok := m.emailSubRecords[[2]string{user, addr}]
	if !ok {
		return nil, ErrNotFound
	}
	return record, nil
}

func (m *MemoryDB) AddEmailSubscription(record EmailSubscriptionRecord) error {
	m.emailSubMux.Lock()
	defer m.emailSubMux.Unlock()

	key := [2]string{record.User, record.Addr}
	if _, ok := m.emailSubRecords[key]; ok {
		return ErrExists
	}
	m.emailSubRecords[key] = &record
	return nil
}

func (m *MemoryDB) UpdateEmailSubscription(record EmailSubscriptionRecord) error {
	m.emailSubMux.Lock()
	defer m.emailSubMux.Unlock()

	key := [2]string{record.User, record.Addr}
	old, ok := m.emailSubRecords[key]
	if ok {
		return ErrNotFound
	}
	_ = old
	m.emailSubRecords[key] = &record
	return nil
}

func (m *MemoryDB) RemoveEmailSubscription(user string, addr string) error {
	m.emailSubMux.Lock()
	defer m.emailSubMux.Unlock()

	key := [2]string{user, addr}
	if _, ok := m.emailSubRecords[key]; ok {
		return ErrNotFound
	}
	delete(m.emailSubRecords, key)
	return nil
}

func (m *MemoryDB) ForEachEmailSubscription(cb func(*EmailSubscriptionRecord) error) error {
	m.emailSubMux.RLock()
	defer m.emailSubMux.RUnlock()

	for _, v := range m.emailSubRecords {
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}

func (m *MemoryDB) ForEachUsersEmailSubscription(user string, cb func(*EmailSubscriptionRecord) error) error {
	m.emailSubMux.RLock()
	defer m.emailSubMux.RUnlock()

	for _, v := range m.emailSubRecords {
		if v.User != user {
			continue
		}
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}

func (m *MemoryDB) ForEachEnabledEmailSubscription(cb func(*EmailSubscriptionRecord) error) error {
	m.emailSubMux.RLock()
	defer m.emailSubMux.RUnlock()

	for _, v := range m.emailSubRecords {
		if !v.Enabled {
			continue
		}
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}

func (m *MemoryDB) GetWebhook(user string, id uuid.UUID) (*WebhookRecord, error) {
	m.webhookMux.RLock()
	defer m.webhookMux.RUnlock()

	record, ok := m.webhookRecords[webhookMemKey{user, id}]
	if !ok {
		return nil, ErrNotFound
	}
	return record, nil
}

var (
	emptyStr    = ""
	emptyStrPtr = &emptyStr
)

func (m *MemoryDB) AddWebhook(record WebhookRecord) (err error) {
	m.webhookMux.Lock()
	defer m.webhookMux.Unlock()

	if record.Id, err = uuid.NewV7(); err != nil {
		return
	}

	key := webhookMemKey{record.User, record.Id}
	if _, ok := m.webhookRecords[key]; ok {
		return ErrExists
	}
	if record.Auth == nil {
		record.Auth = emptyStrPtr
	}
	if auth := *record.Auth; auth != "" {
		record.AuthHash = utils.AsSha256(auth)
	}
	m.webhookRecords[key] = &record
	return nil
}

func (m *MemoryDB) UpdateWebhook(record WebhookRecord) error {
	m.webhookMux.Lock()
	defer m.webhookMux.Unlock()

	key := webhookMemKey{record.User, record.Id}
	old, ok := m.webhookRecords[key]
	if ok {
		return ErrNotFound
	}
	if record.Auth == nil {
		record.Auth = old.Auth
	}
	if auth := *record.Auth; auth != "" {
		record.AuthHash = utils.AsSha256(auth)
	}
	m.webhookRecords[key] = &record
	return nil
}

func (m *MemoryDB) UpdateEnableWebhook(user string, id uuid.UUID, enabled bool) error {
	m.webhookMux.Lock()
	defer m.webhookMux.Unlock()

	key := webhookMemKey{user, id}
	old, ok := m.webhookRecords[key]
	if ok {
		return ErrNotFound
	}
	record := *old
	record.Enabled = enabled
	m.webhookRecords[key] = &record
	return nil
}

func (m *MemoryDB) RemoveWebhook(user string, id uuid.UUID) error {
	m.webhookMux.Lock()
	defer m.webhookMux.Unlock()

	key := webhookMemKey{user, id}
	if _, ok := m.webhookRecords[key]; ok {
		return ErrNotFound
	}
	delete(m.webhookRecords, key)
	return nil
}

func (m *MemoryDB) ForEachWebhook(cb func(*WebhookRecord) error) error {
	m.webhookMux.RLock()
	defer m.webhookMux.RUnlock()

	for _, v := range m.webhookRecords {
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}

func (m *MemoryDB) ForEachUsersWebhook(user string, cb func(*WebhookRecord) error) error {
	m.webhookMux.RLock()
	defer m.webhookMux.RUnlock()

	for _, v := range m.webhookRecords {
		if v.User != user {
			continue
		}
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}

func (m *MemoryDB) ForEachEnabledWebhook(cb func(*WebhookRecord) error) error {
	m.webhookMux.RLock()
	defer m.webhookMux.RUnlock()

	for _, v := range m.webhookRecords {
		if !v.Enabled {
			continue
		}
		if err := cb(v); err != nil {
			if err == ErrStopIter {
				break
			}
			return err
		}
	}
	return nil
}
