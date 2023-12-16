package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

var (
	SyncFileInterval  = time.Minute * 10
	KeepAliveInterval = time.Second * 59
)

var startTime = time.Now()

type Config struct {
	Debug           bool   `json:"debug"`
	RecordServeInfo bool   `json:"record_serve_info"`
	Nohttps         bool   `json:"nohttps"`
	PublicHost      string `json:"public_host"`
	PublicPort      uint16 `json:"public_port"`
	Port            uint16 `json:"port"`
	ClusterId       string `json:"cluster_id"`
	ClusterSecret   string `json:"cluster_secret"`
	UseOss          bool   `json:"use_oss"`
	OssRedirectBase string `json:"oss_redirect_base"`
	SkipMeasureGen  bool   `json:"oss_skip_measure_gen"`
	Hijack          bool   `json:"hijack"`
	HijackPort      uint16 `json:"hijack_port"`
	AntiHijackDNS   string `json:"anti_hijack_dns"`
}

var config Config

func readConfig() {
	const configPath = "config.json"

	config = Config{
		Debug:           false,
		RecordServeInfo: false,
		Nohttps:         false,
		PublicHost:      "example.com",
		PublicPort:      8080,
		Port:            4000,
		ClusterId:       "${CLUSTER_ID}",
		ClusterSecret:   "${CLUSTER_SECRET}",
		UseOss:          false,
		OssRedirectBase: "https://oss.example.com/base/paths",
		SkipMeasureGen:  false,
		Hijack:          false,
		HijackPort:      8090,
		AntiHijackDNS:   "8.8.8.8:53",
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logError("Cannot read config:", err)
			os.Exit(1)
		}
		logError("Config file not exists, create one")
	} else if err = json.Unmarshal(data, &config); err != nil {
		logError("Cannot parse config:", err)
		os.Exit(1)
	}

	if data, err = json.MarshalIndent(config, "", "  "); err != nil {
		logError("Cannot encode config:", err)
		os.Exit(1)
	}
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		logError("Cannot write config:", err)
		os.Exit(1)
	}

	if os.Getenv("DEBUG") == "true" {
		config.Debug = true
	}
	if v := os.Getenv("CLUSTER_IP"); v != "" {
		config.PublicHost = v
	}
	if v := os.Getenv("CLUSTER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			logErrorf("Cannot parse CLUSTER_PORT %q: %v", v, err)
		} else {
			config.Port = (uint16)(n)
		}
	}
	if v := os.Getenv("CLUSTER_PUBLIC_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			logErrorf("Cannot parse CLUSTER_PUBLIC_PORT %q: %v", v, err)
		} else {
			config.PublicPort = (uint16)(n)
		}
	}
	if v := os.Getenv("CLUSTER_ID"); v != "" {
		config.ClusterId = v
	}
	if v := os.Getenv("CLUSTER_SECRET"); v != "" {
		config.ClusterSecret = v
	}
	if byoc := os.Getenv("CLUSTER_BYOC"); byoc != "" {
		config.Nohttps = byoc == "true"
	}
}

const baseDir = "."

var (
	hijackPath   = filepath.Join(baseDir, "__hijack")
	ossMirrorDir = filepath.Join(baseDir, "oss_mirror")
)

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

	ctx, cancel := context.WithCancel(bgctx)

	readConfig()

	var (
		dialer   *net.Dialer
		hjproxy  *HjProxy
		hjServer *http.Server
	)
	if config.Hijack {
		dialer = getDialerWithDNS(config.AntiHijackDNS)
		hjproxy = NewHjProxy(dialer, hijackPath)
		hjServer = &http.Server{
			Addr:    fmt.Sprintf("0.0.0.0:%d", config.HijackPort),
			Handler: hjproxy,
		}
		go func() {
			logInfof("Hijack server start at %q", hjServer.Addr)
			err := hjServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logError("Error on server:", err)
				os.Exit(1)
			}
		}()
		time.Sleep(time.Second * 100000)
	}
	redirectBase := ""
	if config.UseOss {
		redirectBase = config.OssRedirectBase
	}

	logInfof("Starting Go-OpenBmclApi v%s (%s)", ClusterVersion, BuildVersion)
	cluster, err := NewCluster(ctx, baseDir,
		config.PublicHost, config.PublicPort,
		config.ClusterId, config.ClusterSecret,
		config.Nohttps, dialer,
		redirectBase,
	)
	if err != nil {
		logError("Cannot init cluster:", err)
		os.Exit(1)
	}

	if config.UseOss {
		createOssMirrorDir()
		assertOSS(10)
		go func() {
			ticker := time.NewTicker(time.Minute * 5)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					assertOSS(1)
				}
			}
		}()
	}

	logDebugf("Receiving signals")
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	if !cluster.Connect(ctx) {
		os.Exit(1)
	}

	clusterSvr := &http.Server{
		Addr:        fmt.Sprintf("%s:%d", "0.0.0.0", config.Port),
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second,
		Handler:     cluster.GetHandler(),
	}

	go func(ctx context.Context) {
		listener, err := net.Listen("tcp", clusterSvr.Addr)
		if err != nil {
			logErrorf("Cannot listen on %s: %v", clusterSvr.Addr, err)
			os.Exit(1)
		}

		if !config.Nohttps {
			tctx, cancel := context.WithTimeout(ctx, time.Minute*10)
			pair, err := cluster.RequestCert(tctx)
			cancel()
			if err != nil {
				logError("Error when requesting cert key pair:", err)
				os.Exit(1)
			}
			publicHost, _ := parseCertCommonName(([]byte)(pair.Cert))
			certFile, keyFile, err := pair.SaveAsFile()
			if err != nil {
				logError("Error when saving cert key pair:", err)
				os.Exit(1)
			}
			go func() {
				defer listener.Close()
				if err = clusterSvr.ServeTLS(listener, certFile, keyFile); !errors.Is(err, http.ErrServerClosed) {
					logError("Error on server:", err)
					os.Exit(1)
				}
			}()
			if publicHost == "" {
				publicHost = config.PublicHost
			}
			logInfof("Server public at https://%s:%d (%s)", publicHost, config.PublicPort, clusterSvr.Addr)
		} else {
			go func() {
				defer listener.Close()
				if err = clusterSvr.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
					logError("Error on server:", err)
					os.Exit(1)
				}
			}()
			logInfof("Server public at http://%s:%d (%s)", config.PublicHost, config.PublicPort, clusterSvr.Addr)
		}

		logInfof("Fetching file list")
		fl, err := cluster.GetFileList(ctx)
		if err != nil {
			logError("Cannot query cluster file list:", err)
			if errors.Is(err, context.Canceled) {
				return
			}
			os.Exit(1)
		}
		cluster.SyncFiles(ctx, fl)

		createInterval(ctx, func() {
			logInfof("Fetching file list")
			fl, err := cluster.GetFileList(ctx)
			if err != nil {
				logError("Cannot query cluster file list:", err)
				return
			}
			cluster.SyncFiles(ctx, fl)
		}, SyncFileInterval)

		if err := cluster.Enable(ctx); err != nil {
			logError("Cannot enable cluster:", err)
			os.Exit(1)
		}
	}(ctx)

	select {
	case s := <-signalCh:
		cancel()
		shutCtx, cancelShut := context.WithTimeout(context.Background(), 20*time.Second)
		logWarn("Closing server ...")
		shutExit := make(chan struct{}, 0)
		if hjServer != nil {
			go hjServer.Shutdown(shutCtx)
		}
		go func() {
			defer close(shutExit)
			defer cancelShut()
			cluster.Disable(shutCtx)
			logInfo("Cluster disabled, closing http server")
			clusterSvr.Shutdown(shutCtx)
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

func createOssMirrorDir() {
	logInfof("Creating %s", ossMirrorDir)
	if err := os.MkdirAll(ossMirrorDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", ossMirrorDir, err)
		os.Exit(2)
	}
	cacheDir := filepath.Join(baseDir, "cache")
	downloadDir := filepath.Join(ossMirrorDir, "download")
	os.RemoveAll(downloadDir)
	if err := os.Mkdir(downloadDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", downloadDir, err)
		os.Exit(2)
	}
	measureDir := filepath.Join(ossMirrorDir, "measure")
	if err := os.Mkdir(measureDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		logErrorf("Cannot create OSS mirror folder %q: %v", measureDir, err)
		os.Exit(2)
	}
	for i := 0; i < 0x100; i++ {
		d := hex.EncodeToString([]byte{(byte)(i)})
		o := filepath.Join(cacheDir, d)
		t := filepath.Join(downloadDir, d)
		os.Mkdir(o, 0755)
		if err := os.Symlink(filepath.Join("..", "..", o), t); err != nil {
			logErrorf("Cannot create OSS mirror cache symlink %q: %v", t, err)
			os.Exit(2)
		}
	}
	if !config.SkipMeasureGen {
		logDebug("Creating measure files")
		buf := make([]byte, 200*1024*1024)
		for i := 1; i <= 200; i++ {
			size := i * 1024 * 1024
			t := filepath.Join(measureDir, strconv.Itoa(i))
			if stat, err := os.Stat(t); err == nil {
				x := stat.Size()
				if x == (int64)(size) {
					logDebug("Skipping", t)
					continue
				}
				logDebugf("File [%d] size %d does not match %d", i, x, size)
			} else {
				logDebugf("Cannot get stat of %s: %v", t, err)
			}
			logDebug("Writing", t)
			if err := os.WriteFile(t, buf[:size], 0644); err != nil {
				logErrorf("Cannot create OSS mirror measure file %q: %v", t, err)
				os.Exit(2)
			}
		}
		logDebug("Measure files created")
	}
}

func assertOSS(size int) {
	logInfo("Checking OSS ...")
	target, err := url.JoinPath(config.OssRedirectBase, "measure", strconv.Itoa(size))
	if err != nil {
		logError("Cannot check OSS server:", err)
		os.Exit(2)
	}
	res, err := http.Get(target)
	if err != nil {
		logErrorf("OSS check request failed %q: %v", target, err)
		os.Exit(2)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		logErrorf("OSS check request failed %q: %d %s", target, res.StatusCode, res.Status)
		os.Exit(2)
	}
	logDebug("reading OSS response")
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		logErrorf("OSS check request failed %q: %v", target, err)
		os.Exit(2)
	}
	if n != (int64)(size)*1024*1024 {
		logErrorf("OSS check request failed %q: expected %dMB, but got %d bytes", target, size, n)
		os.Exit(2)
	}
}
