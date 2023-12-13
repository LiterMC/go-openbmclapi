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

	ufile "github.com/KpnmServer/go-util/file"
	json "github.com/KpnmServer/go-util/json"
	"github.com/hamba/avro/v2"
	"github.com/klauspost/compress/zstd"
)

type Cluster struct {
	host       string
	publicPort uint16
	username   string
	password   string
	version    string
	useragent  string
	prefix     string
	byoc       bool

	redirectBase string

	cacheDir string
	tmpDir   string
	maxConn  int
	hits     atomic.Int32
	hbytes   atomic.Int64
	issync   atomic.Bool

	ctx         context.Context
	mux         sync.Mutex
	enabled     bool
	socket      *Socket
	keepalive   context.CancelFunc
	downloading map[string]chan struct{}

	client *http.Client
	Server *http.Server
}

func NewCluster(
	ctx context.Context, cacheDir string,
	host string, publicPort uint16,
	username string, password string,
	version string, address string,
	byoc bool, dialer *net.Dialer,
	redirectBase string,
) (cr *Cluster) {
	transport := &http.Transport{}
	if dialer != nil {
		transport.DialContext = dialer.DialContext
	}
	cr = &Cluster{
		ctx: ctx,

		host:       host,
		publicPort: publicPort,
		username:   username,
		password:   password,
		version:    version,
		useragent:  "openbmclapi-cluster/" + version,
		prefix:     "https://openbmclapi.bangbang93.com",
		byoc:       byoc,

		redirectBase: redirectBase,

		cacheDir: cacheDir,
		tmpDir:   filepath.Join(cacheDir, ".tmp"),
		maxConn:  128,

		client: &http.Client{
			Transport: transport,
		},
		Server: &http.Server{
			Addr: address,
		},
	}
	cr.Server.Handler = cr
	os.MkdirAll(cr.cacheDir, 0755)
	os.RemoveAll(cr.tmpDir)
	os.Mkdir(cr.tmpDir, 0700)
	return
}

func (cr *Cluster) Connect() bool {
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

	cr.socket = NewSocket(cr.ctx, NewESocket())
	cr.socket.ConnectHandle = func(*Socket) {
		connected()
	}
	cr.socket.DisconnectHandle = func(*Socket) {
		connected()
		cr.Disable()
	}
	cr.socket.ErrorHandle = func(*Socket) {
		connected()
		go func() {
			cr.Disable()
			if !cr.Connect() {
				logError("Cannot reconnect to server, exit.")
				os.Exit(1)
			}
			if err := cr.Enable(); err != nil {
				logError("Cannot enable cluster:", err, "; exit.")
				os.Exit(1)
			}
		}()
	}
	logInfof("Dialing %s", strings.ReplaceAll(wsurl, cr.password, "<******>"))
	err := cr.socket.GetIO().Dial(wsurl, header)
	if err != nil {
		logError("Websocket connect error:", err)
		return false
	}
	select {
	case <-cr.ctx.Done():
		return false
	case <-connectCh:
	}
	return true
}

func (cr *Cluster) Enable() (err error) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if cr.enabled {
		logDebug("Extra enable")
		return
	}
	logInfo("Sending enable packet")
	data, err := cr.socket.EmitAck("enable", json.JsonObj{
		"host":    cr.host,
		"port":    cr.publicPort,
		"version": cr.version,
		"byoc":    cr.byoc,
	})
	if err != nil {
		return
	}
	if len(data) < 2 || !data.GetBool(1) {
		return errors.New(data.String())
	}
	logInfo("get enable ack:", data)
	cr.enabled = true

	var keepaliveCtx context.Context
	keepaliveCtx, cr.keepalive = context.WithCancel(context.TODO())
	createInterval(keepaliveCtx, func() {
		cr.KeepAlive()
	}, KeepAliveInterval)
	return
}

func (cr *Cluster) KeepAlive() (ok bool) {
	hits, hbytes := cr.hits.Swap(0), cr.hbytes.Swap(0)
	data, err := cr.socket.EmitAck("keep-alive", json.JsonObj{
		"time":  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"hits":  hits,
		"bytes": hbytes,
	})
	if err != nil {
		logError("Error when keep-alive:", err)
		return false
	}
	if len(data) > 1 && data.Get(0) == nil {
		logInfo("Keep-alive success:", hits, bytesToUnit((float64)(hbytes)), data)
	} else {
		logInfo("Keep-alive failed:", data.Get(0))
		cr.Disable()
		if !cr.Connect() {
			logError("Cannot reconnect to server, exit.")
			os.Exit(1)
		}
		if err := cr.Enable(); err != nil {
			logError("Cannot enable cluster:", err, "; exit.")
			os.Exit(1)
		}
	}
	return true
}

func (cr *Cluster) Disable() (successed bool) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if !cr.enabled {
		logDebug("Extra disable")
		return true
	}
	logInfo("Disabling cluster")
	if cr.keepalive != nil {
		cr.keepalive()
		cr.keepalive = nil
	}
	if cr.socket == nil {
		return true
	}
	data, err := cr.socket.EmitAck("disable")
	cr.enabled = false
	cr.socket.Close()
	cr.socket = nil
	if err != nil {
		return false
	}
	logInfo("disable ack:", data)
	if !data.GetBool(1) {
		logError("Disable failed: " + data.String())
		return false
	}
	return true
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

func (cr *Cluster) RequestCert() (ckp *CertKeyPair, err error) {
	logInfo("Requesting cert, please wait ...")
	data, err := cr.socket.EmitAck("request-cert")
	if err != nil {
		return
	}
	if erv := data.Get(0); erv != nil {
		err = fmt.Errorf("socket.io remote error: %v", erv)
		return
	}
	pair := data.GetObj(1)
	ckp = &CertKeyPair{
		Cert: pair.GetString("cert"),
		Key:  pair.GetString("key"),
	}
	logInfo("Certificate requested")
	return
}

func (cr *Cluster) queryFunc(method string, url string, call func(*http.Request)) (res *http.Response, err error) {
	var req *http.Request
	req, err = http.NewRequest(method, cr.prefix+url, nil)
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

func (cr *Cluster) queryURL(method string, url string) (res *http.Response, err error) {
	return cr.queryFunc(method, url, nil)
}

func (cr *Cluster) queryURLHeader(method string, url string, header map[string]string) (res *http.Response, err error) {
	return cr.queryFunc(method, url, func(req *http.Request) {
		if header != nil {
			for k, v := range header {
				req.Header.Set(k, v)
			}
		}
	})
}

func (cr *Cluster) getHashPath(hash string) string {
	return ufile.JoinPath(cr.cacheDir, hash[:2], hash)
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

func (cr *Cluster) GetFileList() (files []FileInfo) {
	var (
		err error
		res *http.Response
	)
	res, err = cr.queryURL("GET", "/openbmclapi/files")
	if err != nil {
		logError("Query filelist error:", err)
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		logErrorf("Query filelist error: Unexpected status code: %d %s", res.StatusCode, res.Status)
		data, _ := io.ReadAll(res.Body)
		logErrorf("Response Body:\n%s", (string)(data))
		return nil
	}
	logDebug("Parsing filelist body ...")
	zr, err := zstd.NewReader(res.Body)
	if err != nil {
		logError("Parse filelist body error:", err)
		return nil
	}
	defer zr.Close() // TODO: reuse the decoder?
	if err = avro.NewDecoderForSchema(fileListSchema, zr).Decode(&files); err != nil {
		logError("Parse filelist body error:", err)
		return nil
	}
	return files
}

type extFileInfo struct {
	*FileInfo
	dlerr    error
	trycount int
}

type syncStats struct {
	totalsize  float64
	downloaded float64
	slots      chan struct{}
	fcount     atomic.Int32
	fl         int
}

func (cr *Cluster) SyncFiles(files0 []FileInfo, ctx context.Context) {
	logInfo("Preparing to sync files...")
	if !cr.issync.CompareAndSwap(false, true) {
		logWarn("Another sync task is running!")
		return
	}
	defer cr.issync.Store(false)

RESYNC:
	var files []FileInfo
	if !withContext(ctx, func() {
		files = cr.CheckFiles(files0, make([]FileInfo, 0, 16))
	}) {
		logWarn("File sync interrupted")
		return
	}

	fl := len(files)
	if fl == 0 {
		logInfo("All file was synchronized")
		go cr.gc(files0)
		return
	}

	// sort the files in descending order of size
	sort.Slice(files, func(i, j int) bool { return files[i].Size > files[j].Size })

	var stats syncStats
	stats.slots = make(chan struct{}, cr.maxConn)
	stats.fl = fl
	for i, _ := range files {
		stats.totalsize += (float64)(files[i].Size)
	}

	logInfof("Starting sync files, count: %d, total: %s", fl, bytesToUnit(stats.totalsize))
	start := time.Now()

	for i, _ := range files {
		cr.dlfile(ctx, &stats, &extFileInfo{FileInfo: &files[i], dlerr: nil, trycount: 0})
	}
	for i := len(stats.slots); i > 0; i-- {
		select {
		case stats.slots <- struct{}{}:
		case <-ctx.Done():
			logWarn("File sync interrupted")
			return
		}
	}

	use := time.Since(start)
	logInfof("All file was synchronized, use time: %v, %s/s", use, bytesToUnit(stats.totalsize/use.Seconds()))
	var flag bool = false
	if use > SyncFileInterval {
		logWarn("Synchronization time was more than 10 min, re-check now.")
		files2 := cr.GetFileList()
		if len(files2) != len(files0) {
			flag = true
		} else {
			for _, f := range files2 {
				p := cr.getHashPath(f.Hash)
				if ufile.IsNotExist(p) {
					flag = true
					break
				}
			}
		}
		if flag {
			files0 = files2
			logWarn("At least one file has changed during file synchronization, re synchronize now.")
			goto RESYNC
		}
	}
	go cr.gc(files0)
}

func (cr *Cluster) dlfile(ctx context.Context, stats *syncStats, f *extFileInfo) {
WAIT_SLOT:
	for {
		select {
		case stats.slots <- struct{}{}:
			break WAIT_SLOT
		case <-ctx.Done():
			return
		}
	}
	go func() {
		defer func() {
			<-stats.slots
		}()
	RETRY:
		err := cr.dlhandle(ctx, f.FileInfo)
		if err != nil {
			logErrorf("Download file error: %s [%s/%s ; %d/%d] %.2f%%\n\t%s",
				f.Path,
				bytesToUnit(stats.downloaded), bytesToUnit(stats.totalsize),
				stats.fcount.Load(), stats.fl,
				stats.downloaded/stats.totalsize*100,
				err)
			if f.trycount < 3 {
				f.trycount++
				goto RETRY
			}
			stats.fcount.Add(1)
			p := cr.getHashPath(f.Hash)
			if ufile.IsExist(p) {
				os.Remove(p)
			}
		} else {
			stats.downloaded += (float64)(f.Size)
			stats.fcount.Add(1)
			logInfof("Downloaded: %s [%s/%s ; %d/%d] %.2f%%", f.Path,
				bytesToUnit(stats.downloaded), bytesToUnit(stats.totalsize),
				stats.fcount.Load(), stats.fl,
				stats.downloaded/stats.totalsize*100)
		}
	}()
}

func (cr *Cluster) dlhandle(ctx context.Context, f *FileInfo) (err error) {
	logInfof("Downloading: %s [%s]", f.Path, bytesToUnit((float64)(f.Size)))
	hashMethod, err := getHashMethod(len(f.Hash))
	if err != nil {
		return
	}

	var buf []byte
	{
		buf0 := bufPool.Get().(*[]byte)
		defer bufPool.Put(buf0)
		buf = *buf0
	}

	for i := 0; i < 3; i++ {
		if err = cr.downloadFileBuf(ctx, f, hashMethod, buf); err == nil {
			return
		}
	}
	return
}

func (cr *Cluster) CheckFiles(files []FileInfo, notf []FileInfo) []FileInfo {
	logInfo("Starting check files")
	for i, _ := range files {
		if cr.issync.Load() && notf == nil {
			logWarn("File check interrupted")
			return nil
		}
		p := cr.getHashPath(files[i].Hash)
		fs, err := os.Stat(p)
		if err == nil {
			if fs.Size() != files[i].Size {
				logInfof("Found wrong size file: '%s'(%s) expect %s",
					p, bytesToUnit((float64)(fs.Size())), bytesToUnit((float64)(files[i].Size)))
				if notf == nil {
					os.Remove(p)
				} else {
					notf = append(notf, files[i])
				}
			}
		} else {
			if notf != nil {
				notf = append(notf, files[i])
			}
			if os.IsNotExist(err) {
				p = ufile.DirPath(p)
				if ufile.IsNotExist(p) {
					os.MkdirAll(p, 0755)
				}
			} else {
				os.Remove(p)
			}
		}
	}
	logInfo("File check finished")
	return notf
}

func (cr *Cluster) gc(files []FileInfo) {
	logInfo("Starting garbage collector")
	fileset := make(map[string]struct{}, 128)
	for i, _ := range files {
		fileset[cr.getHashPath(files[i].Hash)] = struct{}{}
	}
	stack := make([]string, 0, 10)
	stack = append(stack, cr.cacheDir)
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fil, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, f := range fil {
			if cr.issync.Load() {
				logWarn("Global cleanup interrupted")
				return
			}
			n := ufile.JoinPath(p, f.Name())
			if ufile.IsDir(n) {
				stack = append(stack, n)
			} else if _, ok := fileset[n]; !ok {
				logInfo("Found outdated file:", n)
				os.Remove(n)
			}
		}
	}
	logInfo("Garbage collector finished")
}

func (cr *Cluster) downloadFileBuf(ctx context.Context, f *FileInfo, hashMethod crypto.Hash, buf []byte) (err error) {
	var (
		res *http.Response
		fd  *os.File
	)
	if res, err = cr.queryURL("GET", f.Path); err != nil {
		return
	}
	defer res.Body.Close()
	if err = ctx.Err(); err != nil {
		return
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %d", res.StatusCode)
	}

	hw := hashMethod.New()

	if fd, err = os.CreateTemp(cr.tmpDir, "*.downloading"); err != nil {
		return
	}
	tfile := fd.Name()

	_, err = io.CopyBuffer(io.MultiWriter(hw, fd), res.Body, buf)
	stat, err2 := fd.Stat()
	if err2 != nil {
		return err2
	}
	fd.Close()
	if err != nil {
		return
	}
	if t := stat.Size(); f.Size >= 0 && t != f.Size {
		err = fmt.Errorf("File size wrong, got %s, expect %s", bytesToUnit((float64)(t)), bytesToUnit((float64)(f.Size)))
	} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != f.Hash {
		err = fmt.Errorf("File hash not match, got %s, expect %s", hs, f.Hash)
	}
	if err != nil {
		if config.Debug {
			f0, _ := os.Open(cr.getHashPath(f.Hash))
			b0, _ := io.ReadAll(f0)
			if len(b0) < 16*1024 {
				logDebug("File path:", tfile, "; for", f.Path)
			}
		}
		return
	}

	hspt := cr.getHashPath(f.Hash)
	if err = os.Rename(tfile, hspt); err != nil {
		return
	}

	if config.Hijack {
		if !strings.HasPrefix(f.Path, "/openbmclapi/download/") {
			target := filepath.Join(hijackPath, filepath.FromSlash(f.Path))
			dir := filepath.Dir(target)
			os.MkdirAll(dir, 0755)
			if rp, err := filepath.Rel(dir, hspt); err == nil {
				os.Symlink(rp, target)
			}
		}
	}

	return
}

func (cr *Cluster) DownloadFile(ctx context.Context, hash string) (err error) {
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
	f := &FileInfo{
		Path: "/openbmclapi/download/" + hash + "?noopen=1",
		Hash: hash,
		Size: -1,
	}
	return cr.downloadFileBuf(ctx, f, hashMethod, buf)
}
