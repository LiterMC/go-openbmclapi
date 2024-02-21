/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
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
	"crypto"
	"crypto/hmac"
	"net/http"
	"time"
	"encoding/json"
	"encoding/hex"
	"bytes"
	"net/url"
)

type ClusterToken struct {
	Token string
	ExpireAt   time.Time
}

func (cr *Cluster) GetAuthToken(ctx context.Context) (token *ClusterToken, err error) {
	if cr.authToken == nil || cr.authToken.ExpireAt.Before(time.Now()) {
		if cr.authToken, err = cr.fetchToken(ctx); err != nil {
			return nil, err
		}
	}
	return cr.authToken, nil
}

func (cr *Cluster) fetchToken(ctx context.Context) (token *ClusterToken, err error) {
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
		err = NewHTTPStatusErrorFromResponse(res)
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
	res, err = cr.client.Do(req)
	if err != nil {
		return
	}
	if res.StatusCode != http.StatusOK {
		err = NewHTTPStatusErrorFromResponse(res)
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
		Token: res2.Token,
		ExpireAt: time.Now().Add((time.Duration)(res2.TTL) * time.Millisecond - time.Minute),
	}, nil
}
