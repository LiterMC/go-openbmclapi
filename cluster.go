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
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hamba/avro/v2"
	"github.com/klauspost/compress/zstd"
)

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
	reconnectOnce   *sync.Once
	socket          *Socket
	cancelKeepalive context.CancelFunc
	downloadMux     sync.Mutex
	downloading     map[string]chan error
	waitEnable      []chan struct{}

	client   *http.Client
	bufSlots chan []byte

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

		downloading: make(map[string]chan error),

		client: &http.Client{
			Transport: transport,
		},
	}
	close(cr.disabled)

	cr.bufSlots = make(chan []byte, cr.maxConn)
	for i := 0; i < cr.maxConn; i++ {
		cr.bufSlots <- make([]byte, 1024*512)
	}

	// create folder strcture
	os.RemoveAll(cr.tmpDir)
	os.MkdirAll(cr.cacheDir, 0755)
	var b [1]byte
	for i := 0; i < 0x100; i++ {
		b[0] = (byte)(i)
		os.Mkdir(filepath.Join(cr.cacheDir, hex.EncodeToString(b[:])), 0755)
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

	reconnectOnce := new(sync.Once)
	cr.reconnectOnce = reconnectOnce

	cr.socket = NewSocket(NewESocket())
	cr.socket.ConnectHandle = func(*Socket) {
		connected()
	}
	cr.socket.DisconnectHandle = func(*Socket) {
		connected()
		reconnectOnce.Do(func() {
			go cr.disconnected()
		})
	}
	cr.socket.ErrorHandle = func(*Socket) {
		connected()
		reconnectOnce.Do(func() {
			go func() {
				if cr.disconnected() {
					logWarn("Reconnecting due to SIO error")
					if !cr.Connect(ctx) {
						logError("Cannot reconnect to server, exit.")
						os.Exit(0x08)
					}
					if err := cr.Enable(ctx); err != nil {
						logError("Cannot enable cluster:", err, "; exit.")
						os.Exit(0x08)
					}
				}
			}()
		})
	}
	logInfof("Dialing %s", strings.ReplaceAll(wsurl, cr.password, "<******>"))
	tctx, cancel := context.WithTimeout(ctx, time.Second*15)
	err := cr.socket.IO().DialContext(tctx, wsurl, WithHeader(header))
	cancel()
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
	tctx, cancel := context.WithTimeout(ctx, time.Second*(time.Duration)(config.ConnectTimeout))
	data, err := cr.socket.EmitAckContext(tctx, "enable", Map{
		"host":    cr.host,
		"port":    cr.publicPort,
		"version": ClusterVersion,
		"byoc":    cr.byoc,
	})
	cancel()
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
	cr.waitEnable = cr.waitEnable[:0]

	var keepaliveCtx context.Context
	keepaliveCtx, cr.cancelKeepalive = context.WithCancel(ctx)
	reconnectOnce := cr.reconnectOnce
	createInterval(keepaliveCtx, func() {
		tctx, cancel := context.WithTimeout(keepaliveCtx, KeepAliveInterval/2)
		ok := cr.KeepAlive(tctx)
		cancel()
		if !ok {
			if keepaliveCtx.Err() == nil {
				reconnectOnce.Do(func() {
					logInfo("Reconnecting due to keepalive failed")
					cr.Disable(ctx)
					logInfo("Reconnecting ...")
					if !cr.Connect(ctx) {
						logError("Cannot reconnect to server, exit.")
						os.Exit(1)
					}
					if err := cr.Enable(ctx); err != nil {
						logError("Cannot enable cluster:", err, "; exit.")
						os.Exit(1)
					}
				})
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

func (cr *Cluster) disconnected() bool {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if !cr.enabled.Swap(false) {
		return false
	}
	if cr.cancelKeepalive != nil {
		cr.cancelKeepalive()
		cr.cancelKeepalive = nil
	}
	go cr.socket.Close()
	cr.socket = nil
	return true
}

func (cr *Cluster) Disable(ctx context.Context) (ok bool) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if !cr.enabled.Load() {
		logDebug("Extra disable")
		return false
	}
	if cr.cancelKeepalive != nil {
		cr.cancelKeepalive()
		cr.cancelKeepalive = nil
	}
	if cr.socket == nil {
		return false
	}
	logInfo("Disabling cluster")
	{
		logInfo("Making keepalive before disable")
		tctx, cancel := context.WithTimeout(ctx, time.Second*10)
		ok = cr.KeepAlive(tctx)
		cancel()
		if ok {
			tctx, cancel := context.WithTimeout(ctx, time.Second*10)
			data, err := cr.socket.EmitAckContext(tctx, "disable")
			cancel()
			if err != nil {
				logErrorf("Disable failed: %v", err)
				ok = false
			} else {
				logDebug("disable ack:", data)
				if ero := data[0]; ero != nil {
					logErrorf("Disable failed: %v", ero)
					ok = false
				} else if !data[1].(bool) {
					logError("Disable failed: acked non true value")
					ok = false
				}
			}
		} else {
			logWarn("Keep alive failed, disable without send packet")
			ok = true
		}
	}

	cr.enabled.Store(false)
	go cr.socket.Close()
	cr.socket = nil
	close(cr.disabled)
	logWarn("Cluster disabled")
	return
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

func (cr *Cluster) makeReq(ctx context.Context, method string, relpath string, query url.Values) (req *http.Request, err error) {
	var target *url.URL
	if target, err = url.Parse(cr.prefix); err != nil {
		return
	}
	target.Path = path.Join(target.Path, relpath)
	if query != nil {
		target.RawQuery = query.Encode()
	}

	req, err = http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return
	}
	req.SetBasicAuth(cr.username, cr.password)
	req.Header.Set("User-Agent", cr.useragent)
	return
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
	req, err := cr.makeReq(ctx, http.MethodGet, "/openbmclapi/files", nil)
	if err != nil {
		return
	}
	res, err := cr.client.Do(req)
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
	defer zr.Close()
	if err = avro.NewDecoderForSchema(fileListSchema, zr).Decode(&files); err != nil {
		return
	}
	return
}

type syncStats struct {
	totalsize  float64
	downloaded float64
	fcount     atomic.Int32
	fl         int
}

func (cr *Cluster) SyncFiles(ctx context.Context, files []FileInfo, heavyCheck bool) {
	logInfo("Preparing to sync files...")
	if !cr.issync.CompareAndSwap(false, true) {
		logWarn("Another sync task is running!")
		return
	}

	if cr.usedOSS() {
		cr.ossSyncFiles(ctx, files, heavyCheck)
	} else {
		cr.syncFiles(ctx, files, heavyCheck)
	}

	cr.issync.Store(false)

	go cr.gc(files)
}

// syncFiles download objects to the cache folder
func (cr *Cluster) syncFiles(ctx context.Context, files []FileInfo, heavyCheck bool) error {
	missing := cr.CheckFiles(cr.cacheDir, files, heavyCheck)

	fl := len(missing)
	if fl == 0 {
		logInfo("All files was synchronized")
		return nil
	}

	// sort the files in descending order of size
	sort.Slice(missing, func(i, j int) bool { return missing[i].Size > missing[j].Size })

	var stats syncStats
	stats.fl = fl
	for _, f := range missing {
		stats.totalsize += (float64)(f.Size)
	}

	logInfof("Starting sync files, count: %d, total: %s", fl, bytesToUnit(stats.totalsize))
	start := time.Now()

	done := make(chan struct{}, 1)

	for _, f := range missing {
		pathRes, err := cr.fetchFile(ctx, &stats, f)
		if err != nil {
			logWarn("File sync interrupted")
			return err
		}
		go func(f FileInfo) {
			defer func() {
				select {
				case done <- struct{}{}:
				case <-ctx.Done():
				}
			}()
			select {
			case path := <-pathRes:
				if path != "" {
					if _, err := cr.putFileToCache(path, f); err != nil {
						logErrorf("Could not move file %q to cache:\n\t%v", path, err)
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

func (cr *Cluster) CheckFiles(dir string, files []FileInfo, heavy bool) (missing []FileInfo) {
	logInfof("Start checking files, heavy = %v", heavy)

	var fileCheckPool *threadPool[FileInfo, *FileInfo] = nil
	if heavy {
		fileCheckPool = newThreadPool[FileInfo, *FileInfo](16)
		defer fileCheckPool.Release()
	}

	for i, f := range files {
		p := filepath.Join(dir, hashToFilename(f.Hash))
		logDebugf("Checking file %s [%.2f%%]", p, (float32)(i+1)/(float32)(len(files))*100)
		stat, err := os.Stat(p)
		if err == nil {
			if sz := stat.Size(); sz != f.Size {
				logInfof("Found modified file: size of %q is %d, expect %d", p, sz, f.Size)
				os.Remove(p)
				missing = append(missing, f)
				continue
			}
			if heavy {
				fileCheckPool.Do(func(f FileInfo) (*FileInfo) {
					hashMethod, err := getHashMethod(len(f.Hash))
					if err != nil {
						logErrorf("Unknown hash method for %q", f.Hash)
						return nil
					}
					hw := hashMethod.New()

					fd, err := os.Open(p)
					if err != nil {
						logErrorf("Could not open %q: %v", p, err)
					} else {
						defer fd.Close()
						if _, err = io.Copy(hw, fd); err != nil {
							logErrorf("Could not calculate hash for %q: %v", p, err)
							return nil
						}
						var hashBuf [64]byte
						if hs := hex.EncodeToString(hw.Sum(hashBuf[:0])); hs != f.Hash {
							logInfof("Found modified file: hash of %q is %s, expect %s", p, hs, f.Hash)
						} else {
							return nil
						}
					}
					os.Remove(p)
					return &f
				}, f)
			}
			continue
		}
		if config.UseGzip {
			p += ".gz"
			if _, err := os.Stat(p); err == nil {
				if heavy {
					fileCheckPool.Do(func(f FileInfo) *FileInfo {
						hashMethod, err := getHashMethod(len(f.Hash))
						if err != nil {
							logErrorf("Unknown hash method for %q", f.Hash)
							return nil
						}
						hw := hashMethod.New()

						fd, err := os.Open(p)
						if err != nil {
							logErrorf("Could not open %q: %v", p, err)
						} else {
							defer fd.Close()
							var hashBuf [64]byte
							if r, err := gzip.NewReader(fd); err != nil {
								logErrorf("Could not decompress %q: %v", p, err)
							} else if sz, err := io.Copy(hw, r); err != nil {
								logErrorf("Could not calculate hash for %q: %v", p, err)
							} else if sz != f.Size {
								logInfof("Found modified file: size of %q is %d, expect %d", p, sz, f.Size)
							} else if hs := hex.EncodeToString(hw.Sum(hashBuf[:0])); hs != f.Hash {
								logInfof("Found modified file: hash of %q is %s, expect %s", p, hs, f.Hash)
							} else {
								return nil
							}
						}
						os.Remove(p)
						return &f
					}, f)
				}
				continue
			}
		}
		logDebugf("Could not found file %q", p)
		os.Remove(p)
		missing = append(missing, f)
	}

	if heavy {
		disabled := cr.Disabled()
		resCh := fileCheckPool.Wait()
	WAIT_LOOP:
		for {
			select {
			case f, ok := <-resCh:
				if !ok {
					break WAIT_LOOP
				}
				if f != nil {
					missing = append(missing, *f)
				}
			case <-disabled:
				logWarn("File check interrupted")
				return nil
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
		fileset[files[i].Hash] = struct{}{}
	}
	err := walkCacheDir(dir, func(path string) (_ error) {
		if cr.issync.Load() {
			return context.Canceled
		}
		hs := strings.TrimSuffix(filepath.Base(path), ".gz")
		if _, ok := fileset[hs]; !ok {
			logInfo("Found outdated file:", path)
			os.Remove(path)
		}
		return
	})
	if err != nil {
		if err == context.Canceled {
			logWarn("Garbage collector interrupted at", dir)
		} else {
			logErrorf("Garbage collector error: %v", err)
		}
		return
	}
	logInfo("Garbage collect finished for", dir)
}

func (cr *Cluster) fetchFile(ctx context.Context, stats *syncStats, f FileInfo) (<-chan string, error) {
	var buf []byte
WAIT_SLOT:
	for {
		select {
		case buf = <-cr.bufSlots:
			break WAIT_SLOT
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	pathRes := make(chan string, 1)
	go func() {
		defer func() {
			cr.bufSlots <- buf
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

var noOpenQuery = url.Values{
	"noopen": {"1"},
}

func (cr *Cluster) fetchFileWithBuf(ctx context.Context, f FileInfo, hashMethod crypto.Hash, buf []byte) (path string, err error) {
	var (
		query url.Values = nil
		req   *http.Request
		res   *http.Response
		fd    *os.File
		r     io.Reader
	)
	if config.NoOpen {
		query = noOpenQuery
	}
	if req, err = cr.makeReq(ctx, http.MethodGet, f.Path, query); err != nil {
		return
	}
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	if res, err = cr.client.Do(req); err != nil {
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
	switch strings.ToLower(res.Header.Get("Content-Encoding")) {
	case "gzip":
		if r, err = gzip.NewReader(res.Body); err != nil {
			return
		}
	case "deflate":
		if r, err = zlib.NewReader(res.Body); err != nil {
			return
		}
	default:
		r = res.Body
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

	_, err = io.CopyBuffer(io.MultiWriter(hw, fd), r, buf)
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
		err = fmt.Errorf("File size wrong, got %d, expect %d", t, f.Size)
		return
	} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != f.Hash {
		err = fmt.Errorf("File hash not match, got %s, expect %s", hs, f.Hash)
		return
	}
	return
}

func (cr *Cluster) putFileToCache(src string, f FileInfo) (compressed bool, err error) {
	defer os.Remove(src)

	targetPath := filepath.Join(cr.cacheDir, hashToFilename(f.Hash))
	if config.UseGzip && f.Size > 1024*10 {
		var srcFd, dstFd *os.File
		if srcFd, err = os.Open(src); err != nil {
			return
		}
		defer srcFd.Close()
		targetPath += ".gz"
		if dstFd, err = os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
			return
		}
		defer dstFd.Close()
		w := gzip.NewWriter(dstFd)
		defer w.Close()
		if _, err = io.Copy(w, srcFd); err != nil {
			return
		}
		return true, nil
	}

	os.Remove(targetPath) // remove the old file if exists
	if err = os.Rename(src, targetPath); err != nil {
		return
	}
	os.Chmod(targetPath, 0644)

	// TODO: support hijack with compressed file
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
	return false, nil
}

func (cr *Cluster) lockDownloading(target string) (chan error, bool) {
	cr.downloadMux.Lock()
	defer cr.downloadMux.Unlock()

	if ch := cr.downloading[target]; ch != nil {
		return ch, true
	}
	ch := make(chan error, 1)
	cr.downloading[target] = ch
	return ch, false
}

func (cr *Cluster) DownloadFile(ctx context.Context, hash string) (compressed bool, err error) {
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
	done, ok := cr.lockDownloading(hash)
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
	if compressed, err = cr.putFileToCache(path, f); err != nil {
		return
	}
	return
}

func walkCacheDir(dir string, walker func(path string) (err error)) (err error) {
	var b [1]byte
	for i := 0; i < 0x100; i++ {
		b[0] = (byte)(i)
		d := filepath.Join(dir, hex.EncodeToString(b[:]))
		var files []os.DirEntry
		if files, err = os.ReadDir(d); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return
		} else {
			for _, f := range files {
				p := filepath.Join(d, f.Name())
				if err = walker(p); err != nil {
					return
				}
			}
		}
	}
	return nil
}
