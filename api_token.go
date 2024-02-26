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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const jwtIssuerPrefix = "GOBA.dash.api"

const (
	tokenTypeKey = "go-openbmclapi.cluster.token.typ"
	tokenIdKey   = "go-openbmclapi.cluster.token.id"
)

const (
	tokenTypeAuth = "auth"
	tokenTypeAPI  = "api"
)

func getRequestTokenType(req *http.Request) string {
	return req.Context().Value(tokenTypeKey).(string)
}

var (
	ErrUnsupportAuthType = errors.New("unsupported authorization type")
	ErrClientIdNotMatch  = errors.New("client id not match")
	ErrJTINotExists      = errors.New("jti not exists")

	ErrStrictPathNotMatch  = errors.New("strict path not match")
	ErrStrictQueryNotMatch = errors.New("strict query value not match")
)

type TokenStorage struct {
	mux       sync.RWMutex
	tokens    map[string]time.Time
	nextCheck time.Time
	checking  bool
}

func NewTokenStorage() *TokenStorage {
	return &TokenStorage{
		tokens:    make(map[string]time.Time),
		nextCheck: time.Now().Add(time.Minute * 5),
	}
}

func (s *TokenStorage) checker() {
	s.mux.RLock()
	var expired []string
	now := time.Now()
	for k, v := range s.tokens {
		if now.After(v) {
			expired = append(expired, k)
		}
	}
	s.mux.RUnlock()
	s.mux.Lock()
	for _, k := range expired {
		delete(s.tokens, k)
	}
	s.checking = false
	s.nextCheck = time.Now().Add(time.Minute * 5)
	s.mux.Unlock()
}

func (s *TokenStorage) tryCheckLocked() {
	if !s.checking && time.Now().After(s.nextCheck) {
		s.checking = true
		go s.checker()
	}
}

func (s *TokenStorage) tryCheck() {
	s.mux.RLock()
	ok := !s.checking && time.Now().After(s.nextCheck)
	s.mux.RUnlock()
	if ok {
		s.mux.Lock()
		s.tryCheckLocked()
		s.mux.Unlock()
	}
}

func (s *TokenStorage) Register(id string, expire time.Time) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.tokens[id] = expire
	if len(s.tokens) > 256 {
		s.tryCheckLocked()
	}
}

func (s *TokenStorage) Unregister(id string) {
	s.mux.Lock()
	defer s.mux.Unlock()
	delete(s.tokens, id)
}

func (s *TokenStorage) Check(id string) bool {
	s.mux.RLock()
	defer s.mux.RUnlock()
	expire, ok := s.tokens[id]
	if !ok {
		return false
	}
	if time.Now().After(expire) {
		return false
	}
	return true
}

func (cr *Cluster) getJWTKey(t *jwt.Token) (any, error) {
	if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("Unexpected signing method: %v", t.Header["alg"])
	}
	return cr.apiHmacKey, nil
}

const (
	authTokenSubject = "GOBA-auth"
	apiTokenSubject  = "GOBA-API"
)

type authTokenClaims struct {
	jwt.RegisteredClaims

	Client string `json:"cli"`
}

func (cr *Cluster) generateAuthToken(cliId string) (string, error) {
	jti, err := genRandB64(16)
	if err != nil {
		return "", err
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &authTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   authTokenSubject,
			Issuer:    cr.jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour * 24)),
		},
		Client: cliId,
	})
	tokenStr, err := token.SignedString(cr.apiHmacKey)
	if err != nil {
		return "", err
	}
	// registerTokenId(jti) // ignore JTI for auth token; TODO
	return tokenStr, nil
}

func (cr *Cluster) verifyAuthToken(cliId string, token string) (id string, err error) {
	var claims authTokenClaims
	_, err = jwt.ParseWithClaims(
		token,
		&claims,
		cr.getJWTKey,
		jwt.WithSubject(authTokenSubject),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(cr.jwtIssuer),
	)
	if err != nil {
		return
	}
	if claims.Client != cliId {
		return "", ErrClientIdNotMatch
	}
	jti := claims.ID
	// if !checkTokenId(jti) { // ignore JTI here; TODO
	// 	logDebugf("Cannot verity auth token: jti not exists")
	// 	return ""
	// }
	return jti, nil
}

type apiTokenClaims struct {
	jwt.RegisteredClaims

	Client      string            `json:"cli"`
	StrictPath  string            `json:"str-p"`
	StrictQuery map[string]string `json:"str-q,omitempty"`
}

func (cr *Cluster) generateAPIToken(cliId string, path string, query map[string]string) (string, error) {
	jti, err := genRandB64(8)
	if err != nil {
		return "", err
	}
	now := time.Now()
	exp := now.Add(time.Minute * 10)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &apiTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   apiTokenSubject,
			Issuer:    cr.jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Client:      cliId,
		StrictPath:  path,
		StrictQuery: query,
	})
	tokenStr, err := token.SignedString(cr.apiHmacKey)
	if err != nil {
		return "", err
	}
	cr.tokens.Register(jti, exp)
	return tokenStr, nil
}

func (cr *Cluster) verifyAPIToken(cliId string, token string, path string, query url.Values) (id string, err error) {
	var claims apiTokenClaims
	_, err = jwt.ParseWithClaims(
		token,
		&claims,
		cr.getJWTKey,
		jwt.WithSubject(apiTokenSubject),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(cr.jwtIssuer),
	)
	if err != nil {
		return
	}
	if claims.Client != cliId {
		return "", ErrClientIdNotMatch
	}
	jti := claims.ID
	if !cr.tokens.Check(jti) {
		return "", ErrJTINotExists
	}
	if claims.StrictPath != path {
		return "", ErrStrictPathNotMatch
	}
	for k, v := range claims.StrictQuery {
		if query.Get(k) != v {
			return "", ErrStrictQueryNotMatch
		}
	}
	return jti, nil
}
