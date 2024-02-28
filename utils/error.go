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

package utils

import (
	"fmt"
	"net/http"
)

type HTTPStatusError struct {
	Code    int
	URL     string
	Message string
}

func NewHTTPStatusErrorFromResponse(res *http.Response) (e *HTTPStatusError) {
	e = &HTTPStatusError{
		Code: res.StatusCode,
	}
	if res.Request != nil {
		e.URL = res.Request.URL.String()
	}
	var buf [512]byte
	n, _ := res.Body.Read(buf[:])
	e.Message = (string)(buf[:n])
	return
}

func (e *HTTPStatusError) Error() string {
	s := fmt.Sprintf("Unexpected http status %d %s", e.Code, http.StatusText(e.Code))
	if e.URL != "" {
		s += " for " + e.URL
	}
	if e.Message != "" {
		s += ":\n\t" + e.Message
	}
	return s
}
