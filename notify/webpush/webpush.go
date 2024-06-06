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

package webpush

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crow-misia/http-ece"
	"github.com/golang-jwt/jwt/v5"

	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/notify"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type Plugin struct {
	db      database.DB
	subject string
	client  *http.Client
	key     *ecdsa.PrivateKey
}

var _ notify.Plugin = (*Plugin)(nil)

func (p *Plugin) ID() string {
	return "webpush"
}

func (p *Plugin) Init(ctx context.Context, m *notify.Manager) (err error) {
	p.db = m.DB()
	p.subject = m.Subject()
	p.client = m.HTTPClient()
	keyPath := filepath.Join(m.DataDir(), "webpush.private_key")
	if p.key, err = readECPrivateKey(keyPath); err != nil {
		log.Errorf("Cannot read webpush private key: %v", err)
		if p.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			return
		}
		var blk pem.Block
		blk.Type = "EC PRIVATE KEY"
		if blk.Bytes, err = x509.MarshalECPrivateKey(p.key); err != nil {
			return
		}
		if err = os.WriteFile(keyPath, pem.EncodeToMemory(&blk), 0600); err != nil {
			return
		}
	}
	return nil
}

type Subscription struct {
	EndPoint string
	Keys     database.SubscribeRecordKeys
}

type PushOptions struct {
	Topic   string
	TTL     int
	Urgency Urgency
}

type Urgency string

const (
	UrgencyVeryLow Urgency = "very-low"
	UrgencyLow     Urgency = "low"
	UrgencyNormal  Urgency = "normal"
	UrgencyHigh    Urgency = "high"
)

func (p *Plugin) sendNotification(ctx context.Context, message []byte, s *Subscription, opts *PushOptions) (err error) {
	auth, err := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	if err != nil {
		return
	}
	p256dh, err := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if err != nil {
		return
	}
	var salt [16]byte
	if _, err = io.ReadFull(rand.Reader, salt[:]); err != nil {
		return
	}
	localKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}
	localECDHKey, err := localKey.ECDH()
	if err != nil {
		log.Panic(err)
	}
	cipher, err := httpece.Encrypt(message,
		httpece.WithEncoding(httpece.AES128GCM),
		httpece.WithAuthSecret(auth),
		httpece.WithDh(p256dh),
		httpece.WithPrivate(localECDHKey.Bytes()),
		httpece.WithSalt(salt[:]),
	)
	if err != nil {
		log.Errorf("ECE failed: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.EndPoint, bytes.NewReader(cipher))
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", build.ClusterUserAgentFull)
	req.Header.Set("TTL", strconv.Itoa(opts.TTL))
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")

	if opts.Topic != "" {
		req.Header.Set("Topic", opts.Topic)
	}
	urg := opts.Urgency
	if urg == "" {
		urg = UrgencyNormal
	}
	req.Header.Set("Urgency", (string)(urg))

	if err = p.setVAPIDAuthHeader(req); err != nil {
		return
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

func (p *Plugin) sendMessageIf(ctx context.Context, message []byte, opts *PushOptions, filter func(*database.SubscribeRecord) bool) (err error) {
	log.Debugf("Sending notification: %s", message)
	var wg sync.WaitGroup
	var mux sync.Mutex
	var outdated []database.SubscribeRecord
	err = p.db.ForEachSubscribe(func(record *database.SubscribeRecord) error {
		if filter(record) {
			log.Debugf("Sending notification to %s", record.EndPoint)
			wg.Add(1)
			go func(record database.SubscribeRecord) {
				defer wg.Done()
				subs := &Subscription{
					EndPoint: record.EndPoint,
					Keys:     record.Keys,
				}
				if err := p.sendNotification(ctx, message, subs, opts); err != nil {
					log.Warnf("Error when sending notification: %v", err)
					var herr *utils.HTTPStatusError
					if errors.As(err, &herr) && herr != nil {
						if herr.Code == http.StatusForbidden {
							mux.Lock()
							outdated = append(outdated, record)
							mux.Unlock()
						}
					}
				}
			}(*record)
		}
		return nil
	})
	wg.Wait()
	if len(outdated) > 0 {
		log.Warnf("Found %d forbidden push endpoint", len(outdated))
		for _, r := range outdated {
			p.db.RemoveSubscribe(r.User, r.Client)
		}
	}
	return
}

func readECPrivateKey(path string) (key *ecdsa.PrivateKey, err error) {
	bts, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for {
		var blk *pem.Block
		blk, bts = pem.Decode(bts)
		if blk == nil {
			break
		}
		if blk.Type == "EC PRIVATE KEY" {
			return x509.ParseECPrivateKey(blk.Bytes)
		}
	}
	return nil, errors.New(`Cannot find "EC PRIVATE KEY" in pem blocks`)
}

func (p *Plugin) GetPublicKey() []byte {
	key, err := p.key.PublicKey.ECDH()
	if err != nil {
		log.Panicf("Cannot get ecdh public key: %v", err)
	}
	return key.Bytes()
}

// the Authorization header format is depends on the encrypt algorithm:
// - aes128gcm: Authorization: "vapid t=<jwt>, k=<publicKey>"
// - aesgcm: Authorization: "WebPush <jwt>"; Crypto-Key: "p256ecdsa=<publicKey>"
func (p *Plugin) setVAPIDAuthHeader(req *http.Request) error {
	exp := time.Now().Add(time.Hour * 12)
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"aud": fmt.Sprintf("%s://%s", req.URL.Scheme, req.URL.Host),
		"exp": exp.Unix(),
		"sub": p.subject,
	})
	signed, err := token.SignedString(p.key)
	if err != nil {
		return err
	}
	pubKey := base64.RawURLEncoding.EncodeToString(p.GetPublicKey())
	switch req.Header.Get("Content-Encoding") {
	case "aes128gcm":
		req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", signed, pubKey))
	case "aesgcm":
		req.Header.Set("Authorization", "WebPush "+signed)
		req.Header.Set("Crypto-Key", "p256ecdsa="+pubKey)
	}
	return nil
}

type Map = map[string]any

func (p *Plugin) OnEnabled(e *notify.EnabledEvent) error {
	message, err := json.Marshal(Map{
		"typ": "enabled",
		"at":  e.At,
	})
	if err != nil {
		return err
	}
	opts := &PushOptions{
		Topic:   "enabled",
		TTL:     60 * 60 * 24,
		Urgency: UrgencyNormal,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.Enabled })
}

func (p *Plugin) OnDisabled(e *notify.DisabledEvent) error {
	message, err := json.Marshal(Map{
		"typ": "disabled",
		"at":  e.At.UnixMilli(),
	})
	if err != nil {
		return err
	}
	opts := &PushOptions{
		Topic:   "disabled",
		TTL:     60 * 60 * 24,
		Urgency: UrgencyHigh,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.Disabled })
}

func (p *Plugin) OnSyncBegin(e *notify.SyncBeginEvent) error {
	message, err := json.Marshal(Map{
		"typ":   "syncbegin",
		"at":    e.At.UnixMilli(),
		"count": e.Count,
		"size":  e.Size,
	})
	if err != nil {
		return err
	}
	opts := &PushOptions{
		Topic:   "syncbegin",
		TTL:     60 * 60 * 12,
		Urgency: UrgencyLow,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.SyncBegin })
}

func (p *Plugin) OnSyncDone(e *notify.SyncDoneEvent) error {
	message, err := json.Marshal(Map{
		"typ": "syncdone",
		"at":  e.At.UnixMilli(),
	})
	if err != nil {
		return err
	}
	opts := &PushOptions{
		Topic:   "syncdone",
		TTL:     60 * 60 * 12,
		Urgency: UrgencyVeryLow,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.SyncDone })
}

func (p *Plugin) OnUpdateAvaliable(e *notify.UpdateAvaliableEvent) error {
	message, err := json.Marshal(Map{
		"typ": "updates",
		"tag": e.Release.Tag.String(),
	})
	if err != nil {
		return err
	}
	opts := &PushOptions{
		Topic:   "updates",
		TTL:     60 * 60 * 24,
		Urgency: UrgencyHigh,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	return p.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.Updates })
}

func (p *Plugin) OnReportStatus(e *notify.ReportStatusEvent) (err error) {
	stat, err := json.Marshal(e.Stats)
	if err != nil {
		log.Errorf("Cannot marshal subscribe message: %v", err)
		return
	}
	var buf strings.Builder
	bw := base64.NewEncoder(base64.StdEncoding, &buf)
	zw, _ := zlib.NewWriterLevel(bw, zlib.BestCompression)
	zw.Write(stat)
	zw.Close()
	bw.Close()
	message, err := json.Marshal(Map{
		"typ":  "daily-report",
		"data": buf.String(),
	})
	if err != nil {
		return
	}
	opts := &PushOptions{
		Topic:   "daily-report",
		TTL:     60 * 60 * 12,
		Urgency: UrgencyNormal,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	now := e.At.UTC()
	var sent []database.SubscribeRecord
	err = p.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool {
		if !record.Scopes.DailyReport {
			return false
		}
		var t time.Time
		if record.LastReport.Valid {
			t = record.LastReport.Time.UTC()
		}
		if !record.ReportAt.ReadySince(t, now) {
			return false
		}
		sent = append(sent, *record)
		return true
	})
	for _, rec := range sent {
		rec.EndPoint = ""
		rec.LastReport.Valid = true
		rec.LastReport.Time = now
		p.db.SetSubscribe(rec)
	}
	return
}
