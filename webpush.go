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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/log"
)

type WebPushManager struct {
	dataDir  string
	database database.DB

	key *ecdsa.PrivateKey

	reportMux       sync.Mutex
	nextReportAfter time.Time

	lastRelease GithubRelease
}

func NewWebPushManager(dataDir string, db database.DB) (w *WebPushManager) {
	return &WebPushManager{
		dataDir:  dataDir,
		database: db,
	}
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

func (w *WebPushManager) OnEnabled() {
	//
}

func (w *WebPushManager) OnDisabled() {
	//
}

func (w *WebPushManager) OnSyncBegin() {
	//
}

func (w *WebPushManager) OnSyncDone() {
	//
}

func (w *WebPushManager) OnUpdateAvaliable(release *GithubRelease) {
	if w.lastRelease == *release {
		return
	}
	w.lastRelease = *release

	w.database.ForEachSubscribe(func(record *database.SubscribeRecord) error {
		// record.EndPoint
		// record.Scopes.Updates
		return nil
	})
}

func (w *WebPushManager) OnReportStat(stats *Stats) {
	if !w.reportMux.TryLock() {
		return
	}
	defer w.reportMux.Unlock()

	now := time.Now()
	if w.nextReportAfter.IsZero() || now.Before(w.nextReportAfter) {
		return
	}
	w.nextReportAfter = now.Add(time.Minute * 5).Truncate(15 * time.Minute)

	// TOOD: daily report
	// w.database.ForEachSubscribe(func(record *database.SubscribeRecord) error {
	// 	record.EndPoint
	// 	record.Scopes.
	// })
}
