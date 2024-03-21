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

package main

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
	"github.com/LiterMC/go-openbmclapi/utils"
)

type WebPushManager struct {
	dataDir  string
	database database.DB
	subject  string
	client   *http.Client

	key *ecdsa.PrivateKey

	reportMux       sync.Mutex
	nextReportAfter time.Time

	lastRelease GithubRelease
}

func NewWebPushManager(dataDir string, db database.DB, client *http.Client) (w *WebPushManager) {
	return &WebPushManager{
		dataDir:  dataDir,
		database: db,
		client:   client,
	}
}

func (w *WebPushManager) SetSubject(sub string) {
	w.subject = sub
}

func (w *WebPushManager) Init(ctx context.Context) (err error) {
	keyPath := filepath.Join(w.dataDir, "webpush.private_key")
	if w.key, err = readECPrivateKey(keyPath); err != nil {
		log.Errorf("Cannot read webpush private key: %v", err)
		if w.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			return
		}
		var blk pem.Block
		blk.Type = "EC PRIVATE KEY"
		if blk.Bytes, err = x509.MarshalECPrivateKey(w.key); err != nil {
			return
		}
		if err = os.WriteFile(keyPath, pem.EncodeToMemory(&blk), 0600); err != nil {
			return
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
	return nil, errors.New(`Cannot found "EC PRIVATE KEY" in pem blocks`)
}

func (w *WebPushManager) GetPublicKey() []byte {
	key, err := w.key.PublicKey.ECDH()
	if err != nil {
		log.Panicf("Cannot get ecdh public key: %v", err)
	}
	return key.Bytes()
}

// the Authorization header format is depends on the encrypt algorithm:
// - aes128gcm: Authorization: "vapid t=<jwt>, k=<publicKey>"
// - aesgcm: Authorization: "WebPush <jwt>"; Crypto-Key: "p256ecdsa=<publicKey>"
func (w *WebPushManager) setVAPIDAuthHeader(req *http.Request) error {
	exp := time.Now().Add(time.Hour * 12)
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"aud": fmt.Sprintf("%s://%s", req.URL.Scheme, req.URL.Host),
		"exp": exp.Unix(),
		"sub": w.subject,
	})
	signed, err := token.SignedString(w.key)
	if err != nil {
		return err
	}
	pubKey := base64.RawURLEncoding.EncodeToString(w.GetPublicKey())
	switch req.Header.Get("Content-Encoding") {
	case "aes128gcm":
		req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", signed, pubKey))
	case "aesgcm":
		req.Header.Set("Authorization", "WebPush "+signed)
		req.Header.Set("Crypto-Key", "p256ecdsa="+pubKey)
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

func (w *WebPushManager) SendNotification(ctx context.Context, message []byte, s *Subscription, opts *PushOptions) (err error) {
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

	if err = w.setVAPIDAuthHeader(req); err != nil {
		return
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode/100 != 2 {
		return utils.NewHTTPStatusErrorFromResponse(resp)
	}
	return
}

func (w *WebPushManager) sendMessageIf(ctx context.Context, message []byte, opts *PushOptions, filter func(*database.SubscribeRecord) bool) {
	log.Debugf("Sending notification: %s", message)
	var wg sync.WaitGroup
	w.database.ForEachSubscribe(func(record *database.SubscribeRecord) error {
		if filter(record) {
			log.Debugf("Sending notification to %s", record.EndPoint)
			wg.Add(1)
			go func(subs *Subscription) {
				defer wg.Done()
				err := w.SendNotification(ctx, message, subs, opts)
				if err != nil {
					log.Warnf("Error when sending notification: %v", err)
				}
			}(&Subscription{
				EndPoint: record.EndPoint,
				Keys:     record.Keys,
			})
		}
		return nil
	})
	wg.Wait()
}

func (w *WebPushManager) OnEnabled() {
	message, err := json.Marshal(Map{
		"typ": "enabled",
		"at":  time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	opts := &PushOptions{
		Topic:   "enabled",
		TTL:     60 * 60 * 24,
		Urgency: UrgencyLow,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	w.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.Enabled })
}

func (w *WebPushManager) OnDisabled() {
	message, err := json.Marshal(Map{
		"typ": "disabled",
		"at":  time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	opts := &PushOptions{
		Topic:   "disabled",
		TTL:     60 * 60 * 24,
		Urgency: UrgencyHigh,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	w.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.Disabled })
}

func (w *WebPushManager) OnSyncBegin() {
	message, err := json.Marshal(Map{
		"typ": "syncbegin",
		"at":  time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	opts := &PushOptions{
		Topic:   "syncbegin",
		TTL:     60 * 60 * 12,
		Urgency: UrgencyVeryLow,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	w.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return false /*record.Scopes.SyncBegin*/ })
}

func (w *WebPushManager) OnSyncDone() {
	message, err := json.Marshal(Map{
		"typ": "syncdone",
		"at":  time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	opts := &PushOptions{
		Topic:   "syncdone",
		TTL:     60 * 60 * 12,
		Urgency: UrgencyLow,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	w.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.SyncDone })
}

func (w *WebPushManager) OnUpdateAvaliable(release *GithubRelease) {
	if w.lastRelease == *release {
		return
	}
	w.lastRelease = *release

	message, err := json.Marshal(Map{
		"typ": "updates",
		"tag": release.Tag.String(),
	})
	if err != nil {
		log.Errorf("Cannot marshal subscribe message: %v", err)
		return
	}
	opts := &PushOptions{
		Topic:   "updates",
		TTL:     60 * 60 * 24,
		Urgency: UrgencyHigh,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	w.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.Updates })
}

func (w *WebPushManager) OnReportStat(stats *Stats) {
	if !w.reportMux.TryLock() {
		return
	}
	defer w.reportMux.Unlock()

	now := time.Now()
	if !w.nextReportAfter.IsZero() && now.Before(w.nextReportAfter) {
		return
	}
	w.nextReportAfter = now.Add(time.Minute * 5).Truncate(15 * time.Minute)

	stat, err := stats.MarshalJSON()
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
		log.Errorf("Cannot marshal subscribe message: %v", err)
		return
	}
	opts := &PushOptions{
		Topic:   "daily-report",
		TTL:     60 * 60 * 12,
		Urgency: UrgencyNormal,
	}

	tctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	w.sendMessageIf(tctx, message, opts, func(record *database.SubscribeRecord) bool { return record.Scopes.DailyReport })
}
