package main

import (
	"context"
	"errors"
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

	SyncFileInterval  = time.Minute * 10
	KeepAliveInterval = time.Second * 60
)

func readConfig() {
	// TODO: Use struct with json tag
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
		if byoc := os.Getenv("CLUSTER_BYOC"); byoc != "" {
			USE_HTTPS = byoc != "true"
		} else {
			USE_HTTPS = !(obj.Has("nohttps") && obj.GetBool("nohttps"))
		}
	}
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			logError("Panic error:", err)
			panic(err)
		}
	}()

	startFlushLogFile()

	bgctx := context.Background()

	signalCh := make(chan os.Signal, 1)

START:
	signal.Stop(signalCh)

	exitCh := make(chan struct{}, 0)
	ctx, cancel := context.WithCancel(bgctx)

	readConfig()
	cluster := NewCluster(ctx, HOST, PUBLIC_PORT, CLUSTER_ID, CLUSTER_SECRET, VERSION, fmt.Sprintf("%s:%d", "0.0.0.0", PORT))

	logInfof("Starting Go-OpenBmclApi v%s", VERSION)

	// {
	// 	fl := cluster.GetFileList()
	// 	if fl == nil {
	// 		logError("Cluster filelist is nil, exit")
	// 		os.Exit(1)
	// 	}
	// 	cluster.SyncFiles(fl, ctx)
	// }

	if !cluster.Connect() {
		os.Exit(1)
	}

	var certFile, keyFile string
	if USE_HTTPS {
		pair, err := cluster.RequestCert()
		if err != nil {
			logError("Error when requesting cert key pair:", err)
			os.Exit(1)
		}
		certFile, keyFile, err = pair.SaveAsFile()
		if err != nil {
			logError("Error when saving cert key pair:", err)
			os.Exit(1)
		}
	}

	go func() {
		defer close(exitCh)
		logInfof("Server start at \"%s\"", cluster.Server.Addr)
		var err error
		if USE_HTTPS {
			err = cluster.Server.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = cluster.Server.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logError("Error on server:", err)
			os.Exit(1)
		}
	}()

	if err := cluster.Enable(); err != nil {
		logError("Cannot enable cluster:", err)
		os.Exit(1)
	}

	createInterval(ctx, func() {
		fl := cluster.GetFileList()
		if fl == nil {
			logError("Cannot get cluster file list, exit")
			os.Exit(1)
		}
		cluster.SyncFiles(fl, ctx)
	}, SyncFileInterval)

	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	select {
	case <-exitCh:
		return
	case s := <-signalCh:
		shutCtx, cancelShut := context.WithTimeout(bgctx, 16*time.Second)
		logWarn("Closing server ...")
		cancel()
		shutExit := make(chan struct{}, 0)
		go func() {
			defer close(shutExit)
			defer cancelShut()
			cluster.Disable()
			cluster.Server.Shutdown(shutCtx)
		}()
		select {
		case <-shutExit:
		case s := <-signalCh:
			logWarn("signal:", s)
			logError("Second close signal received, exit")
			return
		}
		logWarn("Server closed.")
		if s == syscall.SIGHUP {
			logInfo("Restarting server ...")
			goto START
		}
	}
}
