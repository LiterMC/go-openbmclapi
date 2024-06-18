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

package log

import (
	"github.com/LiterMC/go-openbmclapi/lang"
)

func TrDebugf(key string, vals ...any) {
	Debugf(lang.Tr(key), vals...)
}

func TrInfof(key string, vals ...any) {
	Infof(lang.Tr(key), vals...)
}

func TrWarnf(key string, vals ...any) {
	Warnf(lang.Tr(key), vals...)
}

func TrErrorf(key string, vals ...any) {
	Errorf(lang.Tr(key), vals...)
}

func TrPanicf(key string, vals ...any) {
	Panicf(lang.Tr(key), vals...)
}
