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

type fileInfoWithTargets struct {
	FileInfo
	targets []string
}

func (cr *Cluster) CheckFilesOSS(dir string, files []FileInfo, heavy bool, missing map[string]*fileInfoWithTargets) {
	addMissing := func(f FileInfo) {
		if info := missing[f.Hash]; info != nil {
			info.targets = append(info.targets, dir)
		} else {
			missing[f.Hash] = &fileInfoWithTargets{
				FileInfo: f,
				targets:  []string{dir},
			}
		}
	}
	logInfof("Start checking files at %q, heavy = %v", dir, heavy)
	var hashBuf [64]byte
	for i, f := range files {
		p := filepath.Join(dir, hashToFilename(f.Hash))
		logDebugf("Checking file %s [%.2f%%]", p, (float32)(i+1)/(float32)(len(files))*100)
		if f.Size == 0 {
			logDebugf("Skipped empty file %s", p)
			continue
		}
		stat, err := os.Stat(p)
		if err == nil {
			if sz := stat.Size(); sz != f.Size {
				logInfof("Found modified file: size of %q is %s, expect %s",
					p, bytesToUnit((float64)(sz)), bytesToUnit((float64)(f.Size)))
				goto MISSING
			}
			if heavy {
				hashMethod, err := getHashMethod(len(f.Hash))
				if err != nil {
					logErrorf("Unknown hash method for %q", f.Hash)
					continue
				}
				hw := hashMethod.New()

				fd, err := os.Open(p)
				if err != nil {
					logErrorf("Could not open %q: %v", p, err)
					goto MISSING
				}
				_, err = io.Copy(hw, fd)
				fd.Close()
				if err != nil {
					logErrorf("Could not calculate hash for %q: %v", p, err)
					continue
				}
				if hs := hex.EncodeToString(hw.Sum(hashBuf[:0])); hs != f.Hash {
					logInfof("Found modified file: hash of %q is %s, expect %s", p, hs, f.Hash)
					goto MISSING
				}
			}
			continue
		}
		logDebugf("Could not found file %q", p)
	MISSING:
		os.Remove(p)
		addMissing(f)
	}
	logInfo("File check finished")
	return
}

func (cr *Cluster) ossSyncFiles(ctx context.Context, files []FileInfo, heavyCheck bool) error {
	missingMap := make(map[string]*fileInfoWithTargets, 4)
	for _, item := range cr.ossList {
		dir := filepath.Join(item.FolderPath, "download")
		cr.CheckFilesOSS(dir, files, heavyCheck, missingMap)
	}

	missing := make([]*fileInfoWithTargets, 0, len(missingMap))
	for _, f := range missingMap {
		missing = append(missing, f)
	}

	fl := len(missing)
	if fl == 0 {
		logInfo("All oss files was synchronized")
		return nil
	}

	var stats syncStats
	stats.fl = fl
	for _, f := range missing {
		stats.totalsize += (float64)(f.Size)
	}

	logInfof("Starting sync files, count: %d, total: %s", fl, bytesToUnit(stats.totalsize))
	start := time.Now()

	done := make(chan struct{}, 1)

	for _, f := range missing {
		logDebugf("File %s is for %v", f.Hash, f.targets)
		pathRes, err := cr.fetchFile(ctx, &stats, f.FileInfo)
		if err != nil {
			logWarn("File sync interrupted")
			return err
		}
		go func(f *fileInfoWithTargets) {
			defer func() {
				select {
				case done <- struct{}{}:
				case <-ctx.Done():
				}
			}()
			select {
			case path := <-pathRes:
				if path != "" {
					defer os.Remove(path)
					// acquire slot here
					buf := <-cr.bufSlots
					defer func() {
						cr.bufSlots <- buf
					}()
					var srcFd *os.File
					if srcFd, err = os.Open(path); err != nil {
						return
					}
					defer srcFd.Close()
					relpath := hashToFilename(f.Hash)
					for _, target := range f.targets {
						target = filepath.Join(target, relpath)
						dstFd, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
						if err != nil {
							logErrorf("Could not create %q: %v", target, err)
							continue
						}
						if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
							logErrorf("Could not seek file %q: %v", path, err)
							continue
						}
						_, err = io.CopyBuffer(dstFd, srcFd, buf)
						dstFd.Close()
						if err != nil {
							logErrorf("Could not copy from %q to %q:\n\t%v", path, target, err)
							continue
						}
					}
				}
			case <-ctx.Done():
				return
			}
		}(f)
	}
	for i := len(missing); i > 0; i-- {
		select {
		case <-done:
		case <-ctx.Done():
			logWarn("File sync interrupted")
			return ctx.Err()
		}
	}

	use := time.Since(start)
	logInfof("All files was synchronized, use time: %v, %s/s", use, bytesToUnit(stats.totalsize/use.Seconds()))
	return nil
}

const glbChunkSize = 1024 * 1024

var glbChunk [glbChunkSize]byte

func createOssMirrorDir(item *OSSItem) {
	logInfof("Creating OSS folder %s", item.FolderPath)
	if err := os.MkdirAll(item.FolderPath, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", item.FolderPath, err)
		os.Exit(2)
	}

	if item.PreCreateMeasures {
		logDebug("Creating measure files")
		measureDir := filepath.Join(item.FolderPath, "measure")
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

func createMeasureFile(baseDir string, n int) (err error) {
	t := filepath.Join(baseDir, strconv.Itoa(n))
	if stat, err := os.Stat(t); err == nil {
		size := (int64)(n) * glbChunkSize
		x := stat.Size()
		if x == size {
			return nil
		}
		logDebugf("File [%d] size %d does not match %d", n, x, size)
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
	for j := 0; j < n; j++ {
		if _, err = fd.Write(glbChunk[:]); err != nil {
			logErrorf("Cannot write OSS mirror measure file %q: %v", t, err)
			return
		}
	}
	return nil
}

func checkOSS(ctx context.Context, client *http.Client, item *OSSItem, size int) (supportRange bool, err error) {
	targetSize := (int64)(size) * 1024 * 1024
	logInfof("Checking %s for %d bytes ...", item.RedirectBase, targetSize)

	target, err := url.JoinPath(item.RedirectBase, "measure", strconv.Itoa(size))
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

func (cr *Cluster) DownloadFileOSS(ctx context.Context, dir string, hash string) (err error) {
	hashMethod, err := getHashMethod(len(hash))
	if err != nil {
		return
	}

	var buf []byte
	{
		buf0 := bufPool.Get().(*[]byte)
		defer bufPool.Put(buf0)
		buf = *buf0
	}
	f := FileInfo{
		Path: "/openbmclapi/download/" + hash + "?noopen=1",
		Hash: hash,
		Size: -1,
	}
	target := filepath.Join(dir, hashToFilename(hash))
	done, ok := cr.lockDownloading(target)
	if ok {
		select {
		case err = <-done:
		case <-cr.Disabled():
		}
		return
	}
	defer func() {
		done <- err
	}()
	path, err := cr.fetchFileWithBuf(ctx, f, hashMethod, buf)
	if err != nil {
		return
	}
	defer os.Remove(path)
	err = copyFile(path, target, 0644)
	return
}
