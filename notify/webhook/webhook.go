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

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/cluster"
	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/notify"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type Plugin struct {
	db     database.DB
	client *http.Client
}

var _ notify.Plugin = (*Plugin)(nil)

func (p *Plugin) ID() string {
	return "webhook"
}

func (p *Plugin) Init(ctx context.Context, m *notify.Manager) (err error) {
	p.db = m.DB()
	p.client = m.HTTPClient()
	return nil
}

func (p *Plugin) sendMessage(ctx context.Context, message []byte, r *api.WebhookRecord) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.EndPoint, bytes.NewReader(message))
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", build.ClusterUserAgentFull)
	req.Header.Set("Content-Type", "application/json")
	if r.Auth != nil {
		req.Header.Set("Authorization", *r.Auth)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode/100 != 2 {
		return utils.NewHTTPStatusErrorFromResponse(resp)
	}
	return
}

func (p *Plugin) sendMessageIf(ctx context.Context, msg Message, filter func(*api.WebhookRecord) bool) (err error) {
	message, err := json.Marshal(msg)
	if err != nil {
		return
	}
	log.Debugf("Triggering webhook: %s", message)
	var wg sync.WaitGroup
	err = p.db.ForEachWebhook(func(record *api.WebhookRecord) error {
		if filter(record) {
			log.Debugf("Triggering webhook at %s", record.EndPoint)
			wg.Add(1)
			go func(record api.WebhookRecord) {
				defer wg.Done()
				if err := p.sendMessage(ctx, message, &record); err != nil {
					log.Warnf("Error when triggering webhook: %v", err)
				}
			}(*record)
		}
		return nil
	})
	wg.Wait()
	return
}

type MessageType string

const (
	TypeEnabled     MessageType = "enabled"
	TypeDisabled    MessageType = "disabled"
	TypeSyncBegin   MessageType = "syncbegin"
	TypeSyncDone    MessageType = "syncdone"
	TypeUpdates     MessageType = "updates"
	TypeDailyReport MessageType = "daily-report"
)

type (
	Message struct {
		Type MessageType `json:"type"`
		Data any         `json:"data"`
	}

	EnabledData struct {
		At time.Time `json:"at"`
	}

	DisabledData struct {
		At time.Time `json:"at"`
	}

	SyncBeginData struct {
		At    time.Time `json:"at"`
		Count int       `json:"count"`
		Size  int64     `json:"size"`
	}

	SyncDoneData struct {
		At time.Time `json:"at"`
	}

	UpdatesData struct {
		Tag string `json:"tag"`
	}

	DailyReportData struct {
		Stats *cluster.StatManager `json:"stats"`
	}
)

func (p *Plugin) OnEnabled(e *notify.EnabledEvent) error {
	message := Message{
		Type: TypeEnabled,
		Data: EnabledData{
			At: e.At,
		},
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, func(record *api.WebhookRecord) bool { return record.Scopes.Enabled })
}

func (p *Plugin) OnDisabled(e *notify.DisabledEvent) error {
	message := Message{
		Type: TypeDisabled,
		Data: DisabledData{
			At: e.At,
		},
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, func(record *api.WebhookRecord) bool { return record.Scopes.Disabled })
}

func (p *Plugin) OnSyncBegin(e *notify.SyncBeginEvent) error {
	message := Message{
		Type: TypeSyncBegin,
		Data: SyncBeginData{
			At:    e.At,
			Count: e.Count,
			Size:  e.Size,
		},
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, func(record *api.WebhookRecord) bool { return record.Scopes.SyncBegin })
}

func (p *Plugin) OnSyncDone(e *notify.SyncDoneEvent) error {
	message := Message{
		Type: TypeSyncDone,
		Data: SyncDoneData{
			At: e.At,
		},
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, func(record *api.WebhookRecord) bool { return record.Scopes.SyncDone })
}

func (p *Plugin) OnUpdateAvaliable(e *notify.UpdateAvaliableEvent) error {
	message := Message{
		Type: TypeUpdates,
		Data: UpdatesData{
			Tag: e.Release.Tag.String(),
		},
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, func(record *api.WebhookRecord) bool { return record.Scopes.Updates })
}

func (p *Plugin) OnReportStatus(e *notify.ReportStatusEvent) (err error) {
	message := Message{
		Type: TypeDailyReport,
		Data: DailyReportData{
			Stats: e.Stats,
		},
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, func(record *api.WebhookRecord) bool { return record.Scopes.DailyReport })
}
