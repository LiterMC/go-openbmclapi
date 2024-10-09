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

package notify

import (
	"time"

	"github.com/LiterMC/go-openbmclapi/cluster"
	"github.com/LiterMC/go-openbmclapi/update"
)

type TimestampEvent struct {
	At time.Time
}

type (
	EnabledEvent TimestampEvent

	DisabledEvent TimestampEvent

	SyncBeginEvent struct {
		TimestampEvent
		Count int
		Size  int64
	}

	SyncDoneEvent TimestampEvent

	UpdateAvaliableEvent struct {
		Release *update.GithubRelease
	}

	ReportStatusEvent struct {
		TimestampEvent
		Stats *cluster.StatManager
	}
)
