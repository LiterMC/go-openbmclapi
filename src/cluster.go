package main

import (
	"context"
	"crypto"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
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

	cachedir string
	hits     atomic.Int32
	hbytes   atomic.Int64
	maxConn  int
	issync   bool

	mux         sync.Mutex
	enabled     bool
	socket      *Socket
	keepalive   context.CancelFunc
	downloading map[string]chan struct{}

	client *http.Client
	Server *http.Server
}

func NewCluster(
	host string, publicPort uint16,
	username string, password string,
	version string, address string) (cr *Cluster) {
	cr = &Cluster{
		host:       host,
		publicPort: publicPort,
		username:   username,
		password:   password,
		version:    version,
		useragent:  "openbmclapi-cluster/" + version,
		prefix:     "https://openbmclapi.bangbang93.com",

		cachedir: "cache",
		maxConn:  400,
		issync:   false,

		client: &http.Client{
			Timeout: time.Second * 60,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					// InsecureSkipVerify: true, // Skip verify because the author was lazy
				},
			},
		},
		Server: &http.Server{
			Addr: address,
		},
	}
	cr.Server.Handler = cr
	return
}

func (cr *Cluster) Enable() bool {
	cr.mux.Lock()
	defer cr.mux.Unlock()
	if cr.socket != nil {
		logDebug("Extra enable")
		return true
	}
	var (
		err error
		res *http.Response
	)
	logInfo("Enabling cluster")
	wsurl := httpToWs(cr.prefix) +
		fmt.Sprintf("/socket.io/?clusterId=%s&clusterSecret=%s&EIO=4&transport=websocket", cr.username, cr.password)
	logDebug("Websocket url:", wsurl)
	header := http.Header{}
	header.Set("Origin", cr.prefix)

	cr.socket = NewSocket(NewESocket())
	cr.socket.ConnectHandle = func(*Socket) {
		cr._enable()
	}
	cr.socket.DisconnectHandle = func(*Socket) {
		cr.Disable()
	}
	cr.socket.GetIO().ErrorHandle = func(*ESocket) {
		go func() {
			cr.enabled = false
			ch := cr.Disable()
			if ch != nil {
				<-ch
			}
			if !cr.Enable() {
				logError("Cannot reconnect to server, exit.")
				panic("Cannot reconnect to server, exit.")
			}
		}()
	}
	err = cr.socket.GetIO().Dial(wsurl, header)
	if err != nil {
		logError("Connect websocket error:", err, res)
		return false
	}
	return true
}

func (cr *Cluster) _enable() {
	cr.socket.EmitAck(func(_ uint64, data json.JsonArr) {
		data = data.GetArray(0)
		if len(data) < 2 || !data.GetBool(1) {
			logError("Enable failed: " + data.String())
			panic("Enable failed: " + data.String())
		}
		logInfo("get enable ack:", data)
		cr.enabled = true

		var keepaliveCtx context.Context
		keepaliveCtx, cr.keepalive = context.WithCancel(context.TODO())
		createInterval(keepaliveCtx, func() {
			cr.KeepAlive()
		}, KeepAliveInterval)
	}, "enable", json.JsonObj{
		"host":    cr.host,
		"port":    cr.publicPort,
		"version": cr.version,
	})
}

func (cr *Cluster) KeepAlive() (ok bool) {
	hits, hbytes := cr.hits.Swap(0), cr.hbytes.Swap(0)
	err := cr.socket.EmitAck(func(_ uint64, data json.JsonArr) {
		data = data.GetArray(0)
		if len(data) > 1 && data.Get(0) == nil {
			logInfo("Keep-alive success:", hits, bytesToUnit((float64)(hbytes)), data)
		} else {
			logInfo("Keep-alive failed:", data.Get(0))
			cr.Disable()
			if !cr.Enable() {
				logError("Cannot reconnect to server, exit.")
				panic("Cannot reconnect to server, exit.")
			}
		}
	}, "keep-alive", json.JsonObj{
		"time":  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"hits":  hits,
		"bytes": hbytes,
	})
	if err != nil {
		logError("Error when keep-alive:", err)
		return false
	}
	return true
}

func (cr *Cluster) Disable() (done <-chan struct{}) {
	cr.mux.Lock()
	defer cr.mux.Unlock()

	if !cr.enabled {
		logDebug("Extra disable")
		return nil
	}
	logInfo("Disabling cluster")
	if cr.keepalive != nil {
		cr.keepalive()
		cr.keepalive = nil
	}
	if cr.socket == nil {
		return nil
	}
	sch := make(chan struct{}, 1)
	err := cr.socket.EmitAck(func(_ uint64, data json.JsonArr) {
		data = data.GetArray(0)
		logInfo("disable ack:", data)
		if !data.GetBool(1) {
			logError("Disable failed: " + data.String())
			panic("Disable failed: " + data.String())
		}
		sch <- struct{}{}
	}, "disable")
	cr.enabled = false
	cr.socket.Close()
	cr.socket = nil
	if err != nil {
		return nil
	}
	return sch
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
	return ufile.JoinPath(cr.cachedir, hash[:2], hash)
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
    "fields": [
      {"name": "path", "type": "string"},
      {"name": "hash", "type": "string"},
      {"name": "size", "type": "long"},
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
		return nil
	}
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
	logInfo("Pre sync files...")
	if cr.issync {
		logWarn("Another sync task is running!")
		return
	}
	cr.issync = true
	defer func() {
		cr.issync = false
	}()

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
	cr.issync = false
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
			logWarn("At least one file has changed during file synchronization, re synchronize now.")
			cr.SyncFiles(files2, ctx)
			return
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
			logErrorf("Download file error: %v %s [%s/%s;%d/%d]%.2f%%",
				err, f.Path,
				bytesToUnit(stats.downloaded), bytesToUnit(stats.totalsize),
				stats.fcount.Load(), stats.fl,
				stats.downloaded/stats.totalsize*100)
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
			logInfof("Downloaded: %s [%s/%s;%d/%d]%.2f%%", f.Path,
				bytesToUnit(stats.downloaded), bytesToUnit(stats.totalsize),
				stats.fcount.Load(), stats.fl,
				stats.downloaded/stats.totalsize*100)
		}
	}()
}

func (cr *Cluster) dlhandle(ctx context.Context, f *FileInfo) (err error) {
	logDebug("Downloading:", f.Path)
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
		cr.downloadFileBuf(ctx, f, hashMethod, buf)
	}
	return
}

func (cr *Cluster) CheckFiles(files []FileInfo, notf []FileInfo) []FileInfo {
	logInfo("Starting check files")
	for i, _ := range files {
		if cr.issync && notf == nil {
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
	stack = append(stack, cr.cachedir)
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fil, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, f := range fil {
			if cr.issync {
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

	if fd, err = os.Create(cr.getHashPath(f.Hash)); err != nil {
		return
	}

	var t int64
	t, err = io.CopyBuffer(io.MultiWriter(hw, fd), res.Body, buf)
	fd.Close()
	if err != nil {
		return
	}
	if f.Size >= 0 && t != f.Size {
		err = fmt.Errorf("File size wrong, got %s, expect %s", bytesToUnit((float64)(t)), bytesToUnit((float64)(f.Size)))
	} else if hs := hex.EncodeToString(hw.Sum(buf[:0])); hs != f.Hash {
		err = fmt.Errorf("File hash not match, got %s, expect %s", hs, f.Hash)
	}
	if DEBUG && err != nil {
		f0, _ := os.Open(cr.getHashPath(f.Hash))
		b0, _ := io.ReadAll(f0)
		if len(b0) < 16*1024 {
			logDebug("File content:", (string)(b0), "//for", f.Path)
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
