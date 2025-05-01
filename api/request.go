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
	"net/http"
)

const (
	RealAddrCtxKey       = "handle.real.addr"
	RealPathCtxKey       = "handle.real.path"
	AccessLogExtraCtxKey = "handle.access.extra"
)

func GetRequestRealAddr(req *http.Request) string {
	addr, _ := req.Context().Value(RealAddrCtxKey).(string)
	return addr
}

func GetRequestRealPath(req *http.Request) string {
	return req.Context().Value(RealPathCtxKey).(string)
}

func SetAccessInfo(req *http.Request, key string, value any) {
	if info, ok := req.Context().Value(AccessLogExtraCtxKey).(map[string]any); ok {
		info[key] = value
	}
}
