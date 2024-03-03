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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

var closedCh = func() <-chan struct{} {
	ch := make(chan struct{}, 0)
	close(ch)
	return ch
}()

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

var rd = func() chan int32 {
	ch := make(chan int32, 64)
	r := rand.New(rand.NewSource(time.Now().Unix()))
	go func() {
		for {
			ch <- r.Int31()
		}
	}()
	return ch
}()

func randIntn(n int) int {
	rn := <-rd
	return (int)(rn) % n
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

type RedirectError struct {
	Redirects []*url.URL
	Err       error
}

func ErrorFromRedirect(err error, resp *http.Response) *RedirectError {
	redirects := make([]*url.URL, 0, 4)
	for resp != nil && resp.Request != nil {
		redirects = append(redirects, resp.Request.URL)
		resp = resp.Request.Response
	}
	if len(redirects) > 1 {
		slices.Reverse(redirects)
	} else {
		redirects = nil
	}
	return &RedirectError{
		Redirects: redirects,
		Err:       err,
	}
}

func (e *RedirectError) Error() string {
	if len(e.Redirects) == 0 {
		return e.Err.Error()
	}

	var b strings.Builder
	b.WriteString("Redirect from:\n\t")
	for _, r := range e.Redirects {
		b.WriteString("- ")
		b.WriteString(r.String())
		b.WriteString("\n\t")
	}
	b.WriteString(e.Err.Error())
	return b.String()
}

func (e *RedirectError) Unwrap() error {
	return e.Err
}
