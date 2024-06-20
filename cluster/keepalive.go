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
	"time"
)

type KeepAliveRes int

const (
	KeepAliveSucceed KeepAliveRes = iota
	KeepAliveFailed
	KeepAliveKicked
)

type keepAliveReq struct {
	Time  string `json:"time"`
	Hits  int32  `json:"hits"`
	Bytes int64  `json:"bytes"`
}

// KeepAlive will send the keep-alive packet and fresh hits & hit bytes data
// If cluster is kicked by the central server, the cluster status will be mark as kicked
func (cr *Cluster) KeepAlive(ctx context.Context) KeepAliveRes {
	hits, hbts := cr.hits.Load(), cr.hbts.Load()
	resCh, err := cr.socket.EmitWithAck("keep-alive", keepAliveReq{
		Time:  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Hits:  hits,
		Bytes: hbts,
	})

	if e := cr.stats.Save(cr.dataDir); e != nil {
		log.Errorf(Tr("error.cluster.stat.save.failed"), e)
	}
	if err != nil {
		log.Errorf(Tr("error.cluster.keepalive.send.failed"), err)
		return KeepAliveFailed
	}
	var data []any
	select {
	case <-ctx.Done():
		return KeepAliveFailed
	case data = <-resCh:
	}
	log.Debugf("Keep-alive response: %v", data)
	if ero := data[0]; len(data) <= 1 || ero != nil {
		if ero, ok := ero.(map[string]any); ok {
			if msg, ok := ero["message"].(string); ok {
				log.Errorf(Tr("error.cluster.keepalive.failed"), msg)
				if hashMismatch := reFileHashMismatchError.FindStringSubmatch(msg); hashMismatch != nil {
					hash := hashMismatch[1]
					log.Warnf("Detected hash mismatch error, removing bad file %s", hash)
					for _, s := range cr.storages {
						go s.Remove(hash)
					}
				}
				return KeepAliveFailed
			}
		}
		log.Errorf(Tr("error.cluster.keepalive.failed"), ero)
		return KeepAliveFailed
	}
	log.Infof(Tr("info.cluster.keepalive.success"), ahits, utils.BytesToUnit((float64)(ahbts)), data[1])
	cr.hits.Add(-hits2)
	cr.hbts.Add(-hbts2)
	if data[1] == false {
		cr.markKicked()
		return KeepAliveKicked
	}
	return KeepAliveSucceed
}
