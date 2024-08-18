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

package cluster

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type ClusterToken struct {
	Token    string
	ExpireAt time.Time
}

func (cr *Cluster) GetAuthToken(ctx context.Context) (token string, err error) {
	cr.authTokenMux.RLock()
	expired := cr.authToken == nil || cr.authToken.ExpireAt.Before(time.Now())
	if !expired {
		token = cr.authToken.Token
	}
	almostExpired := !expired && cr.authToken.ExpireAt.Add(-10*time.Minute).Before(time.Now())
	cr.authTokenMux.RUnlock()
	if expired {
		cr.authTokenMux.Lock()
		defer cr.authTokenMux.Unlock()
		if cr.authToken == nil || cr.authToken.ExpireAt.Before(time.Now()) {
			if cr.authToken, err = cr.fetchToken(ctx); err != nil {
				return "", err
			}
		}
		token = cr.authToken.Token
	} else if almostExpired {
		go func() {
			tctx, cancel := context.WithTimeout(ctx, time.Second*30)
			defer cancel()
			cr.authTokenMux.Lock()
			defer cr.authTokenMux.Unlock()
			if cr.authToken != nil && cr.authToken.ExpireAt.Add(-10*time.Minute).Before(time.Now()) {
				tk, err := cr.refreshToken(tctx, cr.authToken.Token)
				if err != nil {
					log.Errorf("Cannot refresh token: %v", err)
					return
				}
				cr.authToken = tk
			}
		}()
	}
	return
}

func (cr *Cluster) fetchToken(ctx context.Context) (token *ClusterToken, err error) {
	log.Infof("Fetching authorization token ...")
	defer func() {
		if err != nil {
			log.Errorf("Cannot fetch authorization token: %v", err)
		} else {
			log.Infof("Authorization token fetched")
		}
	}()
	req, err := cr.makeReq(ctx, http.MethodGet, "/openbmclapi-agent/challenge", url.Values{
		"clusterId": {cr.ID()},
	})
	if err != nil {
		return
	}
	res, err := cr.client.Do(req)
	if err != nil {
		return
	}
	if res.StatusCode != http.StatusOK {
		err = utils.NewHTTPStatusErrorFromResponse(res)
		res.Body.Close()
		return
	}
	var res1 struct {
		Challenge string `json:"challenge"`
	}
	err = json.NewDecoder(res.Body).Decode(&res1)
	res.Body.Close()
	if err != nil {
		return
	}

	var buf [32]byte
	hs := hmac.New(crypto.SHA256.New, ([]byte)(cr.Secret()))
	hs.Write(([]byte)(res1.Challenge))
	signature := hex.EncodeToString(hs.Sum(buf[:0]))

	payload, err := json.Marshal(struct {
		ClusterId string `json:"clusterId"`
		Challenge string `json:"challenge"`
		Signature string `json:"signature"`
	}{
		ClusterId: cr.ID(),
		Challenge: res1.Challenge,
		Signature: signature,
	})

	req, err = cr.makeReqWithBody(ctx, http.MethodPost, "/openbmclapi-agent/token", nil, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	res, err = cr.client.Do(req)
	if err != nil {
		return
	}
	if res.StatusCode/100 != 2 {
		err = utils.NewHTTPStatusErrorFromResponse(res)
		res.Body.Close()
		return
	}
	var res2 struct {
		Token string `json:"token"`
		TTL   int64  `json:"ttl"`
	}
	err = json.NewDecoder(res.Body).Decode(&res2)
	res.Body.Close()
	if err != nil {
		return
	}

	return &ClusterToken{
		Token:    res2.Token,
		ExpireAt: time.Now().Add((time.Duration)(res2.TTL)*time.Millisecond - 10*time.Second),
	}, nil
}

func (cr *Cluster) refreshToken(ctx context.Context, oldToken string) (token *ClusterToken, err error) {
	payload, err := json.Marshal(struct {
		ClusterId string `json:"clusterId"`
		Token     string `json:"token"`
	}{
		ClusterId: cr.ID(),
		Token:     oldToken,
	})
	if err != nil {
		return
	}

	req, err := cr.makeReqWithBody(ctx, http.MethodPost, "/openbmclapi-agent/token", nil, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := cr.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = utils.NewHTTPStatusErrorFromResponse(resp)
		return
	}
	var res struct {
		Token string `json:"token"`
		TTL   int64  `json:"ttl"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return
	}

	return &ClusterToken{
		Token:    res.Token,
		ExpireAt: time.Now().Add((time.Duration)(res.TTL)*time.Millisecond - 10*time.Second),
	}, nil
}

type OpenbmclapiAgentConfig struct {
	Sync OpenbmclapiAgentSyncConfig `json:"sync"`
}

type OpenbmclapiAgentSyncConfig struct {
	Source      string `json:"source"`
	Concurrency int    `json:"concurrency"`
}

func (cr *Cluster) GetConfig(ctx context.Context) (cfg *OpenbmclapiAgentConfig, err error) {
	req, err := cr.makeReqWithAuth(ctx, http.MethodGet, "/openbmclapi/configuration", nil)
	if err != nil {
		return
	}
	res, err := cr.client.DoUseCache(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		err = utils.NewHTTPStatusErrorFromResponse(res)
		return
	}
	cfg = new(OpenbmclapiAgentConfig)
	if err = json.NewDecoder(res.Body).Decode(cfg); err != nil {
		cfg = nil
		return
	}
	return
}

type CertKeyPair struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func (cr *Cluster) RequestCert(ctx context.Context) (ckp *CertKeyPair, err error) {
	resCh, err := cr.socket.EmitWithAck("request-cert")
	if err != nil {
		return
	}
	var data []any
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case data = <-resCh:
	}
	if ero := data[0]; ero != nil {
		err = fmt.Errorf("socket.io remote error: %v", ero)
		return
	}
	pair := data[1].(map[string]any)
	ckp = new(CertKeyPair)
	var ok bool
	if ckp.Cert, ok = pair["cert"].(string); !ok {
		err = fmt.Errorf(`"cert" is not a string, got %T`, pair["cert"])
		return
	}
	if ckp.Key, ok = pair["key"].(string); !ok {
		err = fmt.Errorf(`"key" is not a string, got %T`, pair["key"])
		return
	}
	return
}

func (cr *Cluster) ReportDownload(ctx context.Context, response *http.Response, err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}

	type ReportPayload struct {
		Urls  []string `json:"urls"`
		Error utils.EmbedJSON[struct {
			Message string `json:"message"`
		}] `json:"error"`
	}
	var payload ReportPayload
	redirects := utils.GetRedirects(response)
	payload.Urls = make([]string, len(redirects))
	for i, u := range redirects {
		payload.Urls[i] = u.String()
	}
	payload.Error.V.Message = err.Error()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := cr.makeReqWithAuthBody(ctx, http.MethodPost, "/openbmclapi/report", nil, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cr.client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return utils.NewHTTPStatusErrorFromResponse(resp)
	}
	return nil
}
