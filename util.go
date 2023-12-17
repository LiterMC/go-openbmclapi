/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
 * All rights reserved
 *
 *  This program is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU General Public License as published by
 *  the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU General Public License for more details.
 *
 *  You should have received a copy of the GNU General Public License
 *  along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */
package main

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func split(str string, b byte) (l, r string) {
	i := strings.IndexByte(str, b)
	if i >= 0 {
		return str[:i], str[i+1:]
	}
	return str, ""
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
	unit := "Byte"
	if size >= 1000 {
		size /= 1024
		unit = "KB"
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
	}
	return fmt.Sprintf("%.1f", size) + unit
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
