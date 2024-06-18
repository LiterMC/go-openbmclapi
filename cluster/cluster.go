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

package cluster

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiterMC/socket.io"

	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/log"
	"github.com/LiterMC/go-openbmclapi/storage"
)

var (
	reFileHashMismatchError = regexp.MustCompile(` hash mismatch, expected ([0-9a-f]+), got ([0-9a-f]+)`)
)

type ClusterOptions struct {
	Id          string   `json:"id" yaml:"id"`
	Secret      string   `json:"secret" yaml:"secret"`
	PublicHosts []string `json:"public-hosts" yaml:"public-hosts"`
	Prefix      string   `json:"prefix" yaml:"prefix"`
}

type ClusterGeneralConfig struct {
	Host         string `json:"host" yaml:"host"`
	Port         uint16 `json:"port" yaml:"port"`
	Byoc         bool   `json:"byoc" yaml:"byoc"`
	NoFastEnable bool   `json:"no-fast-enable" yaml:"no-fast-enable"`
}

type Cluster struct {
	opts ClusterOptions
	gcfg ClusterGeneralConfig

	storageManager *storage.Manager
	storages       []int // the index of storages in the storage manager

	enableSignals []chan bool
	disableSignal chan struct{}

	mux    sync.RWMutex
	status atomic.Int32
	socket *socket.Socket
}

func NewCluster(
	opts ClusterOptions, gcfg ClusterGeneralConfig,
	storageManager *storage.Manager, storages []int,
) (cr *Cluster) {
	cr = &Cluster{
		opts: opts,
		gcfg: gcfg,

		storageManager: storageManager,
		storages:       storages,
	}
	return
}

// ID returns the cluster id
func (cr *Cluster) ID() string {
	return cr.opts.Id
}

// Host returns the cluster public host
func (cr *Cluster) Host() string {
	return cr.gcfg.Host
}

// Port returns the cluster public port
func (cr *Cluster) Port() uint16 {
	return cr.gcfg.Port
}

// PublicHosts returns the cluster public hosts
func (cr *Cluster) PublicHosts() []string {
	return cr.opts.PublicHosts
}

// Init do setup on the cluster
// Init should only be called once during the cluster's whole life
// The context passed in only affect the logical of Init method
func (cr *Cluster) Init(ctx context.Context) error {
	return nil
}

type EnableData struct {
	Host         string       `json:"host"`
	Port         uint16       `json:"port"`
	Version      string       `json:"version"`
	Byoc         bool         `json:"byoc"`
	NoFastEnable bool         `json:"noFastEnable"`
	Flavor       ConfigFlavor `json:"flavor"`
}

type ConfigFlavor struct {
	Runtime string `json:"runtime"`
	Storage string `json:"storage"`
}

// Enable send enable packet to central server
// The context passed in only affect the logical of Enable method
func (cr *Cluster) Enable(ctx context.Context) error {
	if cr.status.Load() == clusterEnabled {
		return nil
	}
	cr.mux.Lock()
	defer cr.mux.Unlock()
	if cr.status.Load() == clusterEnabled {
		return nil
	}
	defer func() {
		enabled := cr.Running()
		for _, ch := range cr.enableSignals {
			ch <- enabled
		}
		cr.enableSignals = cr.enableSignals[:0]
	}()
	oldStatus := cr.status.Swap(clusterEnabling)
	defer cr.status.CompareAndSwap(clusterEnabling, oldStatus)

	storageStr := cr.storageManager.GetFlavorString(cr.storages)

	log.TrInfof("info.cluster.enable.sending")
	resCh, err := cr.socket.EmitWithAck("enable", EnableData{
		Host:         cr.gcfg.Host,
		Port:         cr.gcfg.Port,
		Version:      build.ClusterVersion,
		Byoc:         cr.gcfg.Byoc,
		NoFastEnable: cr.gcfg.NoFastEnable,
		Flavor: ConfigFlavor{
			Runtime: "golang/" + runtime.GOOS + "-" + runtime.GOARCH,
			Storage: storageStr,
		},
	})
	if err != nil {
		return err
	}
	var data []any
	{
		tctx, cancel := context.WithTimeout(ctx, time.Minute*6)
		select {
		case data = <-resCh:
			cancel()
		case <-tctx.Done():
			cancel()
			return tctx.Err()
		}
	}
	log.Debug("got enable ack:", data)
	if ero := data[0]; ero != nil {
		if ero, ok := ero.(map[string]any); ok {
			if msg, ok := ero["message"].(string); ok {
				if hashMismatch := reFileHashMismatchError.FindStringSubmatch(msg); hashMismatch != nil {
					hash := hashMismatch[1]
					log.Warnf(Tr("warn.cluster.detected.hash.mismatch"), hash)
					cr.storageManager.RemoveForAll(hash)
				}
				return fmt.Errorf("Enable failed: %v", msg)
			}
		}
		return fmt.Errorf("Enable failed: %v", ero)
	}
	if v := data[1]; !v.(bool) {
		return fmt.Errorf("FATAL: Enable ack non true value, got (%T) %#v", v, v)
	}
	cr.disableSignal = make(chan struct{}, 0)
	log.TrInfof("info.cluster.enabled")
	cr.status.Store(clusterEnabled)
	return nil
}

// Disable send disable packet to central server
// The context passed in only affect the logical of Disable method
// Disable method is thread-safe, and it will wait until the first invoke exited
func (cr *Cluster) Disable(ctx context.Context) error {
	if cr.Enabled() {
		cr.mux.Lock()
		defer cr.mux.Unlock()
		if cr.Enabled() {
			defer close(cr.disableSignal)
			defer cr.status.Store(clusterDisabled)
			return cr.disable(ctx)
		}
	}
	cr.mux.RLock()
	disableCh := cr.disableSignal
	cr.mux.RUnlock()
	select {
	case <-disableCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// disable send disable packet to central server
// The context passed in only affect the logical of disable method
func (cr *Cluster) disable(ctx context.Context) error {
	log.TrInfof("info.cluster.disabling")
	resCh, err := cr.socket.EmitWithAck("disable", nil)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case data := <-resCh:
		log.Debug("disable ack:", data)
		if ero := data[0]; ero != nil {
			return fmt.Errorf("Disable failed: %v", ero)
		} else if !data[1].(bool) {
			return errors.New("Disable acked non true value")
		}
	}
	return nil
}

// markDisconnected marked the cluster as error or kicked
func (cr *Cluster) markDisconnected(kicked bool) {
	if !cr.Enabled() {
		return
	}
	cr.mux.Lock()
	defer cr.mux.Unlock()
	if cr.Enabled() {
		return
	}
	defer close(cr.disableSignal)

	var nextStatus int32
	if kicked {
		nextStatus = clusterKicked
	} else {
		nextStatus = clusterError
	}
	cr.status.Store(nextStatus)
}
