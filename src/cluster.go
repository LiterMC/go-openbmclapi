
package main

import (
	os "os"
	io "io"
	ioutil "io/ioutil"
	time "time"
	sort "sort"
	strings "strings"
	strconv "strconv"
	fmt "fmt"
	tls "crypto/tls"
	http "net/http"

	ufile "github.com/KpnmServer/go-util/file"
	json "github.com/KpnmServer/go-util/json"
	websocket "github.com/gorilla/websocket"
)

type Cluster struct{
	host string
	public_port uint16
	username string
	password string
	version string
	useragent string
	prefix string

	cachedir string
	hits uint64
	hbytes uint64
	max_conn uint

	connecting bool
	enabled bool
	socket *websocket.Conn
	wshandlers map[string]func(id string, args json.JsonArr)
	wsid string
	keepalive func()(bool)

	client *http.Client
	Server *http.Server
}

func newCluster(
	host string, public_port uint16,
	username string, password string,
	version string, address string)(cr *Cluster){
	cr = &Cluster{
		host: host,
		public_port: public_port,
		username: username,
		password: password,
		version: version,
		useragent: "openbmclapi-cluster/" + version,
		prefix: "https://openbmclapi.bangbang93.com",

		cachedir: "cache",
		hits: 0,
		hbytes: 0,
		max_conn: 200,

		connecting: false,
		enabled: false,
		socket: nil,
		wshandlers: make(map[string]func(id string, args json.JsonArr)),
		wsid: "",

		client: &http.Client{
			Timeout: time.Second * 60,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // Skip verify because the author was lazy
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

var PACKET_TYPE0 = make(map[string]string)
var PACKET_TYPE  = make(map[string]string)
func init(){
	PACKET_TYPE0["0"] = "OPEN";
	PACKET_TYPE0["OPEN"] = "0";
	PACKET_TYPE0["1"] = "CLOSE";
	PACKET_TYPE0["CLOSE"] = "1";
	PACKET_TYPE0["2"] = "PING";
	PACKET_TYPE0["PING"] = "2";
	PACKET_TYPE0["3"] = "PONG";
	PACKET_TYPE0["PONG"] = "3";
	PACKET_TYPE0["4"] = "MESSAGE";
	PACKET_TYPE0["MESSAGE"] = "4";
	PACKET_TYPE["0"] = "CONNECT";
	PACKET_TYPE["CONNECT"] = "0";
	PACKET_TYPE["1"] = "DISCONNECT";
	PACKET_TYPE["DISCONNECT"] = "1";
	PACKET_TYPE["2"] = "EVENT";
	PACKET_TYPE["EVENT"] = "2";
	PACKET_TYPE["3"] = "ACK";
	PACKET_TYPE["ACK"] = "3";
	PACKET_TYPE["4"] = "CONNECT_ERROR";
	PACKET_TYPE["CONNECT_ERROR"] = "4";
	PACKET_TYPE["5"] = "BINARY_EVENT";
	PACKET_TYPE["BINARY_EVENT"] = "5";
	PACKET_TYPE["6"] = "BINARY_ACK";
	PACKET_TYPE["BINARY_ACK"] = "6";
}

func (cr *Cluster)runMsgHandler(id string, obj json.JsonArr){
	h, ok := cr.wshandlers[id]
	if ok {
		h(id, obj)
	}else{
		logDebug("Got ws message:", id, obj)
	}
}

func (cr *Cluster)sendMsg(id string, objs ...interface{})(error){
	jarr := (json.JsonArr)(append([]interface{}{id}, objs...))
	return cr.socket.WriteMessage(websocket.TextMessage, ([]byte)(PACKET_TYPE0["MESSAGE"] + PACKET_TYPE["EVENT"] + jarr.String()))
}

func (cr *Cluster)onWsMessage(t string, data []byte){	
	var (
		err error
		str string
		obj json.JsonObj
		arr json.JsonArr
	)
	switch t {
	case "CONNECT_ERROR":
		err = json.DecodeJson(data, &obj)
		if err == nil {
			logErrorf("Cannot connect to server:", obj)
			panic(obj)
		}else if err = json.DecodeJson(data, &str); err == nil {
			logErrorf("Cannot connect to server:", str)
			panic(str)
		}
	case "EVENT", "BINARY_EVENT":
		err = json.DecodeJson(data, &arr)
		logDebug("got event:", string(data))
		cr.runMsgHandler(arr.GetString(0), arr[1:])
	// case "ACK", "BINARY_ACK":
	// 	err = json.DecodeJson(data, &arr)
	default: return
	}
	if err != nil {
		logError("Error when decode message:", err)
	}
}

func (cr *Cluster)Enable()(bool){
	if cr.connecting {
		return true
	}
	var (
		err error
		res *http.Response
	)
	wsurl := httpToWs(cr.prefix) +
		fmt.Sprintf("/socket.io/?clusterId=%s&clusterSecret=%s&EIO=4&transport=websocket", cr.username, cr.password)
	logDebug("Websocket url:", wsurl)
	header := http.Header{}
	header.Set("Origin", cr.prefix)
	cr.socket, res, err = websocket.DefaultDialer.Dial(wsurl, header)
	if err != nil {
		cr.socket = nil
		logError("Websocket connect error:", err, res)
		return false
	}
	cr.connecting = true
	oldclose := cr.socket.CloseHandler()
	cr.socket.SetCloseHandler(func(code int, text string)(err error){
		cr.connecting = false
		if cr.keepalive != nil {
			cr.keepalive()
			cr.keepalive = nil
		}
		err = oldclose(code, text)
		if code == websocket.CloseNormalClosure {
			logWarn("Websocket disconnected")
		}else{
			logErrorf("Websocket disconnected(%d): %s", code, text)
		}
		return
	})
	go func(){
		var (
			code int
			buf []byte
			err error
			t string
			ok bool
		)
		var (
			obj json.JsonObj
		)
		for cr.connecting {
			logDebug("reading message")
			code, buf, err = cr.socket.ReadMessage()
			if err != nil {
				if !cr.connecting {
					return
				}
				cr.Disable()
				logError("Error when reading websocket:", err)
				return
			}
			logDebug("got msg:", string(buf))
			if code == websocket.TextMessage {
				if t, ok = PACKET_TYPE0[(string)(buf[0:1])]; !ok {
					logError("Unknown packet type:", (string)(buf[0:1]))
					continue
				}
				switch t {
				case "OPEN":
					err = json.DecodeJson(buf[1:], &obj)
					if err == nil {
						cr.wsid = obj.GetString("sid")
						logDebug("connected:", cr.wsid)
						cr._enable()
					}
				case "CLOSE":
					err = nil
					logDebug("disconnecting")
					if cr.socket != nil {
						cr.socket.Close()
					}
					return
				case "PING":
					cr.socket.WriteMessage(websocket.TextMessage, ([]byte)(PACKET_TYPE0["PONG"] + (string)(buf[1:])))
				case "MESSAGE":
					cr.onWsMessage(PACKET_TYPE[(string)(buf[1:2])], buf[2:])
				}
			}
		}
	}()
	return true
}

func (cr *Cluster)_enable(){
	cr.enabled = true
	cr.sendMsg("enable", json.JsonObj{
		"host": cr.host,
		"port": cr.public_port,
		"version": cr.version,
	})
	cr.keepalive = createInterval(func(){
		cr.KeepAlive()
	}, time.Second * 60)
}

func (cr *Cluster)KeepAlive()(ok bool){
	ok = false
	hits, hbytes := cr.hits, cr.hbytes
	defer func(){
		if ok {
			cr.hits -= hits
			cr.hbytes -= hbytes
		}
	}()
	var (
		err error
	)
	err = cr.sendMsg("keep-alive", json.JsonObj{
		"time": time.Now().UTC().Format("2007-01-02T15:04:05Z"),
		"hits": cr.hits,
		"bytes": cr.hbytes,
	})
	ok = err == nil
	return
}

func (cr *Cluster)Disable(){
	cr.enabled = false
	cr.socket.WriteMessage(websocket.TextMessage, ([]byte)(PACKET_TYPE["DISCONNECT"]))
	cr.socket.Close()
	cr.socket = nil
}

func (cr *Cluster)queryFunc(method string, url string, call func(*http.Request))(res *http.Response, err error){
	var req *http.Request
	req, err = http.NewRequest(method, cr.prefix + url, nil)
	if err != nil { return }
	req.SetBasicAuth(cr.username, cr.password)
	req.Header.Set("User-Agent", cr.useragent)
	if call != nil {
		call(req)
	}
	res, err = cr.client.Do(req)
	return
}

func (cr *Cluster)queryURL(method string, url string)(res *http.Response, err error){
	return cr.queryFunc(method, url, nil)
}

func (cr *Cluster)queryURLHeader(method string, url string, header map[string]string)(res *http.Response, err error){
	return cr.queryFunc(method, url, func(req *http.Request){
		if header != nil {
			for k, v := range header {
				req.Header.Set(k, v)
			}
		}
	})
}

type FileInfo struct{
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type IFileList struct{
	Files []FileInfo `json:"files"`
}

func (cr *Cluster)getHashPath(hash string)(string){
	return ufile.JoinPath(cr.cachedir, hash[:2], hash)
}

func (cr *Cluster)GetFileList()(files []FileInfo){
	var(
		err error
		res *http.Response
	)
	res, err = cr.queryURL("GET", "/openbmclapi/files")
	if err != nil {
		logError("Query filelist error:", err)
		return nil
	}
	list := new(IFileList)
	err = json.ReadJson(res.Body, &list)
	if err != nil {
		logError("Parse filelist body error:", err)
		return nil
	}
	return list.Files
}

type extFileInfo struct{
	*FileInfo
	Err error
}

func (cr *Cluster)SyncFiles(_files []FileInfo){
	files := make([]FileInfo, 0, len(_files) / 3)
	for _, f := range _files {
		p := cr.getHashPath(f.Hash)
		if ufile.IsNotExist(p) {
			files = append(files, f)
			p = ufile.DirPath(p)
			if ufile.IsNotExist(p) {
				os.MkdirAll(p, 0744)
			}
		}
	}
	sort.Slice(files, func(i, j int)(bool){ return files[i].Hash < files[j].Hash })
	fl := len(files)
	if fl == 0 {
		logInfo("All files are synchronized")
		go cr.gc(_files)
		return
	}
	var (
		totalsize float32 = 0
		downloaded float32 = 0
	)
	for i, _ := range files { totalsize += (float32)(files[i].Size) }
	logInfof("Starting synchronize file, count: %d, total: %.3fMB", fl, totalsize / 1024 / 1024)
	start := time.Now()
	re := make(chan *extFileInfo, (int)(cr.max_conn))
	fcount := 0
	alive := (uint)(0)
	var (
		dlhandle func(f *extFileInfo, c chan<- *extFileInfo)
		handlef func(f *extFileInfo)
		dlfile func(f *FileInfo)
	)
	dlhandle = func(f *extFileInfo, c chan<- *extFileInfo){
		defer func(){
			alive--
			c <- f
		}()
		var(
			err error
			res *http.Response
			fd *os.File
		)
		p := cr.getHashPath(f.Hash)
		defer func(){
			if err != nil {
				if ufile.IsExist(p) {
					ufile.RemoveFile(p)
				}
			}
		}()
		logDebug("Downloading:", f.Path)
		for i := 0; i < 3 ;i++ {
			res, err = cr.queryURL("GET", f.Path)
			if err != nil {
				continue
			}
			fd, err = os.Create(p)
			if err != nil {
				continue
			}
			defer fd.Close()
			_, err = io.Copy(fd, res.Body)
			fd.Close()
			if err != nil {
				continue
			}
			// logDebug("Downloaded:", f.Path)
			return
		}
		if err != nil {
			f.Err = err
		}
		// logDebug("Download Error:", f.Path, err)
	}
	handlef = func(f *extFileInfo){
		if f.Err != nil {
			logError("Download file error:", f.Path, f.Err)
			dlfile(f.FileInfo)
		}else{
			fcount++
			downloaded += (float32)(f.Size)
			logInfo("Downloaded:", f.Path,
				fmt.Sprintf("[%.3f/%.3fMB]%.2f%%", downloaded / 1024 / 1024, totalsize / 1024 / 1024, downloaded / totalsize * 100))
		}
	}
	dlfile = func(f *FileInfo){
		for alive >= cr.max_conn {
			select{
			case r := <-re:
				handlef(r)
			}
		}
		alive++
		go dlhandle(&extFileInfo{ FileInfo: f, Err: nil }, re)
	}
	for i, _ := range files {
		dlfile(&files[i])
	}
	for fcount < fl {
		handlef(<-re)
	}
	use := time.Since(start)
	logInfof("Sync file complete, use time: %v, %.3fMB/s", use, totalsize / 1024 / 1024 / (float32)(use))
	go cr.gc(_files)
}

func (cr *Cluster)gc(files []FileInfo){
	fileset := make(map[string]struct{})
	for i, _ := range files {
		fileset[cr.getHashPath(files[i].Hash)] = struct{}{}
	}
	stack := make([]string, 0, 1)
	stack = append(stack, cr.cachedir)
	var (
		ok bool
		p string
		n string
		fil []os.FileInfo
		err error
	)
	for len(stack) > 0 {
		p = stack[len(stack) - 1]
		stack = stack[:len(stack) - 1]
		fil, err = ioutil.ReadDir(p)
		if err != nil {
			continue
		}
		for _, f := range fil {
			n = ufile.JoinPath(p, f.Name())
			if _, ok = fileset[n]; !ok {
				ufile.RemoveFile(n)
			}
		}
	}
}

func (cr *Cluster)DownloadFile(hash string)(bool){
	var(
		err error
		res *http.Response
		fd *os.File
	)
	res, err = cr.queryURL("GET", "/openbmclapi/download/" + hash + "?noopen=1")
	if err != nil {
		logError("Query file error:", err)
		return false
	}
	fd, err = os.Create(hashToFilename(hash))
	if err != nil {
		logError("Create file error:", err)
		return false
	}
	defer fd.Close()
	_, err = io.Copy(fd, res.Body)
	if err != nil {
		logError("Write file error:", err)
		return false
	}
	return true
}

func (cr *Cluster)ServeHTTP(response http.ResponseWriter, request *http.Request){
	method := request.Method
	url := request.URL
	rawpath := url.EscapedPath()
	logDebug("serve url:", url.String())
	switch{
	case strings.HasPrefix(rawpath, "/download/"):
		if method == "GET" {
			path := cr.getHashPath(rawpath[10:])
			if ufile.IsNotExist(path) {
				if !cr.DownloadFile(path) {
					response.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
			if name := request.Form.Get("name"); name != "" {
				response.Header().Set("Content-Disposition", "attachment; filename=" + name)
			}
			response.Header().Set("Cache-Control", "max-age=2592000") // 30 days
			fd, err := os.Open(path)
			if err != nil {
				response.WriteHeader(http.StatusInternalServerError)
				return
			}
			buf := make([]byte, 1024 * 1024 * 8) // chunk size = 8MB
			var (
				hb uint64
				n int
			)
			for {
				n, err = fd.Read(buf)
				if err != nil {
					response.WriteHeader(http.StatusInternalServerError)
					return
				}
				if n == 0 {
					break
				}
				_, err = response.Write(buf[:n])
				if err != nil {
					response.WriteHeader(http.StatusInternalServerError)
					return
				}
				hb += (uint64)(n)
			}
			cr.hits++
			cr.hbytes += hb
			response.WriteHeader(http.StatusOK)
			return
		}
	case strings.HasPrefix(rawpath, "/measure/"):
		if method == "GET"{
			if request.Header.Get("x-openbmclapi-secret") != cr.password {
				response.WriteHeader(http.StatusForbidden)
				return
			}
			n, e := strconv.Atoi(rawpath[9:])
			if e != nil || n < 0 || n > 200 {
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			response.Write(make([]byte, n * 1024 * 1024))
			response.WriteHeader(http.StatusOK)
			return
		}
	}
	response.WriteHeader(http.StatusNotFound)
}

