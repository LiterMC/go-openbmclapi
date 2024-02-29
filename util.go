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
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var closedCh = func() <-chan struct{} {
	ch := make(chan struct{}, 0)
	close(ch)
	return ch
}()

func split(str string, b byte) (l, r string) {
	i := strings.IndexByte(str, b)
	if i >= 0 {
		return str[:i], str[i+1:]
	}
	return str, ""
}

func createInterval(ctx context.Context, do func(), delay time.Duration) {
	go func() {
		ticker := time.NewTicker(delay)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				do()
				// If another a tick passed during the job, ignore it
				select {
				case <-ticker.C:
				default:
				}
			}
		}
	}()
	return
}

func httpToWs(origin string) string {
	if strings.HasPrefix(origin, "http") {
		return "ws" + origin[4:]
	}
	return origin
}

const byteUnits = "KMGTPE"

func bytesToUnit(size float64) string {
	if size < 1000 {
		return fmt.Sprintf("%dB", (int)(size))
	}
	var unit rune
	for _, u := range byteUnits {
		unit = u
		size /= 1024
		if size < 1000 {
			break
		}
	}
	return fmt.Sprintf("%.1f%sB", size, string(unit))
}

func getHashMethod(l int) (hashMethod crypto.Hash, err error) {
	switch l {
	case 32:
		hashMethod = crypto.MD5
	case 40:
		hashMethod = crypto.SHA1
	default:
		err = fmt.Errorf("Unknown hash length %d", l)
	}
	return
}

func parseCertCommonName(body []byte) (string, error) {
	cert, err := x509.ParseCertificate(body)
	if err != nil {
		return "", err
	}
	return cert.Subject.CommonName, nil
}

var rd = func() chan int {
	ch := make(chan int, 64)
	r := rand.New(rand.NewSource(time.Now().Unix()))
	go func() {
		for {
			ch <- r.Int()
		}
	}()
	return ch
}()

func randIntn(n int) int {
	rn := <-rd
	return rn % n
}

func forEachFromRandomIndex(leng int, cb func(i int) (done bool)) (done bool) {
	if leng <= 0 {
		return false
	}
	start := randIntn(leng)
	for i := start; i < leng; i++ {
		if cb(i) {
			return true
		}
	}
	for i := 0; i < start; i++ {
		if cb(i) {
			return true
		}
	}
	return false
}

func forEachFromRandomIndexWithPossibility(poss []uint, total uint, cb func(i int) (done bool)) (done bool) {
	leng := len(poss)
	if leng == 0 {
		return false
	}
	if total == 0 {
		return forEachFromRandomIndex(leng, cb)
	}
	n := (uint)(randIntn((int)(total)))
	start := 0
	for i, p := range poss {
		if n < p {
			start = i
			break
		}
		n -= p
	}
	for i := start; i < leng; i++ {
		if cb(i) {
			return true
		}
	}
	for i := 0; i < start; i++ {
		if cb(i) {
			return true
		}
	}
	return false
}

func copyFile(src, dst string, mode os.FileMode) (err error) {
	var srcFd, dstFd *os.File
	if srcFd, err = os.Open(src); err != nil {
		return
	}
	defer srcFd.Close()
	if dstFd, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode); err != nil {
		return
	}
	defer dstFd.Close()
	_, err = io.Copy(dstFd, srcFd)
	return
}

func checkQuerySign(hash string, secret string, query url.Values) bool {
	if config.Advanced.SkipSignatureCheck {
		return true
	}
	sign, e := query.Get("s"), query.Get("e")
	if len(sign) == 0 || len(e) == 0 {
		return false
	}
	before, err := strconv.ParseInt(e, 36, 64)
	if err != nil {
		return false
	}
	hs := crypto.SHA1.New()
	io.WriteString(hs, secret)
	io.WriteString(hs, hash)
	io.WriteString(hs, e)
	var (
		buf  [20]byte
		sbuf [27]byte
	)
	base64.RawURLEncoding.Encode(sbuf[:], hs.Sum(buf[:0]))
	if (string)(sbuf[:]) != sign {
		return false
	}
	return time.Now().UnixMilli() < before
}

type SyncMap[K comparable, V any] struct {
	l sync.RWMutex
	m map[K]V
}

func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: make(map[K]V),
	}
}

func (m *SyncMap[K, V]) Len() int {
	m.l.RLock()
	defer m.l.RUnlock()
	return len(m.m)
}

func (m *SyncMap[K, V]) Set(k K, v V) {
	m.l.Lock()
	defer m.l.Unlock()
	m.m[k] = v
}

func (m *SyncMap[K, V]) Get(k K) V {
	m.l.RLock()
	defer m.l.RUnlock()
	return m.m[k]
}

func (m *SyncMap[K, V]) Has(k K) bool {
	m.l.RLock()
	defer m.l.RUnlock()
	_, ok := m.m[k]
	return ok
}

func (m *SyncMap[K, V]) GetOrSet(k K, setter func() V) (v V, has bool) {
	m.l.RLock()
	v, has = m.m[k]
	m.l.RUnlock()
	if has {
		return
	}
	m.l.Lock()
	defer m.l.Unlock()
	v, has = m.m[k]
	if !has {
		v = setter()
		m.m[k] = v
	}
	return
}

func comparePasswd(p1, p2 string) bool {
	a := sha256.Sum256(([]byte)(p1))
	b := sha256.Sum256(([]byte)(p2))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 0
}

// return a URL encoded base64 string
func asSha256(s string) string {
	buf := sha256.Sum256(([]byte)(s))
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

func asSha256Hex(s string) string {
	buf := sha256.Sum256(([]byte)(s))
	return hex.EncodeToString(buf[:])
}

func genRandB64(n int) (s string, err error) {
	buf := make([]byte, n)
	if _, err = crand.Read(buf); err != nil {
		return
	}
	s = base64.RawURLEncoding.EncodeToString(buf)
	return
}

func loadOrCreateHmacKey(dataDir string) (key []byte, err error) {
	path := filepath.Join(dataDir, "server.hmac.private_key")
	buf, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return
		}
		var sbuf string
		if sbuf, err = genRandB64(256); err != nil {
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
