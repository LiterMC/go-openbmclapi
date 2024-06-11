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
	"sync/atomic"
)

type Cluster struct {
	id     string
	secret string
	host   string
	port   uint16

	storageManager *storage.Manager
	storages       []int // the index of storages in the storage manager

	status atomic.Int32
}

// ID returns the cluster id
func (cr *Cluster) ID() string {
	return cr.id
}

// Host returns the cluster public host
func (cr *Cluster) Host() string {
	return cr.host
}

// Port returns the cluster public port
func (cr *Cluster) Port() string {
	return cr.port
}

// Init do setup on the cluster
// Init should only be called once during the cluster's whole life
// The context passed in only affect the logical of Init method
func (cr *Cluster) Init(ctx context.Context) error {
	return
}

// Enable send enable packet to central server
// The context passed in only affect the logical of Enable method
func (cr *Cluster) Enable(ctx context.Context) error {
	return
}

// Disable send disable packet to central server
// The context passed in only affect the logical of Disable method
func (cr *Cluster) Disable(ctx context.Context) error {
	return
}

// setDisabled marked the cluster as disabled or kicked
func (cr *Cluster) setDisabled(kicked bool) {
	return
}
