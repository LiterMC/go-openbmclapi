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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiterMC/socket.io"
	"github.com/LiterMC/socket.io/engine.io"
	"github.com/gorilla/websocket"
	"github.com/gregjones/httpcache"
	"github.com/hamba/avro/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	gocache "github.com/LiterMC/go-openbmclapi/cache"
	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type Cluster struct {
	host          string
	publicPort    uint16
	clusterId     string
	clusterSecret string
	prefix        string
	byoc          bool
	jwtIssuer     string

	dataDir            string
	maxConn            int
	storageOpts        []storage.StorageOption
	storages           []storage.Storage
	storageWeights     []uint
	storageTotalWeight uint
	cache              gocache.Cache
	apiHmacKey         []byte

	stats     Stats
	hits      atomic.Int32
	hbts      atomic.Int64
	issync    atomic.Bool
	syncProg  atomic.Int64
	syncTotal atomic.Int64

	mux             sync.RWMutex
	enabled         atomic.Bool
	disabled        chan struct{}
	waitEnable      []chan struct{}
	shouldEnable    atomic.Bool
	reconnectCount  int
	socket          *socket.Socket
	cancelKeepalive context.CancelFunc
	downloadMux     sync.Mutex
	downloading     map[string]chan error
	fileMux         sync.RWMutex
	fileset         map[string]int64
	authTokenMux    sync.RWMutex
	authToken       *ClusterToken

	client    *http.Client
	cachedCli *http.Client
	bufSlots  *limited.BufSlots
	tokens    *TokenStorage

	wsUpgrader   *websocket.Upgrader
	handlerAPIv0 http.Handler
	handlerAPIv1 http.Handler
}

func NewCluster(
	ctx context.Context,
	prefix string,
	baseDir string,
	host string, publicPort uint16,
	clusterId string, clusterSecret string,
	byoc bool, dialer *net.Dialer,
	storageOpts []storage.StorageOption,
	cache gocache.Cache,
) (cr *Cluster) {
	transport := http.DefaultTransport
	if dialer != nil {
		transport = &http.Transport{
			DialContext: dialer.DialContext,
		}
	}

	cachedTransport := transport
	if cache != gocache.NoCache {
		cachedTransport = &httpcache.Transport{
			Transport: transport,
			Cache:     gocache.WrapToHTTPCache(gocache.NewCacheWithNamespace(cache, "http@")),
		}
	}

	cr = &Cluster{
		host:          host,
		publicPort:    publicPort,
		clusterId:     clusterId,
		clusterSecret: clusterSecret,
		prefix:        prefix,
		byoc:          byoc,
		jwtIssuer:     jwtIssuerPrefix + "#" + clusterId,

		dataDir:     filepath.Join(baseDir, "data"),
		maxConn:     config.DownloadMaxConn,
		storageOpts: storageOpts,
		cache:       cache,

		disabled: make(chan struct{}, 0),

		downloading: make(map[string]chan error),

		client: &http.Client{
			Transport: transport,
		},
		cachedCli: &http.Client{
			Transport: cachedTransport,
		},
		tokens: NewTokenStorage(),

		wsUpgrader: &websocket.Upgrader{
			HandshakeTimeout: time.Minute,
		},
	}
	close(cr.disabled)

	cr.bufSlots = limited.NewBufSlots(cr.maxConn)

	{
		var (
			n   uint = 0
			wgs      = make([]uint, len(storageOpts))
			sts      = make([]storage.Storage, len(storageOpts))
		)
		for i, s := range storageOpts {
			sts[i] = storage.NewStorage(s)
			wgs[i] = s.Weight
			n += s.Weight
		}
		cr.storages = sts
		cr.storageWeights = wgs
		cr.storageTotalWeight = n
	}
	return
}

func (cr *Cluster) Init(ctx context.Context) (err error) {
	// Init storages
	vctx := context.WithValue(ctx, storage.ClusterCacheCtxKey, cr.cache)
	for _, s := range cr.storages {
		s.Init(vctx)
	}
	// create data folder
	os.MkdirAll(cr.dataDir, 0755)
	// read old stats
	if err := cr.stats.Load(cr.dataDir); err != nil {
		log.Errorf("Could not load stats: %v", err)
	}
	if cr.apiHmacKey, err = loadOrCreateHmacKey(cr.dataDir); err != nil {
		return fmt.Errorf("Cannot load hmac key: %w", err)
	}
	return
}

func (cr *Cluster) allocBuf(ctx context.Context) (slotId int, buf []byte, free func()) {
	return cr.bufSlots.Alloc(ctx)
}

func (cr *Cluster) Connect(ctx context.Context) bool {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if cr.socket != nil {
		log.Debug("Extra connect")
		return true
	}

	engio, err := engine.NewSocket(engine.Options{
		Host: cr.prefix,
		Path: "/socket.io/",
		ExtraHeaders: http.Header{
			"Origin":     {cr.prefix},
			"User-Agent": {build.ClusterUserAgent},
		},
		DialTimeout: time.Minute * 6,
	})
	if err != nil {
		log.Errorf("Could not parse Engine.IO options: %v; exit.", err)
		os.Exit(1)
	}

	cr.reconnectCount = 0

	if config.Advanced.SocketIOLog {
		engio.OnRecv(func(_ *engine.Socket, data []byte) {
			log.Debugf("Engine.IO recv: %q", (string)(data))
		})
		engio.OnSend(func(_ *engine.Socket, data []byte) {
			log.Debugf("Engine.IO sending: %q", (string)(data))
		})
	}
	engio.OnConnect(func(*engine.Socket) {
		log.Info("Engine.IO connected")
	})
	engio.OnDisconnect(func(_ *engine.Socket, err error) {
		if ctx.Err() != nil {
			// Ignore if the error is because context cancelled
			return
		}
		if err != nil {
			log.Warnf("Engine.IO disconnected: %v", err)
		}
		if config.Advanced.ExitWhenDisconnected {
			if cr.shouldEnable.Load() {
				log.Errorf("Cluster disconnected from remote; exit.")
				os.Exit(0x08)
			}
		}
		go cr.disconnected()
	})
	engio.OnDialError(func(_ *engine.Socket, err error) {
		const maxReconnectCount = 8
		cr.reconnectCount++
		log.Errorf("Failed to connect to the center server (%d/%d): %v", cr.reconnectCount, maxReconnectCount, err)
		if cr.reconnectCount >= maxReconnectCount {
			if cr.shouldEnable.Load() {
				log.Errorf("Cluster failed to connect too much times; exit.")
				os.Exit(0x08)
			}
		}
	})

	cr.socket = socket.NewSocket(engio, socket.WithAuthTokenFn(func() string {
		token, err := cr.GetAuthToken(ctx)
		if err != nil {
			log.Errorf("Cannot get auth token: %v; exit.", err)
			os.Exit(2)
		}
		return token
	}))
	cr.socket.OnBeforeConnect(func(*socket.Socket) {
		log.Info("Preparing to connect to center server")
	})
	cr.socket.OnConnect(func(*socket.Socket, string) {
		log.Debugf("shouldEnable is %v", cr.shouldEnable.Load())
		if cr.shouldEnable.Load() {
			if err := cr.Enable(ctx); err != nil {
				log.Errorf("Cannot enable cluster: %v; exit.", err)
				os.Exit(0x08)
			}
		}
		cr.reconnectCount = 0
	})
	cr.socket.OnDisconnect(func(*socket.Socket, string) {
		go cr.disconnected()
	})
	cr.socket.OnError(func(_ *socket.Socket, err error) {
		if ctx.Err() != nil {
			// Ignore if the error is because context cancelled
			return
		}
		log.Errorf("Socket.IO error: %v", err)
	})
	cr.socket.OnMessage(func(event string, data []any) {
		if event == "message" {
			log.Infof("[remote]: %v", data[0])
		}
	})
	log.Infof("Dialing %s", engio.URL().String())
	if err := engio.Dial(ctx); err != nil {
		log.Errorf("Dial error: %v", err)
		return false
	}
	log.Info("Connecting to socket.io namespace")
	if err := cr.socket.Connect(""); err != nil {
		log.Errorf("Open namespace error: %v", err)
		return false
	}
	return true
}

func (cr *Cluster) WaitForEnable() <-chan struct{} {
	if cr.enabled.Load() {
		return closedCh
	}

	cr.mux.Lock()
	defer cr.mux.Unlock()

	if cr.enabled.Load() {
		return closedCh
	}
	ch := make(chan struct{}, 0)
	cr.waitEnable = append(cr.waitEnable, ch)
	return ch
}

func (cr *Cluster) Enable(ctx context.Context) (err error) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if cr.enabled.Load() {
		log.Debug("Extra enable")
		return
	}

	if cr.socket != nil && !cr.socket.IO().Connected() && config.Advanced.ExitWhenDisconnected {
		log.Errorf("Cluster disconnected from remote; exit.")
		os.Exit(0x08)
		return
	}

	cr.shouldEnable.Store(true)

	log.Info("Sending enable packet")
	resCh, err := cr.socket.EmitWithAck("enable", Map{
		"host":         cr.host,
		"port":         cr.publicPort,
		"version":      build.ClusterVersion,
		"byoc":         cr.byoc,
		"noFastEnable": config.Advanced.NoFastEnable,
	})
	if err != nil {
		return
	}
	var data []any
	tctx, cancel := context.WithTimeout(ctx, time.Minute*6)
	select {
	case <-tctx.Done():
		cancel()
		return tctx.Err()
	case data = <-resCh:
		cancel()
	}
	log.Debug("got enable ack:", data)
	if ero := data[0]; ero != nil {
		return fmt.Errorf("Enable failed: %v", ero)
	}
	if !data[1].(bool) {
		return errors.New("Enable ack non true value")
	}
	log.Info("Cluster enabled")
	cr.disabled = make(chan struct{}, 0)
	cr.enabled.Store(true)
	for _, ch := range cr.waitEnable {
		close(ch)
	}
	cr.waitEnable = cr.waitEnable[:0]

	var keepaliveCtx context.Context
	keepaliveCtx, cr.cancelKeepalive = context.WithCancel(ctx)
	createInterval(keepaliveCtx, func() {
		tctx, cancel := context.WithTimeout(keepaliveCtx, KeepAliveInterval/2)
		ok := cr.KeepAlive(tctx)
		cancel()
		if !ok {
			if keepaliveCtx.Err() == nil {
				log.Info("Reconnecting due to keepalive failed")
				cr.disable(ctx)
				log.Info("Reconnecting ...")
				if !cr.Connect(ctx) {
					log.Error("Cannot reconnect to server, exit.")
					os.Exit(1)
				}
				if err := cr.Enable(ctx); err != nil {
					log.Error("Cannot enable cluster:", err, "; exit.")
					os.Exit(1)
				}
			}
		}
	}, KeepAliveInterval)
	return
}

// KeepAlive will fresh hits & hit bytes data and send the keep-alive packet
func (cr *Cluster) KeepAlive(ctx context.Context) (ok bool) {
	hits, hbts := cr.hits.Swap(0), cr.hbts.Swap(0)
	cr.stats.AddHits(hits, hbts)
	resCh, err := cr.socket.EmitWithAck("keep-alive", Map{
		"time":  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"hits":  hits,
		"bytes": hbts,
	})
	if e := cr.stats.Save(cr.dataDir); e != nil {
		log.Error("Error when saving status:", e)
	}
	if err != nil {
		log.Error("Error when keep-alive:", err)
		return false
	}
	var data []any
	select {
	case <-ctx.Done():
		return false
	case data = <-resCh:
	}
	if ero := data[0]; len(data) <= 1 || ero != nil {
		log.Error("Keep-alive failed:", ero)
		return false
	}
	log.Info("Keep-alive success:", hits, bytesToUnit((float64)(hbts)), data[1])
	return true
}

func (cr *Cluster) disconnected() bool {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if cr.enabled.CompareAndSwap(true, false) {
		return false
	}
	if cr.cancelKeepalive != nil {
		cr.cancelKeepalive()
		cr.cancelKeepalive = nil
	}
	return true
}

func (cr *Cluster) Disable(ctx context.Context) (ok bool) {
	cr.shouldEnable.Store(false)
	return cr.disable(ctx)
}

func (cr *Cluster) disable(ctx context.Context) (ok bool) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if !cr.enabled.Load() {
		log.Debug("Extra disable")
		return false
	}
	if cr.cancelKeepalive != nil {
		cr.cancelKeepalive()
		cr.cancelKeepalive = nil
	}
	if cr.socket == nil {
		return false
	}
	log.Info("Disabling cluster")
	if resCh, err := cr.socket.EmitWithAck("disable"); err == nil {
		tctx, cancel := context.WithTimeout(ctx, time.Second*(time.Duration)(config.Advanced.KeepaliveTimeout))
		select {
		case <-tctx.Done():
			cancel()
		case data := <-resCh:
			cancel()
			log.Debug("disable ack:", data)
			if ero := data[0]; ero != nil {
				log.Errorf("Disable failed: %v", ero)
			} else if !data[1].(bool) {
				log.Error("Disable failed: acked non true value")
			} else {
				ok = true
			}
		}
	} else {
		log.Errorf("Disable failed: %v", err)
	}

	cr.enabled.Store(false)
	go cr.socket.Close()
	cr.socket = nil
	close(cr.disabled)
	log.Warn("Cluster disabled")
	return
}

func (cr *Cluster) Disabled() <-chan struct{} {
	cr.mux.RLock()
	defer cr.mux.RUnlock()
	return cr.disabled
}

func (cr *Cluster) CachedFileSize(hash string) (size int64, ok bool) {
	cr.fileMux.RLock()
	defer cr.fileMux.RUnlock()
	size, ok = cr.fileset[hash]
	return
}

type CertKeyPair struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func (cr *Cluster) RequestCert(ctx context.Context) (ckp *CertKeyPair, err error) {
	log.Info("Requesting certificates, please wait ...")
	resCh, err := cr.socket.EmitWithAck("request-cert")
	if err != nil {
		return
	}
	var data []any
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case data = <-resCh:
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
	log.Info("Certificate requested")
	return
}

func (cr *Cluster) makeReq(ctx context.Context, method string, relpath string, query url.Values) (req *http.Request, err error) {
	return cr.makeReqWithBody(ctx, method, relpath, query, nil)
}

func (cr *Cluster) makeReqWithBody(
	ctx context.Context,
	method string, relpath string,
	query url.Values, body io.Reader,
) (req *http.Request, err error) {
	var u *url.URL
	if u, err = url.Parse(cr.prefix); err != nil {
		return
	}
	u.Path = path.Join(u.Path, relpath)
	if query != nil {
		u.RawQuery = query.Encode()
	}
	target := u.String()

	req, err = http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", build.ClusterUserAgent)
	return
}

func (cr *Cluster) makeReqWithAuth(ctx context.Context, method string, relpath string, query url.Values) (req *http.Request, err error) {
	req, err = cr.makeReq(ctx, method, relpath, query)
	if err != nil {
		return
	}
	token, err := cr.GetAuthToken(ctx)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
	req, err := cr.makeReqWithAuth(ctx, http.MethodGet, "/openbmclapi/files", nil)
	if err != nil {
		return
	}
	res, err := cr.cachedCli.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(res.Body)
		err = fmt.Errorf("Unexpected status code: %d %s Body:\n\t%s", res.StatusCode, res.Status, (string)(data))
		return
	}
	log.Debug("Parsing filelist body ...")
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

func (cr *Cluster) GetConfig(ctx context.Context) (cfg *OpenbmclapiAgentConfig, err error) {
	req, err := cr.makeReqWithAuth(ctx, http.MethodGet, "/openbmclapi/configuration", nil)
	if err != nil {
		return
	}
	res, err := cr.cachedCli.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(res.Body)
		err = fmt.Errorf("Unexpected status code: %d %s Body:\n\t%s", res.StatusCode, res.Status, (string)(data))
		return
	}
	cfg = new(OpenbmclapiAgentConfig)
	if err = json.NewDecoder(res.Body).Decode(cfg); err != nil {
		cfg = nil
		return
	}
	return
}

type syncStats struct {
	slots  *limited.BufSlots
	noOpen bool

	totalSize          int64
	okCount, failCount atomic.Int32
	totalFiles         int

	pg       *mpb.Progress
	totalBar *mpb.Bar
	lastInc  atomic.Int64
}

func (cr *Cluster) SyncFiles(ctx context.Context, files []FileInfo, heavyCheck bool) bool {
	log.Info("Preparing to sync files...")
	if !cr.issync.CompareAndSwap(false, true) {
		log.Warn("Another sync task is running!")
		return false
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Hash < files[j].Hash })
	if cr.syncFiles(ctx, files, heavyCheck) == nil {
		fileset := make(map[string]int64, len(files))
		for _, f := range files {
			fileset[f.Hash] = f.Size
		}
		cr.fileMux.Lock()
		cr.fileset = fileset
		cr.fileMux.Unlock()
	}
	cr.issync.Store(false)

	if !config.Advanced.NoGC {
		go cr.gc()
	}

	return true
}

type fileInfoWithTargets struct {
	FileInfo
	tgMux   sync.Mutex
	targets []storage.Storage
}

func (cr *Cluster) checkFileFor(
	ctx context.Context,
	sto storage.Storage, files []FileInfo,
	heavy bool,
	missing *SyncMap[string, *fileInfoWithTargets],
	pg *mpb.Progress,
) {
	var missingCount atomic.Int32
	addMissing := func(f FileInfo) {
		missingCount.Add(1)
		if info, has := missing.GetOrSet(f.Hash, func() *fileInfoWithTargets {
			return &fileInfoWithTargets{
				FileInfo: f,
				targets:  []storage.Storage{sto},
			}
		}); has {
			info.tgMux.Lock()
			info.targets = append(info.targets, sto)
			info.tgMux.Unlock()
		}
	}

	log.Infof("Start checking files for %s, heavy = %v", sto.String(), heavy)

	var (
		checkingHashMux  sync.Mutex
		checkingHash     string
		lastCheckingHash string
		slots            *limited.BufSlots
	)

	if heavy {
		slots = limited.NewBufSlots(runtime.GOMAXPROCS(0) * 2)
	}

	bar := pg.AddBar(0,
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name("> Checking "),
			decor.Name(sto.String()),
			decor.OnCondition(
				decor.Any(func(decor.Statistics) string {
					c, l := slots.Cap(), slots.Len()
					return fmt.Sprintf(" (%d / %d)", c-l, c)
				}),
				heavy,
			),
		),
		mpb.AppendDecorators(
			decor.CountersNoUnit("%d / %d", decor.WCSyncSpaceR),
			decor.NewPercentage("%d", decor.WCSyncSpaceR),
			decor.EwmaETA(decor.ET_STYLE_GO, 30),
		),
		mpb.BarExtender((mpb.BarFillerFunc)(func(w io.Writer, _ decor.Statistics) (err error) {
			if checkingHashMux.TryLock() {
				lastCheckingHash = checkingHash
				checkingHashMux.Unlock()
			}
			if lastCheckingHash != "" {
				_, err = fmt.Fprintln(w, "\t", lastCheckingHash)
			}
			return
		}), false),
	)
	defer bar.Wait()
	defer bar.Abort(true)

	bar.SetTotal(0x100, false)

	sizeMap := make(map[string]int64, len(files))
	{
		start := time.Now()
		var checkedMp [256]bool
		sto.WalkDir(func(hash string, size int64) error {
			if n := utils.HexTo256(hash); !checkedMp[n] {
				checkedMp[n] = true
				now := time.Now()
				bar.EwmaIncrement(now.Sub(start))
				start = now
			}
			sizeMap[hash] = size
			return nil
		})
	}

	bar.SetCurrent(0)
	bar.SetTotal((int64)(len(files)), false)
	for _, f := range files {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		hash := f.Hash
		if checkingHashMux.TryLock() {
			checkingHash = hash
			checkingHashMux.Unlock()
		}
		if f.Size == 0 {
			log.Debugf("Skipped empty file %s", hash)
		} else if size, ok := sizeMap[hash]; ok {
			if size != f.Size {
				log.Warnf("Found modified file: size of %q is %d, expect %d", hash, size, f.Size)
				addMissing(f)
			} else if heavy {
				hashMethod, err := getHashMethod(len(hash))
				if err != nil {
					log.Errorf("Unknown hash method for %q", hash)
				} else {
					_, buf, free := slots.Alloc(ctx)
					if buf == nil {
						return
					}
					go func(f FileInfo, buf []byte, free func()) {
						defer free()
						miss := true
						r, err := sto.Open(hash)
						if err != nil {
							log.Errorf("Could not open %q: %v", hash, err)
						} else {
							hw := hashMethod.New()
							_, err = io.CopyBuffer(hw, r, buf[:])
							r.Close()
							if err != nil {
								log.Errorf("Could not calculate hash for %s: %v", hash, err)
							} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != hash {
								log.Warnf("Found modified file: hash of %s became %s", hash, hs)
							} else {
								miss = false
							}
						}
						if miss {
							addMissing(f)
						}
						bar.EwmaIncrement(time.Since(start))
					}(f, buf, free)
					continue
				}
			}
		} else {
			log.Debugf("Could not found file %q", hash)
			addMissing(f)
		}
		bar.EwmaIncrement(time.Since(start))
	}

	checkingHashMux.Lock()
	checkingHash = ""
	checkingHashMux.Unlock()

	bar.SetTotal(-1, true)
	log.Infof("File check finished for %s, missing %d files", sto.String(), missingCount.Load())
	return
}

func (cr *Cluster) syncFiles(ctx context.Context, files []FileInfo, heavyCheck bool) error {
	pg := mpb.New(mpb.WithRefreshRate(time.Second), mpb.WithAutoRefresh(), mpb.WithWidth(140))
	defer pg.Shutdown()
	log.SetLogOutput(pg)
	defer log.SetLogOutput(nil)

	cr.syncProg.Store(0)
	cr.syncTotal.Store(-1)

	missingMap := NewSyncMap[string, *fileInfoWithTargets]()
	done := make(chan struct{}, 0)

	for _, s := range cr.storages {
		go func(s storage.Storage) {
			defer func() {
				select {
				case done <- struct{}{}:
				case <-ctx.Done():
				}
			}()
			cr.checkFileFor(ctx, s, files, heavyCheck, missingMap, pg)
		}(s)
	}
	for i := len(cr.storages); i > 0; i-- {
		select {
		case <-done:
		case <-ctx.Done():
			log.Warn("File sync interrupted")
			return ctx.Err()
		}
	}

	totalFiles := len(missingMap.m)
	if totalFiles == 0 {
		log.Info("All files were synchronized")
		return nil
	}

	cr.syncTotal.Store((int64)(totalFiles))

	ccfg, err := cr.GetConfig(ctx)
	if err != nil {
		return err
	}
	syncCfg := ccfg.Sync
	log.Infof("Sync config: %#v", syncCfg)

	missing := make([]*fileInfoWithTargets, 0, len(missingMap.m))
	for _, f := range missingMap.m {
		missing = append(missing, f)
	}

	var stats syncStats
	stats.pg = pg
	stats.noOpen = config.Advanced.NoOpen || syncCfg.Source == "center"
	stats.slots = limited.NewBufSlots(syncCfg.Concurrency)
	stats.totalFiles = totalFiles
	for _, f := range missing {
		stats.totalSize += f.Size
	}

	var barUnit decor.SizeB1024
	stats.lastInc.Store(time.Now().UnixNano())
	stats.totalBar = pg.AddBar(stats.totalSize,
		mpb.BarRemoveOnComplete(),
		mpb.BarPriority(stats.slots.Cap()),
		mpb.PrependDecorators(
			decor.Name("Total: "),
			decor.NewPercentage("%.2f"),
		),
		mpb.AppendDecorators(
			decor.Any(func(decor.Statistics) string {
				return fmt.Sprintf("(%d + %d / %d) ", stats.okCount.Load(), stats.failCount.Load(), stats.totalFiles)
			}),
			decor.Counters(barUnit, "(%.1f/%.1f) "),
			decor.EwmaSpeed(barUnit, "%.1f ", 30),
			decor.OnComplete(
				decor.EwmaETA(decor.ET_STYLE_GO, 30), "done",
			),
		),
	)

	log.Infof("Starting sync files, count: %d, total: %s", totalFiles, bytesToUnit((float64)(stats.totalSize)))
	start := time.Now()

	for _, f := range missing {
		log.Debugf("File %s is for %v", f.Hash, f.targets)
		pathRes, err := cr.fetchFile(ctx, &stats, f.FileInfo)
		if err != nil {
			log.Warn("File sync interrupted")
			return err
		}
		go func(f *fileInfoWithTargets, pathRes <-chan string) {
			defer func() {
				select {
				case done <- struct{}{}:
				case <-ctx.Done():
				}
			}()
			select {
			case path := <-pathRes:
				cr.syncProg.Add(1)
				if path != "" {
					defer os.Remove(path)
					// acquire slot here
					slotId, buf, free := stats.slots.Alloc(ctx)
					if buf == nil {
						return
					}
					defer free()
					_ = slotId
					var srcFd *os.File
					if srcFd, err = os.Open(path); err != nil {
						return
					}
					defer srcFd.Close()
					for _, target := range f.targets {
						if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
							log.Errorf("Could not seek file %q to start: %v", path, err)
							continue
						}
						err := target.Create(f.Hash, srcFd)
						if err != nil {
							log.Errorf("Could not create %s/%s: %v", target.String(), f.Hash, err)
							continue
						}
					}
				}
			case <-ctx.Done():
				return
			}
		}(f, pathRes)
	}
	for i := len(missing); i > 0; i-- {
		select {
		case <-done:
		case <-ctx.Done():
			log.Warn("File sync interrupted")
			return ctx.Err()
		}
	}

	use := time.Since(start)
	pg.Wait()

	log.Infof("All files were synchronized, use time: %v, %s/s", use, bytesToUnit((float64)(stats.totalSize)/use.Seconds()))
	return nil
}

func (cr *Cluster) gc() {
	for _, s := range cr.storages {
		cr.gcFor(s)
	}
}

func (cr *Cluster) gcFor(s storage.Storage) {
	log.Info("Starting garbage collector for", s.String())
	err := s.WalkDir(func(hash string, _ int64) error {
		if cr.issync.Load() {
			return context.Canceled
		}
		if _, ok := cr.CachedFileSize(hash); !ok {
			log.Info("Found outdated file:", hash)
			s.Remove(hash)
		}
		return nil
	})
	if err != nil {
		if err == context.Canceled {
			log.Warn("Garbage collector interrupted at", s.String())
		} else {
			log.Errorf("Garbage collector error: %v", err)
		}
		return
	}
	log.Info("Garbage collect finished for", s.String())
}

func (cr *Cluster) fetchFile(ctx context.Context, stats *syncStats, f FileInfo) (<-chan string, error) {
	const (
		maxRetryCount  = 5
		maxTryWithOpen = 3
	)

	slotId, buf, free := stats.slots.Alloc(ctx)
	if buf == nil {
		return nil, ctx.Err()
	}

	pathRes := make(chan string, 1)
	go func() {
		defer free()
		defer close(pathRes)

		var barUnit decor.SizeB1024
		var trycount atomic.Int32
		trycount.Store(1)
		bar := stats.pg.AddBar(f.Size,
			mpb.BarRemoveOnComplete(),
			mpb.BarPriority(slotId),
			mpb.PrependDecorators(
				decor.Name("> Downloading "),
				decor.Any(func(decor.Statistics) string {
					tc := trycount.Load()
					if tc <= 1 {
						return ""
					}
					return fmt.Sprintf("(%d/%d) ", tc, maxRetryCount)
				}),
				decor.Name(f.Path, decor.WCSyncSpaceR),
			),
			mpb.AppendDecorators(
				decor.NewPercentage("%d", decor.WCSyncSpace),
				decor.Counters(barUnit, "[%.1f / %.1f]", decor.WCSyncSpace),
				decor.EwmaSpeed(barUnit, "%.1f", 10, decor.WCSyncSpace),
				decor.OnComplete(
					decor.EwmaETA(decor.ET_STYLE_GO, 10, decor.WCSyncSpace), "done",
				),
			),
		)
		defer bar.Abort(true)

		noOpen := stats.noOpen
		interval := time.Second
		for {
			bar.SetCurrent(0)
			hashMethod, err := getHashMethod(len(f.Hash))
			if err == nil {
				var path string
				if path, err = cr.fetchFileWithBuf(ctx, f, hashMethod, buf, noOpen, func(r io.Reader) io.Reader {
					return ProxyReader(r, bar, stats.totalBar, &stats.lastInc)
				}); err == nil {
					pathRes <- path
					stats.okCount.Add(1)
					log.Infof("Downloaded %s [%s] %.2f%%", f.Path,
						bytesToUnit((float64)(f.Size)),
						(float64)(stats.totalBar.Current())/(float64)(stats.totalSize)*100)
					return
				}
			}
			bar.SetRefill(bar.Current())

			log.Errorf("Download error %s:\n\t%s", f.Path, err)
			c := trycount.Add(1)
			if c > maxRetryCount {
				break
			}
			if c > maxTryWithOpen {
				noOpen = true
			}
			select {
			case <-time.After(interval):
				interval *= 2
			case <-ctx.Done():
				return
			}
		}
		stats.failCount.Add(1)
	}()
	return pathRes, nil
}

var noOpenQuery = url.Values{
	"noopen": {"1"},
}

func (cr *Cluster) fetchFileWithBuf(
	ctx context.Context, f FileInfo,
	hashMethod crypto.Hash, buf []byte,
	noOpen bool,
	wrapper func(io.Reader) io.Reader,
) (path string, err error) {
	var (
		query url.Values = nil
		req   *http.Request
		res   *http.Response
		fd    *os.File
		r     io.Reader
	)
	if noOpen {
		query = noOpenQuery
	}
	if req, err = cr.makeReqWithAuth(ctx, http.MethodGet, f.Path, query); err != nil {
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
		err = utils.NewHTTPStatusErrorFromResponse(res)
		return
	}
	switch ce := strings.ToLower(res.Header.Get("Content-Encoding")); ce {
	case "":
		r = res.Body
	case "gzip":
		if r, err = gzip.NewReader(res.Body); err != nil {
			return
		}
	case "deflate":
		if r, err = zlib.NewReader(res.Body); err != nil {
			return
		}
	default:
		err = fmt.Errorf("Unexpected Content-Encoding %q", ce)
		return
	}
	if wrapper != nil {
		r = wrapper(r)
	}

	hw := hashMethod.New()

	if fd, err = os.CreateTemp("", "*.downloading"); err != nil {
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

func (cr *Cluster) DownloadFile(ctx context.Context, hash string) (err error) {
	hashMethod, err := getHashMethod(len(hash))
	if err != nil {
		return
	}

	var buf []byte
	_, buf, free := cr.allocBuf(ctx)
	if buf == nil {
		return ctx.Err()
	}
	defer free()

	f := FileInfo{
		Path: "/openbmclapi/download/" + hash,
		Hash: hash,
		Size: -1,
	}
	done, ok := cr.lockDownloading(hash)
	if ok {
		select {
		case err = <-done:
		case <-cr.Disabled():
			err = context.Canceled
		}
		return
	}
	defer func() {
		done <- err
	}()

	log.Infof("Downloading %s from handler", hash)
	defer func() {
		if err != nil {
			log.Errorf("Could not download %s: %v", hash, err)
		}
	}()

	path, err := cr.fetchFileWithBuf(ctx, f, hashMethod, buf, true, nil)
	if err != nil {
		return
	}
	defer os.Remove(path)
	var srcFd *os.File
	if srcFd, err = os.Open(path); err != nil {
		return
	}
	defer srcFd.Close()
	var stat os.FileInfo
	if stat, err = srcFd.Stat(); err != nil {
		return
	}
	size := stat.Size()

	for _, target := range cr.storages {
		if _, err = srcFd.Seek(0, io.SeekStart); err != nil {
			log.Errorf("Could not seek file %q: %v", path, err)
			return
		}
		if err := target.Create(hash, srcFd); err != nil {
			log.Errorf("Could not create %q: %v", target.String(), err)
			continue
		}
	}

	cr.fileMux.Lock()
	cr.fileset[hash] = size
	cr.fileMux.Unlock()
	return
}
