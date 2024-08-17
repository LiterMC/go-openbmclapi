/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"runtime/pprof"

	doh "github.com/libp2p/go-doh-resolver"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/api/bmclapi"
	"github.com/LiterMC/go-openbmclapi/cluster"
	"github.com/LiterMC/go-openbmclapi/config"
	"github.com/LiterMC/go-openbmclapi/database"
	"github.com/LiterMC/go-openbmclapi/limited"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/notify"
	"github.com/LiterMC/go-openbmclapi/notify/email"
	"github.com/LiterMC/go-openbmclapi/notify/webhook"
	"github.com/LiterMC/go-openbmclapi/notify/webpush"
	"github.com/LiterMC/go-openbmclapi/storage"
	"github.com/LiterMC/go-openbmclapi/token"
	"github.com/LiterMC/go-openbmclapi/utils"
)

type Runner struct {
	Config *config.Config

	configHandler  *ConfigHandler
	client         *cluster.HTTPClient
	database       database.DB
	clusters       map[string]*cluster.Cluster
	userManager    api.UserManager
	tokenManager   api.TokenManager
	subManager     api.SubscriptionManager
	notifyManager  *notify.Manager
	storageManager *storage.Manager
	statManager    *cluster.StatManager
	hijacker       *bmclapi.HjProxy

	server         *http.Server
	apiRateLimiter *limited.APIRateMiddleWare
	handler        http.Handler
	handlerAPIv0   http.Handler
	hijackHandler  http.Handler

	tlsConfig    *tls.Config
	certificates map[string]*tls.Certificate
	publicHost   string
	publicPort   uint16
	listener     *utils.HTTPTLSListener

	reloading    atomic.Bool
	updating     atomic.Bool
	tunnelCancel context.CancelFunc
}

func NewRunner() *Runner {
	r := new(Runner)

	r.configHandler = &ConfigHandler{r: r}

	var dialer *net.Dialer
	r.client = cluster.NewHTTPClient(dialer, r.Config.Cache.NewCache())

	{
		var err error
		if r.Config.Database.Driver == "memory" {
			r.database = database.NewMemoryDB()
		} else if r.database, err = database.NewSqlDB(r.Config.Database.Driver, r.Config.Database.DSN); err != nil {
			log.Errorf("Cannot connect to database: %v", err)
			os.Exit(1)
		}
	}

	r.userManager = &singleUserManager{
		user: &api.User{
			Username:    r.Config.Dashboard.Username,
			Password:    r.Config.Dashboard.Password,
			Permissions: api.RootPerm,
		},
	}
	if apiHMACKey, err := utils.LoadOrCreateHmacKey(dataDir, "server"); err != nil {
		log.Errorf("Cannot load HMAC key: %v", err)
		os.Exit(1)
	} else {
		r.tokenManager = token.NewDBManager("go-openbmclapi", apiHMACKey, r.database)
	}
	{
		r.notifyManager = notify.NewManager(dataDir, r.database, r.client.CachedClient(), "go-openbmclapi")
		r.notifyManager.AddPlugin(new(webhook.Plugin))
		if r.Config.Notification.EnableEmail {
			emailPlg, err := email.NewSMTP(r.Config.Notification.EmailSMTP, r.Config.Notification.EmailSMTPEncryption,
				r.Config.Notification.EmailSender, r.Config.Notification.EmailSenderPassword)
			if err != nil {
				log.Errorf("Cannot init SMTP client: %v", err)
				os.Exit(1)
			}
			r.notifyManager.AddPlugin(emailPlg)
		}
		r.notifyManager.AddPlugin(new(email.Plugin))
		webpushPlg := new(webpush.Plugin)
		r.notifyManager.AddPlugin(webpushPlg)

		r.subManager = &subscriptionManager{
			webpushPlg: webpushPlg,
			DB:         r.database,
		}
	}
	{
		storages := make([]storage.Storage, len(r.Config.Storages))
		for i, s := range r.Config.Storages {
			storages[i] = storage.NewStorage(s)
		}
		r.storageManager = storage.NewManager(storages)
	}
	r.statManager = cluster.NewStatManager()
	if err := r.statManager.Load(dataDir); err != nil {
		log.Errorf("Stat load failed: %v", err)
	}
	r.apiRateLimiter = limited.NewAPIRateMiddleWare(api.RealAddrCtxKey, "go-openbmclapi.cluster.logged.user" /* api/v0.loggedUserKey */)
	return r
}

func (r *Runner) getPublicPort() uint16 {
	if r.publicPort > 0 {
		return r.publicPort
	}
	return r.Config.Port
}

func (r *Runner) getCertCount() int {
	if r.tlsConfig == nil {
		return 0
	}
	return len(r.tlsConfig.Certificates)
}

func (r *Runner) InitServer() {
	r.server = &http.Server{
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 5 * time.Second,
		Handler: (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
			r.handler.ServeHTTP(rw, req)
		}),
		ErrorLog: log.ProxiedStdLog,
	}
	r.updateRateLimit(context.TODO())
	r.handler = r.GetHandler()
}

// StartServer will start the HTTP server
// If a server is already running on an old listener, the listener will be closed.
func (r *Runner) StartServer(ctx context.Context) error {
	htListener, err := r.CreateHTTPListener(ctx)
	if err != nil {
		return err
	}
	if r.listener != nil {
		r.listener.Close()
	}
	r.listener = htListener
	go func() {
		defer htListener.Close()
		if err := r.server.Serve(htListener); !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Error("Error when serving:", err)
			os.Exit(1)
		}
	}()
	return nil
}

func (r *Runner) GetClusterGeneralConfig() config.ClusterGeneralConfig {
	return config.ClusterGeneralConfig{
		PublicHost:        r.publicHost,
		PublicPort:        r.getPublicPort(),
		NoFastEnable:      r.Config.Advanced.NoFastEnable,
		MaxReconnectCount: r.Config.MaxReconnectCount,
	}
}

func (r *Runner) InitClusters(ctx context.Context) {

	_ = doh.NewResolver // TODO: use doh resolver

	r.clusters = make(map[string]*cluster.Cluster)
	gcfg := r.GetClusterGeneralConfig()
	for name, opts := range r.Config.Clusters {
		cr := cluster.NewCluster(name, opts, gcfg, r.storageManager, r.statManager)
		if err := cr.Init(ctx); err != nil {
			log.TrErrorf("error.init.failed", err)
		} else {
			r.clusters[name] = cr
		}
	}

	// r.cluster = NewCluster(ctx,
	// 	ClusterServerURL,
	// 	baseDir,
	// 	config.PublicHost, r.getPublicPort(),
	// 	config.ClusterId, config.ClusterSecret,
	// 	config.Byoc, dialer,
	// 	config.Storages,
	// 	cache,
	// )
	// if err := r.cluster.Init(ctx); err != nil {
	// 	log.TrErrorf("error.init.failed"), err)
	// 	os.Exit(1)
	// }
}

func (r *Runner) ListenSignals(ctx context.Context, cancel context.CancelFunc) int {
	signalCh := make(chan os.Signal, 1)
	log.Debugf("Receiving signals")
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(signalCh)

	var (
		forceStop context.CancelFunc
		exited    = make(chan struct{}, 0)
	)

	for {
		select {
		case <-exited:
			return 0
		case s := <-signalCh:
			switch s {
			case syscall.SIGQUIT:
				// avaliable commands see <https://pkg.go.dev/runtime/pprof#Profile>
				dumpCommand := "heap"
				dumpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("go-openbmclapi-dump-command.%d.in", os.Getpid()))
				log.Infof("Reading dump command file at %q", dumpFileName)
				var buf [128]byte
				if dumpFile, err := os.Open(dumpFileName); err != nil {
					log.Errorf("Cannot open dump command file: %v", err)
				} else if n, err := dumpFile.Read(buf[:]); err != nil {
					dumpFile.Close()
					log.Errorf("Cannot read dump command file: %v", err)
				} else {
					dumpFile.Truncate(0)
					dumpFile.Close()
					dumpCommand = (string)(bytes.TrimSpace(buf[:n]))
				}
				pcmd := pprof.Lookup(dumpCommand)
				if pcmd == nil {
					log.Errorf("No pprof command is named %q", dumpCommand)
					continue
				}
				name := fmt.Sprintf(time.Now().Format("dump-%s-20060102-150405.txt"), dumpCommand)
				log.Infof("Creating goroutine dump file at %s", name)
				if fd, err := os.Create(name); err != nil {
					log.Infof("Cannot create dump file: %v", err)
				} else {
					err := pcmd.WriteTo(fd, 1)
					fd.Close()
					if err != nil {
						log.Infof("Cannot write dump file: %v", err)
					} else {
						log.Info("Dump file created")
					}
				}
			case syscall.SIGHUP:
				go r.ReloadConfig(ctx)
			default:
				cancel()
				if forceStop == nil {
					ctx, cancel := context.WithCancel(context.Background())
					forceStop = cancel
					go func() {
						defer close(exited)
						r.StopServer(ctx)
					}()
				} else {
					log.Warn("signal:", s)
					log.Error("Second close signal received, forcely shutting down")
					forceStop()
				}
			}

		}
	}
}

func (r *Runner) ReloadConfig(ctx context.Context) {
	if r.reloading.CompareAndSwap(false, true) {
		log.Error("Config is already reloading!")
		return
	}
	defer r.reloading.Store(false)

	config, err := readAndRewriteConfig()
	if err != nil {
		log.Errorf("Config error: %v", err)
	} else {
		if err := r.updateConfig(ctx, config); err != nil {
			log.Errorf("Error when reloading config: %v", err)
		}
	}
}

func (r *Runner) updateConfig(ctx context.Context, newConfig *config.Config) error {
	if err := r.configHandler.update(newConfig); err != nil {
		return err
	}
	return r.configHandler.doUpdateProcesses(ctx)
}

func (r *Runner) SetupLogger(ctx context.Context) error {
	if r.Config.Advanced.DebugLog {
		log.SetLevel(log.LevelDebug)
	} else {
		log.SetLevel(log.LevelInfo)
	}
	log.SetLogSlots(r.Config.LogSlots)
	if r.Config.NoAccessLog {
		log.SetAccessLogSlots(-1)
	} else {
		log.SetAccessLogSlots(r.Config.AccessLogSlots)
	}

	r.Config.ApplyWebManifest(dsbManifest)
	return nil
}

func (r *Runner) StopServer(ctx context.Context) {
	r.tunnelCancel()
	shutCtx, cancelShut := context.WithTimeout(context.Background(), time.Second*15)
	defer cancelShut()
	log.TrWarnf("warn.server.closing")
	shutDone := make(chan struct{}, 0)
	go func() {
		defer close(shutDone)
		defer cancelShut()
		var wg sync.WaitGroup
		for _, cr := range r.clusters {
			go func() {
				defer wg.Done()
				cr.Disable(shutCtx)
			}()
		}
		wg.Wait()
		log.TrWarnf("warn.httpserver.closing")
		r.server.Shutdown(shutCtx)
		r.listener.Close()
		r.listener = nil
	}()
	select {
	case <-shutDone:
	case <-ctx.Done():
		return
	}
	log.TrWarnf("warn.server.closed")
}

func (r *Runner) UpdateFileRecords(files map[string]*cluster.StorageFileInfo, oldfileset map[string]int64) {
	if !r.Config.Hijack.Enable {
		return
	}
	if !r.updating.CompareAndSwap(false, true) {
		return
	}
	defer r.updating.Store(false)

	sem := limited.NewSemaphore(12)
	log.Info("Begin to update file records")
	for _, f := range files {
		for _, u := range f.URLs {
			if strings.HasPrefix(u.Path, "/openbmclapi/download/") {
				continue
			}
			if oldfileset[f.Hash] > 0 {
				continue
			}
			sem.Acquire()
			go func(rec database.FileRecord) {
				defer sem.Release()
				r.database.SetFileRecord(rec)
			}(database.FileRecord{
				Path: u.Path,
				Hash: f.Hash,
				Size: f.Size,
			})
		}
	}
	sem.Wait()
	log.Info("All file records are updated")
}

func (r *Runner) InitSynchronizer(ctx context.Context) {
	fileMap := make(map[string]*cluster.StorageFileInfo)
	for _, cr := range r.clusters {
		log.TrInfof("info.filelist.fetching", cr.ID())
		if err := cr.GetFileList(ctx, fileMap, true); err != nil {
			log.TrErrorf("error.filelist.fetch.failed", cr.ID(), err)
			if errors.Is(err, context.Canceled) {
				return
			}
		}
	}

	checkCount := -1
	heavyCheck := !r.Config.Advanced.NoHeavyCheck
	heavyCheckInterval := r.Config.Advanced.HeavyCheckInterval
	if heavyCheckInterval <= 0 {
		heavyCheck = false
	}

	// if !r.Config.Advanced.SkipFirstSync
	{
		slots := 10
		if err := r.client.SyncFiles(ctx, r.storageManager, fileMap, false, slots); err != nil {
			log.Errorf("Sync failed: %v", err)
			return
		}
		go r.UpdateFileRecords(fileMap, nil)

		if !r.Config.Advanced.NoGC {
			go r.client.Gc(context.TODO(), r.storageManager, fileMap)
		}
	}
	// else if fl != nil {
	// 	if err := r.cluster.SetFilesetByExists(ctx, fl); err != nil {
	// 		return
	// 	}
	// }

	createInterval(ctx, func() {
		fileMap := make(map[string]*cluster.StorageFileInfo)
		for _, cr := range r.clusters {
			log.TrInfof("info.filelist.fetching", cr.ID())
			if err := cr.GetFileList(ctx, fileMap, false); err != nil {
				log.TrErrorf("error.filelist.fetch.failed", cr.ID(), err)
				return
			}
		}
		if len(fileMap) == 0 {
			log.Infof("No file was updated since last check")
			return
		}

		checkCount = (checkCount + 1) % heavyCheckInterval
		slots := 10
		if err := r.client.SyncFiles(ctx, r.storageManager, fileMap, heavyCheck && (checkCount == 0), slots); err != nil {
			log.Errorf("Sync failed: %v", err)
			return
		}
	}, (time.Duration)(r.Config.SyncInterval)*time.Minute)
}

func (r *Runner) CreateHTTPListener(ctx context.Context) (*utils.HTTPTLSListener, error) {
	addr := net.JoinHostPort(r.Config.Host, strconv.Itoa((int)(r.Config.Port)))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.TrErrorf("error.address.listen.failed", addr, err)
		return nil, err
	}
	if r.Config.ServeLimit.Enable {
		limted := limited.NewLimitedListener(listener, r.Config.ServeLimit.MaxConn, 0, r.Config.ServeLimit.UploadRate*1024)
		limted.SetMinWriteRate(1024)
		listener = limted
	}

	if r.Config.UseCert {
		var err error
		r.tlsConfig, err = r.GenerateTLSConfig()
		if err != nil {
			log.Errorf("Failed to generate TLS config: %v", err)
			return nil, err
		}
	}
	return utils.NewHttpTLSListener(listener, r.tlsConfig), nil
}

func (r *Runner) GenerateTLSConfig() (*tls.Config, error) {
	if len(r.Config.Certificates) == 0 {
		log.TrErrorf("error.cert.not.set")
		return nil, errors.New("No certificate is defined")
	}
	tlsConfig := new(tls.Config)
	tlsConfig.Certificates = make([]tls.Certificate, len(r.Config.Certificates))
	for i, c := range r.Config.Certificates {
		var err error
		tlsConfig.Certificates[i], err = tls.LoadX509KeyPair(c.Cert, c.Key)
		if err != nil {
			log.TrErrorf("error.cert.parse.failed", i, err)
			return nil, err
		}
	}
	return tlsConfig, nil
}

func (r *Runner) RequestClusterCert(ctx context.Context, cr *cluster.Cluster) (*tls.Certificate, error) {
	if cr.Options().Byoc {
		return nil, nil
	}
	log.TrInfof("info.cert.requesting", cr.ID())
	tctx, cancel := context.WithTimeout(ctx, time.Minute*10)
	pair, err := cr.RequestCert(tctx)
	cancel()
	if err != nil {
		log.TrErrorf("error.cert.request.failed", err)
		return nil, err
	}
	cert, err := tls.X509KeyPair(([]byte)(pair.Cert), ([]byte)(pair.Key))
	if err != nil {
		log.TrErrorf("error.cert.requested.parse.failed", err)
		return nil, err
	}
	certHost, _ := parseCertCommonName(cert.Certificate[0])
	log.TrInfof("info.cert.requested", certHost)
	return &cert, nil
}

func (r *Runner) PatchTLSWithClusterCertificates(tlsConfig *tls.Config) *tls.Config {
	certs := make([]tls.Certificate, 0, len(r.certificates))
	for _, c := range r.certificates {
		if c != nil {
			certs = append(certs, *c)
		}
	}
	if len(certs) == 0 {
		if tlsConfig == nil {
			tlsConfig = new(tls.Config)
		} else {
			tlsConfig = tlsConfig.Clone()
		}
		tlsConfig.Certificates = append(tlsConfig.Certificates, certs...)
	}
	return tlsConfig
}

// updateClustersWithGeneralConfig will re-enable all clusters with latest general config
func (r *Runner) updateClustersWithGeneralConfig(ctx context.Context) error {
	gcfg := r.GetClusterGeneralConfig()
	var wg sync.WaitGroup
	for _, cr := range r.clusters {
		wg.Add(1)
		go func(cr *cluster.Cluster) {
			defer wg.Done()
			cr.Disable(ctx)
			*cr.GeneralConfig() = gcfg
			if err := cr.Enable(ctx); err != nil {
				log.TrErrorf("error.cluster.enable.failed", cr.ID(), err)
				return
			}
		}(cr)
	}
	wg.Wait()
	return nil
}

func (r *Runner) EnableClusterAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, cr := range r.clusters {
		wg.Add(1)
		go func(cr *cluster.Cluster) {
			defer wg.Done()
			if err := cr.Enable(ctx); err != nil {
				log.TrErrorf("error.cluster.enable.failed", cr.ID(), err)
				return
			}
		}(cr)
	}
	wg.Wait()
}

func (r *Runner) StartTunneler() {
	ctx, cancel := context.WithCancel(context.Background())
	r.tunnelCancel = cancel
	go func() {
		dur := time.Second
		for {
			start := time.Now()
			r.RunTunneler(ctx)
			used := time.Since(start)
			// If the program runs no longer than 30s, then it fails too fast.
			if used < time.Second*30 {
				dur = min(dur*2, time.Minute*10)
			} else {
				dur = time.Second
			}
			select {
			case <-time.After(dur):
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (r *Runner) RunTunneler(ctx context.Context) {
	cmd := exec.CommandContext(ctx, r.Config.Tunneler.TunnelProg)
	log.TrInfof("info.tunnel.running", cmd.String())
	var (
		cmdOut, cmdErr io.ReadCloser
		err            error
	)
	cmd.Env = append(os.Environ(),
		"CLUSTER_PORT="+strconv.Itoa((int)(r.Config.Port)))
	if cmdOut, err = cmd.StdoutPipe(); err != nil {
		log.TrErrorf("error.tunnel.command.prepare.failed", err)
		os.Exit(1)
	}
	if cmdErr, err = cmd.StderrPipe(); err != nil {
		log.TrErrorf("error.tunnel.command.prepare.failed", err)
		os.Exit(1)
	}
	if err = cmd.Start(); err != nil {
		log.TrErrorf("error.tunnel.command.prepare.failed", err)
		os.Exit(1)
	}
	type addrOut struct {
		host string
		port uint16
	}
	detectedCh := make(chan addrOut, 1)
	onLog := func(line []byte) {
		tunnelHost, tunnelPort, ok := r.Config.Tunneler.MatchTunnelOutput(line)
		if !ok {
			return
		}
		if len(tunnelHost) > 0 && tunnelHost[0] == '[' && tunnelHost[len(tunnelHost)-1] == ']' { // a IPv6 with port [<ipv6>]:<port>
			tunnelHost = tunnelHost[1 : len(tunnelHost)-1]
		}
		port, err := strconv.Atoi((string)(tunnelPort))
		if err != nil {
			log.Panic(err)
		}
		select {
		case detectedCh <- addrOut{
			host: (string)(tunnelHost),
			port: (uint16)(port),
		}:
		default:
		}
	}
	go func() {
		defer cmdOut.Close()
		defer cmd.Process.Kill()
		sc := bufio.NewScanner(cmdOut)
		for sc.Scan() {
			log.Info("[tunneler/stdout]:", sc.Text())
			onLog(sc.Bytes())
		}
	}()
	go func() {
		defer cmdErr.Close()
		defer cmd.Process.Kill()
		sc := bufio.NewScanner(cmdErr)
		for sc.Scan() {
			log.Info("[tunneler/stderr]:", sc.Text())
			onLog(sc.Bytes())
		}
	}()
	go func() {
		defer log.RecordPanic()
		defer cmd.Process.Kill()
		for {
			select {
			case addr := <-detectedCh:
				log.TrInfof("info.tunnel.detected", addr.host, addr.port)
				r.publicHost, r.publicPort = addr.host, addr.port
				r.updateClustersWithGeneralConfig(ctx)
				if ctx.Err() != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	if _, err := cmd.Process.Wait(); err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Errorf("Tunnel program exited: %v", err)
	}
}

type subscriptionManager struct {
	webpushPlg *webpush.Plugin
	database.DB
}

func (s *subscriptionManager) GetWebPushKey() string {
	return base64.RawURLEncoding.EncodeToString(s.webpushPlg.GetPublicKey())
}

type singleUserManager struct {
	user *api.User
}

func (m *singleUserManager) GetUsers() []*api.User {
	return []*api.User{m.user}
}

func (m *singleUserManager) GetUser(id string) *api.User {
	if id == m.user.Username {
		return m.user
	}
	return nil
}

func (m *singleUserManager) AddUser(user *api.User) error {
	if user.Username == m.user.Username {
		return api.ErrExist
	}
	return errors.New("Not implemented")
}

func (m *singleUserManager) RemoveUser(id string) error {
	if id != m.user.Username {
		return api.ErrNotFound
	}
	return errors.New("Not implemented")
}

func (m *singleUserManager) ForEachUser(cb func(*api.User) error) error {
	err := cb(m.user)
	if err == api.ErrStopIter {
		return nil
	}
	return err
}

func (m *singleUserManager) UpdateUserPassword(username string, password string) error {
	if username != m.user.Username {
		return api.ErrNotFound
	}
	m.user.Password = password
	return nil
}

func (m *singleUserManager) UpdateUserPermissions(username string, permissions api.PermissionFlag) error {
	if username != m.user.Username {
		return api.ErrNotFound
	}
	m.user.Permissions = permissions
	return nil
}

func (m *singleUserManager) VerifyUserPassword(userId string, comparator func(password string) bool) error {
	if userId != m.user.Username {
		return errors.New("Username or password is incorrect")
	}
	if !comparator(m.user.Password) {
		return errors.New("Username or password is incorrect")
	}
	return nil
}
