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
	"encoding/pem"
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

func splitCSV(line string) (values map[string]float32) {
	list := strings.Split(line, ",")
	values = make(map[string]float32, len(list))
	for _, v := range list {
		name, opt := split(strings.ToLower(strings.TrimSpace(v)), ';')
		var q float64 = 1
		if v, ok := strings.CutPrefix(opt, "q="); ok {
			q, _ = strconv.ParseFloat(v, 32)
		}
		values[name] = (float32)(q)
	}
	return
}

func hashToFilename(hash string) string {
	return filepath.Join(hash[0:2], hash)
}

func createInterval(ctx context.Context, do func(), delay time.Duration) {
	logDebug("Interval created:", ctx)
	go func() {
		ticker := time.NewTicker(delay)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logDebug("Interval stopped:", ctx)
				return
			case <-ticker.C:
				do()
				// If there is a tick passed during the job, ignore it
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

func bytesToUnit(size float64) string {
	if size < 1000 {
		return fmt.Sprintf("%dB", (int)(size))
	}
	size /= 1024
	unit := "KB"
	if size >= 1000 {
		size /= 1024
		unit = "MB"
		if size >= 1000 {
			size /= 1024
			unit = "GB"
			if size >= 1000 {
				size /= 1024
				unit = "TB"
			}
		}
	}
	return fmt.Sprintf("%.1f%s", size, unit)
}

func withContext(ctx context.Context, call func()) bool {
	if ctx == nil {
		call()
		return true
	}
	done := make(chan struct{}, 0)
	go func() {
		defer close(done)
		call()
	}()
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return true
	}
}

const BUF_SIZE = 1024 * 512 // 512KB
var bufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, BUF_SIZE)
		return &buf
	},
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

func parseCertCommonName(cert []byte) (string, error) {
	rest := cert
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return "", nil
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", err
		}
		return cert.Subject.CommonName, nil
	}
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

func forEachSliceFromRandomIndex(leng int, cb func(i int) (done bool)) (done bool) {
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

type devNull struct{}

var (
	DevNull = devNull{}

	_ io.ReaderAt   = DevNull
	_ io.ReadSeeker = DevNull
	_ io.Writer     = DevNull
)

func (devNull) Read([]byte) (int, error)          { return 0, io.EOF }
func (devNull) ReadAt([]byte, int64) (int, error) { return 0, io.EOF }
func (devNull) Seek(int64, int) (int64, error)    { return 0, nil }
func (devNull) Write(buf []byte) (int, error)     { return len(buf), nil }

var errNotSeeker = errors.New("r is not an io.Seeker")

func getFileSize(r io.Reader) (n int64, err error) {
	if s, ok := r.(io.Seeker); ok {
		if n, err = s.Seek(0, io.SeekEnd); err == nil {
			if _, err = s.Seek(0, io.SeekStart); err != nil {
				return
			}
		}
	} else {
		err = errNotSeeker
	}
	return
}

func checkQuerySign(hash string, secret string, query url.Values) bool {
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
