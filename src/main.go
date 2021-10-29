
package main

import (
	os "os"
	time "time"
	strconv "strconv"
	fmt "fmt"
	context "context"
	signal "os/signal"
	syscall "syscall"
	http "net/http"

	json "github.com/KpnmServer/go-util/json"
)

const (
	VERSION = "0.6.0"
)

var (
	DEBUG bool = false
	SHOW_SERVE_INFO bool = false
	IGNORE_SERVE_ERROR bool = true
	HOST string = ""
	PORT uint16 = 0
	PUBLIC_PORT uint16 = 0
	CLUSTER_ID string = "username"
	CLUSTER_SECRET string = "password"
	USE_HTTPS bool = false
	CRT_FILE string = ""
	KEY_FILE string = ""
)

func readConfig(){
	{ // read config file
		var (
			fd *os.File
			obj json.JsonObj = nil
			err error
			n int
		)
		fd, err = os.Open("config.json")
		if err != nil { panic(err) }
		defer fd.Close()
		obj = make(json.JsonObj)
		err = json.ReadJson(fd, &obj)
		if err != nil { panic(err) }
		DEBUG = obj.Has("debug") && obj.GetBool("debug")
		SHOW_SERVE_INFO = obj.Has("show_serve_info") && obj.GetBool("show_serve_info")
		IGNORE_SERVE_ERROR = obj.Has("ignore_serve_error") && obj.GetBool("ignore_serve_error")
		if os.Getenv("CLUSTER_IP") != "" {
			HOST = os.Getenv("CLUSTER_IP")
		}else{
			HOST = obj.GetString("host")
		}
		if os.Getenv("CLUSTER_PORT") != "" {
			n, err = strconv.Atoi(os.Getenv("CLUSTER_PORT"))
			if err != nil { panic(err) }
			PORT = (uint16)(n)
		}else if obj.Has("port"){
			PORT = obj.GetUInt16("port")
		}
		if os.Getenv("CLUSTER_PUBLIC_PORT") != "" {
			n, err = strconv.Atoi(os.Getenv("CLUSTER_PUBLIC_PORT"))
			if err != nil { panic(err) }
			PUBLIC_PORT = (uint16)(n)
		}else if obj.Has("public_port"){
			PUBLIC_PORT = obj.GetUInt16("public_port")
		}else{
			PUBLIC_PORT = PORT
		}
		if os.Getenv("CLUSTER_ID") != "" {
			CLUSTER_ID = os.Getenv("CLUSTER_ID")
		}else{
			CLUSTER_ID = obj.GetString("cluster_id")
		}
		if os.Getenv("CLUSTER_SECRET") != "" {
			CLUSTER_SECRET = os.Getenv("CLUSTER_SECRET")
		}else{
			CLUSTER_SECRET = obj.GetString("cluster_secret")
		}
		USE_HTTPS = obj.Has("https") && obj.GetBool("https")
		if USE_HTTPS {
			CRT_FILE = obj.GetString("crt_file")
			KEY_FILE = obj.GetString("key_file")
		}
	}
}

var cluster *Cluster = nil
var syncFileTimer func()(bool) = nil

func before()(bool){
	readConfig()
	cluster = NewCluster(HOST, PUBLIC_PORT, CLUSTER_ID, CLUSTER_SECRET, VERSION, fmt.Sprintf("%s:%d", "0.0.0.0", PORT))

	logInfof("Starting OpenBmclApi(golang) v%s", VERSION)
	{
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Filelist nil, exit")
			return false
		}
		cluster.SyncFiles(fl)
	}
	if !cluster.Enable() {
		logError("Can not enable, exit")
		return false
	}

	syncFileTimer = createInterval(func(){
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Can not get file list.")
			return
		}
		cluster.SyncFiles(fl)
	}, time.Minute * 10)

	go func(){
		logInfof("Server start at \"%s\"", cluster.Server.Addr)
		var err error
		if USE_HTTPS {
			err = cluster.Server.ListenAndServeTLS(CRT_FILE, KEY_FILE)
		}else{
			err = cluster.Server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logError("Error on server:", err)
		}
	}()

	return true
}

func after(){
	if syncFileTimer != nil {
		syncFileTimer()
		syncFileTimer = nil
	}
	cluster.Disable()
}

func main(){
	defer func(){
		err := recover()
		if err != nil {
			logError("Panic error:", err)
			panic(err)
		}
	}()


	bgcont := context.Background()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	var s os.Signal
	start: if !before() { return }

	select {
	case s = <-sigs:
		timeoutCtx, _ := context.WithTimeout(bgcont, 16 * time.Second)
		logWarn("Closing server...")
		after()
		cluster.Server.Shutdown(timeoutCtx)
		logWarn("Server closed.")
	}
	if s == syscall.SIGHUP {
		goto start
	}
}
