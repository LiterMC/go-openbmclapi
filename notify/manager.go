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

package notify

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/update"
)

type Plugin interface {
	ID() string
	Init(ctx context.Context, m *Manager) error
	OnEnabled(*EnabledEvent) error
	OnDisabled(*DisabledEvent) error
	OnSyncBegin(*SyncBeginEvent) error
	OnSyncDone(*SyncDoneEvent) error
	OnUpdateAvaliable(*UpdateAvaliableEvent) error
	OnReportStatus(*ReportStatusEvent) error
}

type Manager struct {
	dataDir  string
	database database.DB
	subject  string
	client   *http.Client

	reportMux       sync.Mutex
	nextReportAfter time.Time
	lastRelease     update.GithubRelease

	plugins []Plugin
}

func NewManager(dataDir string, db database.DB, client *http.Client, subject string) *Manager {
	return &Manager{
		dataDir:  dataDir,
		database: db,
		subject:  subject,
		client:   client,
	}
}

func (m *Manager) DataDir() string {
	return m.dataDir
}

func (m *Manager) DB() database.DB {
	return m.database
}

func (m *Manager) HTTPClient() *http.Client {
	return m.client
}

func (m *Manager) Subject() string {
	return m.subject
}

func (m *Manager) AddPlugin(p Plugin) {
	m.plugins = append(m.plugins, p)
}

func (m *Manager) Init(ctx context.Context) (err error) {
	for _, p := range m.plugins {
		if err = p.Init(ctx, m); err != nil {
			return
		}
	}
	return
}

func (m *Manager) OnEnabled() {
	e := &EnabledEvent{
		At: time.Now(),
	}
	res := make(chan error, 0)
	for _, p := range m.plugins {
		go func(p Plugin) {
			defer log.RecoverPanic()
			res <- p.OnEnabled(e)
		}(p)
	}
	for i := len(m.plugins); i > 0; i-- {
		err := <-res
		if err != nil {
			log.Errorf("Cannot send notification: %v", err)
		}
	}
}
func (m *Manager) OnDisabled() {
	e := &DisabledEvent{
		At: time.Now(),
	}
	res := make(chan error, 0)
	for _, p := range m.plugins {
		go func(p Plugin) {
			defer log.RecoverPanic()
			res <- p.OnDisabled(e)
		}(p)
	}
	for i := len(m.plugins); i > 0; i-- {
		err := <-res
		if err != nil {
			log.Errorf("Cannot send notification: %v", err)
		}
	}
}
func (m *Manager) OnSyncBegin(count int, size int64) {
	e := &SyncBeginEvent{
		TimestampEvent: TimestampEvent{
			At: time.Now(),
		},
		Count: count,
		Size:  size,
	}
	res := make(chan error, 0)
	for _, p := range m.plugins {
		go func(p Plugin) {
			defer log.RecoverPanic()
			res <- p.OnSyncBegin(e)
		}(p)
	}
	for i := len(m.plugins); i > 0; i-- {
		err := <-res
		if err != nil {
			log.Errorf("Cannot send notification: %v", err)
		}
	}
}
func (m *Manager) OnSyncDone() {
	e := &SyncDoneEvent{
		At: time.Now(),
	}
	res := make(chan error, 0)
	for _, p := range m.plugins {
		go func(p Plugin) {
			defer log.RecoverPanic()
			res <- p.OnSyncDone(e)
		}(p)
	}
	for i := len(m.plugins); i > 0; i-- {
		err := <-res
		if err != nil {
			log.Errorf("Cannot send notification: %v", err)
		}
	}
}
func (m *Manager) OnUpdateAvaliable(release *update.GithubRelease) {
	if m.lastRelease == *release {
		return
	}
	m.lastRelease = *release

	e := &UpdateAvaliableEvent{
		Release: release,
	}
	res := make(chan error, 0)
	for _, p := range m.plugins {
		go func(p Plugin) {
			defer log.RecoverPanic()
			res <- p.OnUpdateAvaliable(e)
		}(p)
	}
	for i := len(m.plugins); i > 0; i-- {
		err := <-res
		if err != nil {
			log.Errorf("Cannot send notification: %v", err)
		}
	}
}

func (m *Manager) OnReportStatus(stats *Stats) {
	if !m.reportMux.TryLock() {
		return
	}
	defer m.reportMux.Unlock()

	now := time.Now()
	if !m.nextReportAfter.IsZero() && now.Before(m.nextReportAfter) {
		return
	}
	m.nextReportAfter = now.Add(time.Minute * 8).Truncate(15 * time.Minute).Add(30 * time.Minute)

	e := &ReportStatusEvent{
		TimestampEvent: TimestampEvent{
			At: now,
		},
		Stats: stats.Clone(),
	}
	res := make(chan error, 0)
	for _, p := range m.plugins {
		go func(p Plugin) {
			defer log.RecoverPanic()
			res <- p.OnReportStatus(e)
		}(p)
	}
	for i := len(m.plugins); i > 0; i-- {
		err := <-res
		if err != nil {
			log.Errorf("Cannot send notification: %v", err)
		}
	}
}
