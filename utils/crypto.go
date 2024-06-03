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

package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"crypto/hmac"
)

func ComparePasswd(p1, p2 string) bool {
	a := sha256.Sum256(([]byte)(p1))
	b := sha256.Sum256(([]byte)(p2))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 0
}

func BytesAsSha256(b []byte) string {
	buf := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

// return a URL encoded base64 string
func AsSha256(s string) string {
	buf := sha256.Sum256(([]byte)(s))
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

func AsSha256Hex(s string) string {
	buf := sha256.Sum256(([]byte)(s))
	return hex.EncodeToString(buf[:])
}

func HMACSha256Hex(key, data string) string {
	m := hmac.New(sha256.New, ([]byte)(key))
	m.Write(([]byte)(data))
	buf := m.Sum(nil)
	return hex.EncodeToString(buf[:])
}

func GenRandB64(n int) (s string, err error) {
	buf := make([]byte, n)
	if _, err = io.ReadFull(rand.Reader, buf); err != nil {
		return
	}
	s = base64.RawURLEncoding.EncodeToString(buf)
	return
}

func LoadOrCreateHmacKey(dataDir string) (key []byte, err error) {
	path := filepath.Join(dataDir, "server.hmac.private_key")
	buf, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return
		}
		var sbuf string
		if sbuf, err = GenRandB64(256); err != nil {
			return
		}
		buf = ([]byte)(sbuf)
		if err = os.WriteFile(path, buf, 0600); err != nil {
			return
		}
	}
	key = make([]byte, base64.RawURLEncoding.DecodedLen(len(buf)))
	if _, err = base64.RawURLEncoding.Decode(key, buf); err != nil {
		return
	}
	return
}
