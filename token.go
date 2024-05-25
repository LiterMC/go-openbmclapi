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
	"context"
	"crypto"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
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
		"clusterId": {cr.clusterId},
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
	hs := hmac.New(crypto.SHA256.New, ([]byte)(cr.clusterSecret))
	hs.Write(([]byte)(res1.Challenge))
	signature := hex.EncodeToString(hs.Sum(buf[:0]))

	payload, err := json.Marshal(struct {
		ClusterId string `json:"clusterId"`
		Challenge string `json:"challenge"`
		Signature string `json:"signature"`
	}{
		ClusterId: cr.clusterId,
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
		ClusterId: cr.clusterId,
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
