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
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/LiterMC/go-openbmclapi/log"
)

var closedCh = func() <-chan struct{} {
	ch := make(chan struct{}, 0)
	close(ch)
	return ch
}()

func createInterval(ctx context.Context, do func(), delay time.Duration) {
	ticker := time.NewTicker(delay)
	context.AfterFunc(ctx, func() {
		ticker.Stop()
	})
	go func() {
		defer log.RecordPanic()
		defer ticker.Stop()

		for range ticker.C {
			do()
			// If another a tick passed during the job, ignore it
			select {
			case <-ticker.C:
			default:
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
