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

package log_test

import (
	"testing"

	"bytes"

	"github.com/LiterMC/go-openbmclapi/log"
)

func TestProxiedStdLog(t *testing.T) {
	log.AddStdLogFilter(func(line []byte) bool {
		return bytes.HasPrefix(line, ([]byte)("http: TLS handshake error"))
	})
	log.ProxiedStdLog.Printf("some messages log with printf")
	log.ProxiedStdLog.Println("some messages\n that cross multiple\n lines")
	log.ProxiedStdLog.Printf("http: TLS handshake error from %s", "127.0.0.1")
	log.ProxiedStdLog.Printf("the next message")
	log.ProxiedStdLog.Printf("http: crosslines log\nhttp: TLS handshake error from %s", "127.0.0.1")
}
