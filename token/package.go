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

package token

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/LiterMC/go-openbmclapi/utils"
)

var (
	ErrUnsupportAuthType = errors.New("unsupported authorization type")
	ErrScopeNotMatch     = errors.New("scope not match")
	ErrJTINotExists      = errors.New("jti not exists")

	ErrStrictPathNotMatch  = errors.New("strict path not match")
	ErrStrictQueryNotMatch = errors.New("strict query value not match")
)

const (
	challengeTokenScope = "GOBA-challenge"
	authTokenScope      = "GOBA-auth"
	apiTokenScope       = "GOBA-API"
)

type (
	basicTokenManager struct {
		impl basicTokenManagerImpl
	}
	basicTokenManagerImpl interface {
		Issuer() string
		HmacKey() []byte
		AddJTI(string, time.Time) error
		ValidJTI(string) bool
	}
)

type (
	challengeTokenClaims struct {
		jwt.RegisteredClaims

		Scope  string `json:"scope"`
		Action string `json:"act"`
	}

	authTokenClaims struct {
		jwt.RegisteredClaims

		Scope string `json:"scope"`
		User  string `json:"usr"`
	}

	apiTokenClaims struct {
		jwt.RegisteredClaims

		Scope       string            `json:"scope"`
		User        string            `json:"usr"`
		StrictPath  string            `json:"str-p"`
		StrictQuery map[string]string `json:"str-q,omitempty"`
	}
)

func (m *basicTokenManager) getJWTKey(t *jwt.Token) (any, error) {
	if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("Unexpected signing method: %v", t.Header["alg"])
	}
	return m.impl.HmacKey(), nil
}

func (m *basicTokenManager) GenerateChallengeToken(cliId string, action string) (string, error) {
	now := time.Now()
	exp := now.Add(time.Minute * 1)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &challengeTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   cliId,
			Issuer:    m.impl.Issuer(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Scope:  challengeTokenScope,
		Action: action,
	})
	tokenStr, err := token.SignedString(m.impl.HmacKey())
	if err != nil {
		return "", err
	}
	return tokenStr, nil
}

func (m *basicTokenManager) VerifyChallengeToken(cliId string, action string, token string) (err error) {
	var claims challengeTokenClaims
	if _, err = jwt.ParseWithClaims(
		token,
		&claims,
		m.getJWTKey,
		jwt.WithSubject(cliId),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(m.impl.Issuer()),
	); err != nil {
		return
	}
	if claims.Scope != challengeTokenScope {
		return ErrScopeNotMatch
	}
	if claims.Action != action {
		return ErrJTINotExists
	}
	return
}

func (m *basicTokenManager) GenerateAuthToken(cliId string, userId string) (string, error) {
	jti, err := utils.GenRandB64(16)
	if err != nil {
		return "", err
	}
	now := time.Now()
	exp := now.Add(time.Hour * 24)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &authTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   cliId,
			Issuer:    m.impl.Issuer(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Scope: authTokenScope,
		User:  userId,
	})
	tokenStr, err := token.SignedString(m.impl.HmacKey())
	if err != nil {
		return "", err
	}
	if err = m.impl.AddJTI(jti, exp); err != nil {
		return "", err
	}
	return tokenStr, nil
}

func (m *basicTokenManager) VerifyAuthToken(cliId string, token string) (id string, user string, err error) {
	var claims authTokenClaims
	if _, err = jwt.ParseWithClaims(
		token,
		&claims,
		m.getJWTKey,
		jwt.WithSubject(cliId),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(m.impl.Issuer()),
	); err != nil {
		return
	}
	if claims.Scope != authTokenScope {
		err = ErrScopeNotMatch
		return
	}
	if user = claims.User; user == "" {
		// reject old token
		err = ErrJTINotExists
		return
	}
	id = claims.ID
	if ok := m.impl.ValidJTI(id); !ok {
		err = ErrJTINotExists
		return
	}
	return
}

func (m *basicTokenManager) GenerateAPIToken(cliId string, userId string, path string, query map[string]string) (string, error) {
	jti, err := utils.GenRandB64(8)
	if err != nil {
		return "", err
	}
	now := time.Now()
	exp := now.Add(time.Minute * 10)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &apiTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   cliId,
			Issuer:    m.impl.Issuer(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Scope:       apiTokenScope,
		User:        userId,
		StrictPath:  path,
		StrictQuery: query,
	})
	tokenStr, err := token.SignedString(m.impl.HmacKey())
	if err != nil {
		return "", err
	}
	if err = m.impl.AddJTI(jti, exp); err != nil {
		return "", err
	}
	return tokenStr, nil
}

func (m *basicTokenManager) VerifyAPIToken(cliId string, token string, path string, query url.Values) (user string, err error) {
	var claims apiTokenClaims
	_, err = jwt.ParseWithClaims(
		token,
		&claims,
		m.getJWTKey,
		jwt.WithSubject(cliId),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(m.impl.Issuer()),
	)
	if err != nil {
		return
	}
	if claims.Scope != apiTokenScope {
		err = ErrScopeNotMatch
		return
	}
	if user = claims.User; user == "" {
		err = ErrJTINotExists
		return
	}
	if ok := m.impl.ValidJTI(claims.ID); !ok {
		err = ErrJTINotExists
		return
	}
	if claims.StrictPath != path {
		err = ErrStrictPathNotMatch
		return
	}
	for k, v := range claims.StrictQuery {
		if query.Get(k) != v {
			err = ErrStrictQueryNotMatch
			return
		}
	}
	return
}
