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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hamba/avro/v2"
	"github.com/klauspost/compress/zstd"
)

const ClusterVersion = "1.6.7"

type Cluster struct {
	host       string
	publicPort uint16
	username   string
	password   string
	useragent  string
	prefix     string
	byoc       bool

	cacheDir string
	tmpDir   string
	dataDir  string
	maxConn  int
	ossList  []*OSSItem

	stats  Stats
	hits   atomic.Int32
	hbts   atomic.Int64
	issync atomic.Bool

	mux             sync.RWMutex
	enabled         atomic.Bool
	disabled        chan struct{}
	socket          *Socket
	cancelKeepalive context.CancelFunc
	downloadMux     sync.Mutex
	downloading     map[string]chan struct{}
	waitEnable      []chan struct{}

	client *http.Client

	handlerAPIv0 http.Handler
	handlerAPIv1 http.Handler
}

func NewCluster(
	ctx context.Context, baseDir string,
	host string, publicPort uint16,
	username string, password string,
	byoc bool, dialer *net.Dialer,
	ossList []*OSSItem,
) (cr *Cluster, err error) {
	transport := &http.Transport{}
	if dialer != nil {
		transport.DialContext = dialer.DialContext
	}
	cr = &Cluster{
		host:       host,
		publicPort: publicPort,
		username:   username,
		password:   password,
		useragent:  "openbmclapi-cluster/" + ClusterVersion,
		prefix:     "https://openbmclapi.bangbang93.com",
		byoc:       byoc,

		cacheDir: filepath.Join(baseDir, "cache"),
		tmpDir:   filepath.Join(baseDir, "cache", ".tmp"),
		dataDir:  filepath.Join(baseDir, "data"),
		maxConn:  config.DownloadMaxConn,
		ossList:  ossList,

		disabled: make(chan struct{}, 0),

		downloading: make(map[string]chan struct{}),

		client: &http.Client{
			Transport: transport,
		},
	}
	close(cr.disabled)

	// create folder strcture
	os.RemoveAll(cr.tmpDir)
	os.MkdirAll(cr.cacheDir, 0755)
	for i := 0; i < 0x100; i++ {
		os.Mkdir(filepath.Join(cr.cacheDir, hex.EncodeToString([]byte{(byte)(i)})), 0755)
	}
	os.MkdirAll(cr.dataDir, 0755)
	os.MkdirAll(cr.tmpDir, 0700)

	// read old stats
	if err = cr.stats.Load(cr.dataDir); err != nil {
		return
	}
	return
}

func (cr *Cluster) usedOSS() bool {
	return cr.ossList != nil
}

func (cr *Cluster) Connect(ctx context.Context) bool {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if cr.socket != nil {
		logDebug("Extra connect")
		return true
	}
	wsurl := httpToWs(cr.prefix) +
		fmt.Sprintf("/socket.io/?clusterId=%s&clusterSecret=%s&EIO=4&transport=websocket", cr.username, cr.password)
	header := http.Header{}
	header.Set("Origin", cr.prefix)
	header.Set("User-Agent", cr.useragent)

	connectCh := make(chan struct{}, 0)
	connected := sync.OnceFunc(func() {
		close(connectCh)
	})

	cr.socket = NewSocket(NewESocket())
	cr.socket.ConnectHandle = func(*Socket) {
		connected()
	}
	cr.socket.DisconnectHandle = func(*Socket) {
		connected()
		go cr.Disable(ctx)
	}
	cr.socket.ErrorHandle = func(*Socket) {
		connected()
		go func() {
			logWarn("Reconnecting due to SIO error")
			if cr.Disable(ctx) {
				if !cr.Connect(ctx) {
					logError("Cannot reconnect to server, exit.")
					os.Exit(1)
				}
				if err := cr.Enable(ctx); err != nil {
					logError("Cannot enable cluster:", err, "; exit.")
					os.Exit(1)
				}
			}
		}()
	}
	logInfof("Dialing %s", strings.ReplaceAll(wsurl, cr.password, "<******>"))
	err := cr.socket.IO().DialContext(ctx, wsurl, WithHeader(header))
	if err != nil {
		logError("Websocket connect error:", err)
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-connectCh:
	}
	return true
}

func (cr *Cluster) WaitForEnable() <-chan struct{} {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	ch := make(chan struct{}, 0)
	if cr.enabled.Load() {
		close(ch)
	} else {
		cr.waitEnable = append(cr.waitEnable, ch)
	}
	return ch
}

func (cr *Cluster) Enable(ctx context.Context) (err error) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if cr.enabled.Load() {
		logDebug("Extra enable")
		return
	}
	logInfo("Sending enable packet")
	data, err := cr.socket.EmitAckContext(ctx, "enable", Map{
		"host":    cr.host,
		"port":    cr.publicPort,
		"version": ClusterVersion,
		"byoc":    cr.byoc,
	})
	if err != nil {
		return
	}
	logInfo("get enable ack:", data)
	if ero := data[0]; ero != nil {
		return fmt.Errorf("Enable failed: %v", ero)
	}
	if !data[1].(bool) {
		return errors.New("Enable ack non true value")
	}
	cr.disabled = make(chan struct{}, 0)
	cr.enabled.Store(true)
	for _, ch := range cr.waitEnable {
		close(ch)
	}

	var keepaliveCtx context.Context
	keepaliveCtx, cr.cancelKeepalive = context.WithCancel(ctx)
	createInterval(keepaliveCtx, func() {
		ctx, cancel := context.WithTimeout(keepaliveCtx, KeepAliveInterval/2)
		defer cancel()
		if !cr.KeepAlive(ctx) {
			logInfo("Reconnecting due to keepalive failed")
			cr.Disable(keepaliveCtx)
			logError("TODO: figure out what caused infinite reconnect")
			os.Exit(0xfe)
			if !cr.Connect(keepaliveCtx) {
				logError("Cannot reconnect to server, exit.")
				os.Exit(1)
			}
			if err := cr.Enable(keepaliveCtx); err != nil {
				logError("Cannot enable cluster:", err, "; exit.")
				os.Exit(1)
			}
		}
	}, KeepAliveInterval)
	return
}

// KeepAlive will fresh hits & hit bytes data and send the keep-alive packet
func (cr *Cluster) KeepAlive(ctx context.Context) (ok bool) {
	hits, hbts := cr.hits.Swap(0), cr.hbts.Swap(0)
	cr.stats.AddHits(hits, hbts)
	data, err := cr.socket.EmitAckContext(ctx, "keep-alive", Map{
		"time":  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"hits":  hits,
		"bytes": hbts,
	})
	if e := cr.stats.Save(cr.dataDir); e != nil {
		logError("Error when saving status:", e)
	}
	if err != nil {
		logError("Error when keep-alive:", err)
		return false
	}
	if ero := data[0]; len(data) <= 1 || ero != nil {
		logError("Keep-alive failed:", ero)
		return false
	}
	logInfo("Keep-alive success:", hits, bytesToUnit((float64)(hbts)), data[1])
	return true
}

func (cr *Cluster) Disable(ctx context.Context) (ok bool) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if !cr.enabled.Load() {
		logDebug("Extra disable")
		return false
	}
	logInfo("Disabling cluster")
	if cr.cancelKeepalive != nil {
		cr.cancelKeepalive()
		cr.cancelKeepalive = nil
	}
	if cr.socket == nil {
		return false
	}
	{
		logInfo("Making keepalive before disable")
		tctx, cancel := context.WithTimeout(ctx, time.Second*10)
		cr.KeepAlive(tctx)
		cancel()
	}
	data, err := cr.socket.EmitAckContext(ctx, "disable")
	cr.enabled.Store(false)
	cr.socket.Close()
	cr.socket = nil
	close(cr.disabled)
	if err != nil {
		return false
	}
	logDebug("disable ack:", data)
	if ero := data[0]; ero != nil {
		logErrorf("Disable failed: %v", ero)
		return false
	}
	if !data[1].(bool) {
		logError("Disable failed: ack non true value")
		return false
	}
	return true
}

func (cr *Cluster) Disabled() <-chan struct{} {
	cr.mux.RLock()
	defer cr.mux.RUnlock()
	return cr.disabled
}

type CertKeyPair struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func (pair *CertKeyPair) SaveAsFile() (cert, key string, err error) {
	const pemBase = "pems"
	if _, err = os.Stat(pemBase); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return
		}
		if err = os.Mkdir(pemBase, 0700); err != nil {
			return
		}
	}
	cert, key = filepath.Join(pemBase, "cert.pem"), filepath.Join(pemBase, "key.pem")
	if err = os.WriteFile(cert, ([]byte)(pair.Cert), 0600); err != nil {
		return
	}
	if err = os.WriteFile(key, ([]byte)(pair.Key), 0600); err != nil {
		return
	}
	return
}

func (cr *Cluster) RequestCert(ctx context.Context) (ckp *CertKeyPair, err error) {
	logInfo("Requesting certificates, please wait ...")
	data, err := cr.socket.EmitAckContext(ctx, "request-cert")
	if err != nil {
		return
	}
	if ero := data[0]; ero != nil {
		err = fmt.Errorf("socket.io remote error: %v", ero)
		return
	}
	pair := data[1].(map[string]any)
	ckp = &CertKeyPair{
		Cert: pair["cert"].(string),
		Key:  pair["key"].(string),
	}
	logInfo("Certificate requested")
	return
}

func (cr *Cluster) queryFunc(ctx context.Context, method string, url string, call func(*http.Request)) (res *http.Response, err error) {
	var req *http.Request
	req, err = http.NewRequestWithContext(ctx, method, cr.prefix+url, nil)
	if err != nil {
		return
	}
	req.SetBasicAuth(cr.username, cr.password)
	req.Header.Set("User-Agent", cr.useragent)
	if call != nil {
		call(req)
	}
	res, err = cr.client.Do(req)
	return
}

func (cr *Cluster) queryURL(ctx context.Context, method string, url string) (res *http.Response, err error) {
	return cr.queryFunc(ctx, method, url, nil)
}

func (cr *Cluster) queryURLHeader(ctx context.Context, method string, url string, header map[string]string) (res *http.Response, err error) {
	return cr.queryFunc(ctx, method, url, func(req *http.Request) {
		if header != nil {
			for k, v := range header {
				req.Header.Set(k, v)
			}
		}
	})
}

type FileInfo struct {
	Path string `json:"path" avro:"path"`
	Hash string `json:"hash" avro:"hash"`
	Size int64  `json:"size" avro:"size"`
}

// from <https://github.com/bangbang93/openbmclapi/blob/master/src/cluster.ts>
var fileListSchema = avro.MustParse(`{
  "type": "array",
  "items": {
    "type": "record",
  	"name": "fileinfo",
    "fields": [
      {"name": "path", "type": "string"},
      {"name": "hash", "type": "string"},
      {"name": "size", "type": "long"}
    ]
  }
}`)

func (cr *Cluster) GetFileList(ctx context.Context) (files []FileInfo, err error) {
	res, err := cr.queryURL(ctx, "GET", "/openbmclapi/files")
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(res.Body)
		err = fmt.Errorf("Unexpected status code: %d %s Body:\n%s", res.StatusCode, res.Status, (string)(data))
		return
	}
	logDebug("Parsing filelist body ...")
	zr, err := zstd.NewReader(res.Body)
	if err != nil {
		return
	}
	defer zr.Close() // TODO: reuse the decoder?
	if err = avro.NewDecoderForSchema(fileListSchema, zr).Decode(&files); err != nil {
		return
	}
	return
}

type syncStats struct {
	totalsize  float64
	downloaded float64
	slots      chan []byte
	fcount     atomic.Int32
	fl         int
}

func (cr *Cluster) SyncFiles(ctx context.Context, files []FileInfo) {
	logInfo("Preparing to sync files...")
	if !cr.issync.CompareAndSwap(false, true) {
		logWarn("Another sync task is running!")
		return
	}

	if cr.usedOSS() {
		cr.ossSyncFiles(ctx, files)
	} else {
		cr.syncFiles(ctx, files)
	}

	cr.issync.Store(false)

	go cr.gc(files)
}

// syncFiles download objects to the cache folder
func (cr *Cluster) syncFiles(ctx context.Context, files []FileInfo) error {
	missing := cr.CheckFiles(cr.cacheDir, files)

	fl := len(missing)
	if fl == 0 {
		logInfo("All files was synchronized")
		return nil
	}

	// sort the files in descending order of size
	sort.Slice(missing, func(i, j int) bool { return missing[i].Size > missing[j].Size })

	var stats syncStats
	stats.slots = make(chan []byte, cr.maxConn)
	stats.fl = fl
	for _, f := range missing {
		stats.totalsize += (float64)(f.Size)
	}
	for i := cap(stats.slots); i > 0; i-- {
		stats.slots <- make([]byte, 1024*1024)
	}

	logInfof("Starting sync files, count: %d, total: %s", fl, bytesToUnit(stats.totalsize))
	start := time.Now()

	for _, f := range missing {
		pathRes, err := cr.fetchFile(ctx, &stats, f)
		if err != nil {
			logWarn("File sync interrupted")
			return err
		}
		go func(f FileInfo) {
			select {
			case path := <-pathRes:
				if path != "" {
					if err := cr.putFileToCache(path, f); err != nil {
						logErrorf("Could not move file %q to cache:\n\t%v", path, err)
					}
				}
			case <-ctx.Done():
				return
			}
		}(f)
	}
	for i := cap(stats.slots); i > 0; i-- {
		select {
		case <-stats.slots:
		case <-ctx.Done():
			logWarn("File sync interrupted")
			return ctx.Err()
		}
	}

	use := time.Since(start)
	logInfof("All files was synchronized, use time: %v, %s/s", use, bytesToUnit(stats.totalsize/use.Seconds()))
	return nil
}

func (cr *Cluster) CheckFiles(dir string, files []FileInfo) (missing []FileInfo) {
	logInfof("Start checking files at %q", dir)
	usedOSS := cr.usedOSS()
	for i, f := range files {
		p := filepath.Join(dir, hashToFilename(f.Hash))
		logDebugf("Checking file %s [%.2f%%]", p, (float32)(i+1)/(float32)(len(files))*100)
		if usedOSS && f.Size == 0 {
			logDebugf("Skipped empty file %s", p)
			continue
		}
		stat, err := os.Stat(p)
		if err == nil {
			if sz := stat.Size(); sz != f.Size {
				logInfof("Found modified file: size of %q is %s but expect %s",
					p, bytesToUnit((float64)(sz)), bytesToUnit((float64)(f.Size)))
				missing = append(missing, f)
			}
		} else {
			logDebugf("Could not found file %q", p)
			missing = append(missing, f)
			if !errors.Is(err, os.ErrNotExist) {
				os.Remove(p)
			}
		}
	}
	logInfo("File check finished")
	return
}

func (cr *Cluster) gc(files []FileInfo) {
	if cr.ossList == nil {
		cr.gcAt(files, cr.cacheDir)
	} else {
		for _, item := range cr.ossList {
			cr.gcAt(files, filepath.Join(item.FolderPath, "download"))
		}
	}
}

func (cr *Cluster) gcAt(files []FileInfo, dir string) {
	logInfo("Starting garbage collector at", dir)
	fileset := make(map[string]struct{}, 128)
	for i, _ := range files {
		fileset[filepath.Join(dir, hashToFilename(files[i].Hash))] = struct{}{}
	}
	stack := make([]string, 0, 256)
	stack = append(stack, dir)
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fil, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, f := range fil {
			if cr.issync.Load() {
				logWarn("Garbage collector interrupted at", dir)
				return
			}
			n := filepath.Join(p, f.Name())
			if stat, err := os.Stat(n); err == nil {
				if stat.IsDir() {
					stack = append(stack, n)
				} else if _, ok := fileset[n]; !ok {
					logInfo("Found outdated file:", n)
					os.Remove(n)
				}
			}
		}
	}
	logInfo("Garbage collect finished for", dir)
}

func (cr *Cluster) fetchFile(ctx context.Context, stats *syncStats, f FileInfo) (<-chan string, error) {
	var buf []byte
WAIT_SLOT:
	for {
		select {
		case buf = <-stats.slots:
			break WAIT_SLOT
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	pathRes := make(chan string, 1)
	go func() {
		defer func() {
			stats.slots <- buf
		}()
		defer close(pathRes)

		for trycount := 1; trycount <= 3; trycount++ {
			logInfof("Downloading: %s [%s]", f.Path, bytesToUnit((float64)(f.Size)))

			hashMethod, err := getHashMethod(len(f.Hash))
			if err == nil {
				var path string
				if path, err = cr.fetchFileWithBuf(ctx, f, hashMethod, buf); err == nil {
					pathRes <- path
					stats.downloaded += (float64)(f.Size)
					stats.fcount.Add(1)
					logInfof("Downloaded: %s [%s/%s ; %d/%d] %.2f%%", f.Path,
						bytesToUnit(stats.downloaded), bytesToUnit(stats.totalsize),
						stats.fcount.Load(), stats.fl,
						stats.downloaded/stats.totalsize*100)
					return
				}
			}

			logErrorf("File download error: %s [%s/%s ; %d/%d] %.2f%%\n\t%s",
				f.Path,
				bytesToUnit(stats.downloaded), bytesToUnit(stats.totalsize),
				stats.fcount.Load(), stats.fl,
				stats.downloaded/stats.totalsize*100,
				err)
		}
		stats.fcount.Add(1)
	}()
	return pathRes, nil
}

func (cr *Cluster) fetchFileWithBuf(ctx context.Context, f FileInfo, hashMethod crypto.Hash, buf []byte) (path string, err error) {
	var (
		res *http.Response
		fd  *os.File
	)
	if res, err = cr.queryURL(ctx, "GET", f.Path); err != nil {
		return
	}
	defer res.Body.Close()
	if err = ctx.Err(); err != nil {
		return
	}
	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("Unexpected status code: %d", res.StatusCode)
		return
	}

	hw := hashMethod.New()

	if fd, err = os.CreateTemp(cr.tmpDir, "*.downloading"); err != nil {
		return
	}
	path = fd.Name()
	defer func(path string) {
		if err != nil {
			os.Remove(path)
		}
	}(path)

	_, err = io.CopyBuffer(io.MultiWriter(hw, fd), res.Body, buf)
	stat, err2 := fd.Stat()
	fd.Close()
	if err != nil {
		return
	}
	if err2 != nil {
		err = err2
		return
	}
	if t := stat.Size(); f.Size >= 0 && t != f.Size {
		err = fmt.Errorf("File size wrong, got %s, expect %s", bytesToUnit((float64)(t)), bytesToUnit((float64)(f.Size)))
		return
	} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != f.Hash {
		err = fmt.Errorf("File hash not match, got %s, expect %s", hs, f.Hash)
		return
	}
	return
}

func (cr *Cluster) putFileToCache(src string, f FileInfo) (err error) {
	targetPath := filepath.Join(cr.cacheDir, hashToFilename(f.Hash))
	os.Remove(targetPath) // remove the old file if exists
	if err = os.Rename(src, targetPath); err != nil {
		return
	}
	os.Chmod(targetPath, 0644)

	if config.Hijack.Enable {
		if !strings.HasPrefix(f.Path, "/openbmclapi/download/") {
			target := filepath.Join(config.Hijack.Path, filepath.FromSlash(f.Path))
			dir := filepath.Dir(target)
			os.MkdirAll(dir, 0755)
			if rp, err := filepath.Rel(dir, targetPath); err == nil {
				os.Symlink(rp, target)
			}
		}
	}
	return
}

func (cr *Cluster) lockDownloading(target string) (chan struct{}, bool) {
	cr.downloadMux.Lock()
	defer cr.downloadMux.Unlock()
	if ch := cr.downloading[target]; ch != nil {
		return ch, true
	}
	ch := make(chan struct{}, 1)
	cr.downloading[target] = ch
	return ch, false
}

func (cr *Cluster) DownloadFile(ctx context.Context, dir string, hash string) (err error) {
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
		case <-done:
		case <-cr.disabled:
		}
		return
	}
	defer close(done)
	path, err := cr.fetchFileWithBuf(ctx, f, hashMethod, buf)
	if err != nil {
		return
	}
	return copyFile(path, target, 0644)
}
