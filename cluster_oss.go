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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const glbChunkSize = 1024 * 1024

var glbChunk [glbChunkSize]byte

func createOssMirrorDir(path string, createAll bool) {
	logInfof("Creating OSS folder %s", path)
	if err := os.MkdirAll(path, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", path, err)
		os.Exit(2)
	}

	if createAll {
		logDebug("Creating measure files")
		measureDir := filepath.Join(path, "measure")
		if err := os.Mkdir(measureDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			logErrorf("Cannot create OSS mirror folder %q: %v", measureDir, err)
			os.Exit(2)
		}
		for i := 1; i <= 200; i++ {
			if err := createMeasureFile(measureDir, i); err != nil {
				os.Exit(2)
			}
		}
		logDebug("Measure files created")
	}
}

func createMeasureFile(baseDir string, size int) (err error) {
	t := filepath.Join(baseDir, strconv.Itoa(size))
	if stat, err := os.Stat(t); err == nil {
		size := (int64)(size) * glbChunkSize
		x := stat.Size()
		if x == size {
			return nil
		}
		logDebugf("File [%d] size %d does not match %d", size, x, size)
	} else if !errors.Is(err, os.ErrNotExist) {
		logDebugf("Cannot get stat of %s: %v", t, err)
	}
	logDebug("Writing measure file", t)
	fd, err := os.Create(t)
	if err != nil {
		logErrorf("Cannot create OSS mirror measure file %q: %v", t, err)
		return
	}
	defer fd.Close()
	for j := 0; j < size; j++ {
		if _, err = fd.Write(glbChunk[:]); err != nil {
			logErrorf("Cannot write OSS mirror measure file %q: %v", t, err)
			return
		}
	}
	return nil
}

func checkWebdav(ctx context.Context, client *http.Client, redirectBase string, folderPath string, size int) (supportRange bool, err error) {
	targetSize := (int64)(size) * 1024 * 1024
	logInfof("Checking %s for %d bytes ...", redirectBase, targetSize)

	if err = createMeasureFile(filepath.Join(folderPath, "measure"), size); err != nil {
		return
	}

	target, err := url.JoinPath(redirectBase, "measure", strconv.Itoa(size))
	if err != nil {
		return false, fmt.Errorf("Cannot check OSS server: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return
	}
	req.Header.Set("Range", "bytes=1-")
	res, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("OSS check request failed %q: %w", target, err)
	}
	defer res.Body.Close()
	logDebugf("OSS check response status code %d %s", res.StatusCode, res.Status)
	if supportRange = res.StatusCode == http.StatusPartialContent; supportRange {
		logDebug("OSS support Range header!")
		targetSize--
	} else if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("OSS check request failed %q: %d %s", target, res.StatusCode, res.Status)
	} else {
		crange := res.Header.Get("Content-Range")
		if len(crange) > 0 {
			logWarn("Non standard http response detected, responsed 'Content-Range' header with status 200, expected status 206")
			fields := strings.Fields(crange)
			if len(fields) >= 2 && fields[0] == "bytes" && strings.HasPrefix(fields[1], "1-") {
				logDebug("OSS support Range header?")
				supportRange = true
				targetSize--
			}
		}
	}
	logDebug("reading OSS response")
	start := time.Now()
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		return false, fmt.Errorf("OSS check request failed %q: %w", target, err)
	}
	used := time.Since(start)
	if n != targetSize {
		return false, fmt.Errorf("OSS check request failed %q: expected %d bytes, but got %d bytes", target, targetSize, n)
	}
	logInfof("Check finished for %q, used %v, %s/s; supportRange=%v", target, used, bytesToUnit((float64)(n)/used.Seconds()), supportRange)
	return
}
