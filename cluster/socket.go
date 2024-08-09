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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/LiterMC/socket.io"
	"github.com/LiterMC/socket.io/engine.io"

	"github.com/LiterMC/go-openbmclapi/internal/build"
	"github.com/LiterMC/go-openbmclapi/log"
)

// Connect connects to the central server
// The context passed in only affect the logical of Connect method
// Connection will not be closed after disable
//
// See Disconnect
func (cr *Cluster) Connect(ctx context.Context) error {
	if !cr.Disconnected() {
		return errors.New("Attempt to connect while connecting")
	}
	_, err := cr.GetAuthToken(ctx)
	if err != nil {
		return fmt.Errorf("Auth failed %w", err)
	}

	engio, err := engine.NewSocket(engine.Options{
		Host: cr.opts.Prefix,
		Path: "/socket.io/",
		ExtraHeaders: http.Header{
			"Origin":     {cr.opts.Prefix},
			"User-Agent": {build.ClusterUserAgent},
		},
		DialTimeout: time.Minute * 6,
	})
	if err != nil {
		return fmt.Errorf("Could not parse Engine.IO options: %w", err)
	}
	if ctx.Value("cluster.options.engine-io.debug") == true {
		engio.OnRecv(func(s *engine.Socket, data []byte) {
			log.Debugf("Engine.IO %s recv: %q", s.ID(), (string)(data))
		})
		engio.OnSend(func(s *engine.Socket, data []byte) {
			log.Debugf("Engine.IO %s send: %q", s.ID(), (string)(data))
		})
	}
	engio.OnConnect(func(s *engine.Socket) {
		log.Info("Engine.IO %s connected for cluster %s", s.ID(), cr.ID())
	})
	engio.OnDisconnect(cr.onDisconnected)
	engio.OnDialError(func(s *engine.Socket, err *engine.DialErrorContext) {
		if err.Count() < 0 {
			return
		}
		log.TrErrorf("error.cluster.connect.failed", cr.ID(), err.Count(), cr.gcfg.MaxReconnectCount, err.Err())
		if cr.gcfg.MaxReconnectCount >= 0 && err.Count() >= cr.gcfg.MaxReconnectCount {
			log.TrErrorf("error.cluster.connect.failed.toomuch", cr.ID())
			s.Close()
		}
	})
	log.Infof("Dialing %s for cluster %s", engio.URL().String(), cr.ID())
	if err := engio.Dial(ctx); err != nil {
		return fmt.Errorf("Dial error: %w", err)
	}

	cr.socket = socket.NewSocket(engio, socket.WithAuthTokenFn(func() (string, error) {
		token, err := cr.GetAuthToken(ctx)
		if err != nil {
			log.TrErrorf("error.cluster.auth.failed", err)
			return "", err
		}
		return token, nil
	}))
	cr.socket.OnError(func(_ *socket.Socket, err error) {
		log.Errorf("Socket.IO error: %v", err)
	})
	cr.socket.OnMessage(func(event string, data []any) {
		if event == "message" {
			log.Infof("[remote]: %v", data[0])
		}
	})
	log.Info("Connecting to socket.io namespace")
	if err := cr.socket.Connect(""); err != nil {
		return fmt.Errorf("Namespace connect error: %w", err)
	}
	return nil
}

// Disconnect close the connection which connected to the central server
// Disconnect will not disable the cluster
//
// See Connect
func (cr *Cluster) Disconnect() error {
	if cr.Disconnected() {
		return nil
	}
	cr.mux.Lock()
	defer cr.mux.Unlock()
	err := cr.socket.Close()
	cr.socketStatus.Store(socketDisconnected)
	cr.socket = nil
	return err
}

func (cr *Cluster) onDisconnected(s *engine.Socket, err error) {
	if err != nil {
		log.Warnf("Engine.IO %s disconnected: %v", s.ID(), err)
	}
	cr.socketStatus.Store(socketDisconnected)
	cr.socket = nil
}
