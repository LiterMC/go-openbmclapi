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
	"github.com/LiterMC/go-openbmclapi/api"
)

func (cr *Cluster) Status() api.ClusterStatus {
	return (api.ClusterStatus)(cr.status.Load())
}

// Disabled returns true if the cluster is disabled manually
func (cr *Cluster) Disabled() bool {
	return cr.Status() == api.ClusterDisabled && !cr.shouldEnable.Load()
}

// IsKicked returns true if the cluster is kicked by the central server
func (cr *Cluster) IsKicked() bool {
	return cr.Status() == api.ClusterDisabled && cr.shouldEnable.Load()
}

// WaitForEnable returns a channel which receives true when cluster enabled succeed, or receives false when it failed to enable
// If the cluster is already enable, the channel always returns true
// The channel should not be used multiple times
func (cr *Cluster) WaitForEnable() <-chan bool {
	cr.mux.Lock()
	defer cr.mux.Unlock()
	ch := make(chan bool, 1)
	if cr.Status().Running() {
		ch <- true
	} else {
		cr.enableSignals = append(cr.enableSignals, ch)
	}
	return ch
}
