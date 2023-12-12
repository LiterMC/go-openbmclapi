package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	json "github.com/KpnmServer/go-util/json"
)

var (
	DEBUG              bool   = false
	SHOW_SERVE_INFO    bool   = false
	IGNORE_SERVE_ERROR bool   = true
	HOST               string = ""
	PORT               uint16 = 0
	PUBLIC_PORT        uint16 = 0
	CLUSTER_ID         string = "username"
	CLUSTER_SECRET     string = "password"
	USE_HTTPS          bool   = false
	CRT_FILE           string = ""
	KEY_FILE           string = ""

	SyncFileInterval  = time.Minute * 10
	KeepAliveInterval = time.Second * 60
)

func readConfig() {
	{ // read config file
		var (
			fd  *os.File
			obj json.JsonObj = nil
			err error
			n   int
		)
		fd, err = os.Open("config.json")
		if err != nil {
			panic(err)
		}
		defer fd.Close()
		obj = make(json.JsonObj)
		err = json.ReadJson(fd, &obj)
		if err != nil {
			panic(err)
		}
		DEBUG = obj.Has("debug") && obj.GetBool("debug")
		SHOW_SERVE_INFO = obj.Has("show_serve_info") && obj.GetBool("show_serve_info")
		IGNORE_SERVE_ERROR = obj.Has("ignore_serve_error") && obj.GetBool("ignore_serve_error")
		if os.Getenv("CLUSTER_IP") != "" {
			HOST = os.Getenv("CLUSTER_IP")
		} else {
			HOST = obj.GetString("public_host")
		}
		if os.Getenv("CLUSTER_PORT") != "" {
			n, err = strconv.Atoi(os.Getenv("CLUSTER_PORT"))
			if err != nil {
				panic(err)
			}
			PORT = (uint16)(n)
		} else if obj.Has("port") {
			PORT = obj.GetUInt16("port")
		}
		if os.Getenv("CLUSTER_PUBLIC_PORT") != "" {
			n, err = strconv.Atoi(os.Getenv("CLUSTER_PUBLIC_PORT"))
			if err != nil {
				panic(err)
			}
			PUBLIC_PORT = (uint16)(n)
		} else if obj.Has("public_port") {
			PUBLIC_PORT = obj.GetUInt16("public_port")
		} else {
			PUBLIC_PORT = PORT
		}
		if os.Getenv("CLUSTER_ID") != "" {
			CLUSTER_ID = os.Getenv("CLUSTER_ID")
		} else {
			CLUSTER_ID = obj.GetString("cluster_id")
		}
		if os.Getenv("CLUSTER_SECRET") != "" {
			CLUSTER_SECRET = os.Getenv("CLUSTER_SECRET")
		} else {
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
var syncFileTimer func() bool = nil

func before(ctx context.Context) bool {
	readConfig()
	cluster = NewCluster(HOST, PUBLIC_PORT, CLUSTER_ID, CLUSTER_SECRET, VERSION, fmt.Sprintf("%s:%d", "0.0.0.0", PORT))

	logInfof("Starting OpenBmclApi(golang) v%s", VERSION)
	{
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Cluster filelist is nil, exit")
			return false
		}
		cluster.SyncFiles(fl, ctx)
	}

	go func() {
		logInfof("Server start at \"%s\"", cluster.Server.Addr)
		var err error
		if USE_HTTPS {
			err = cluster.Server.ListenAndServeTLS(CRT_FILE, KEY_FILE)
		} else {
			err = cluster.Server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logError("Error on server:", err)
		}
	}()

	if !cluster.Enable() {
		logError("Cannot enable cluster, exit")
		return false
	}

	createInterval(ctx, func() {
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Cannot get cluster file list, exit")
			return
		}
		cluster.SyncFiles(fl, ctx)
	}, SyncFileInterval)

	return true
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			logError("Panic error:", err)
			panic(err)
		}
	}()

	bgctx := context.Background()

	var (
		signalCh = make(chan os.Signal, 1)
		exitCh   = make(chan struct{}, 1)
	)
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

START:
	ctx, cancel := context.WithCancel(bgctx)
	go func() {
		defer close(exitCh)
		if !before(ctx) {
			return
		}
	}()

	select {
	case <-exitCh:
		return
	case s := <-signalCh:
		timeoutCtx, cancelShut := context.WithTimeout(bgctx, 16*time.Second)
		logWarn("Closing server ...")
		cancel()
		cluster.Disable()
		cluster.Server.Shutdown(timeoutCtx)
		cancelShut()
		logWarn("Server closed.")
		if s == syscall.SIGHUP {
			logInfo("Restarting server ...")
			goto START
		}
	}
}
