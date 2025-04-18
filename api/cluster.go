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

package api

import (
	"context"
)

type ClusterStatus int32

const (
	ClusterDisconnected = iota
	ClusterConnecting
	ClusterDisabled
	ClusterEnabling
	ClusterEnabled
)


// Disconnected returns true if the cluster is disconnected from the central server
func (s ClusterStatus) Disconnected() bool {
	return s <= ClusterConnecting
}

// Connected returns true if the cluster is connected to the central server
func (s ClusterStatus) Connected() bool {
	return s > ClusterConnecting
}

// Enabled returns true if the cluster is enabled or enabling
func (s ClusterStatus) Enabled() bool {
	return s >= ClusterEnabling
}

// Running returns true if the cluster is completely enabled
func (s ClusterStatus) Running() bool {
	return s == ClusterEnabled
}

type Cluster interface {
	Name() string
	ID() string
	Secret() string
	Host() string
	Port() uint16
	PublicHosts() []string

	Status() ClusterStatus
	Connect(context.Context) error
	Disconnect(context.Context) error
	Enable(context.Context) error
	Disable(ctx context.Context) error
}

type ClusterManager interface {
	//
}
