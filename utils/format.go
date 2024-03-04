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
	"strconv"
	"strings"
)

func SplitCSV(line string) (values map[string]float32) {
	list := strings.Split(line, ",")
	values = make(map[string]float32, len(list))
	for _, v := range list {
		name, opt, _ := strings.Cut(strings.ToLower(strings.TrimSpace(v)), ";")
		var q float64 = 1
		if v, ok := strings.CutPrefix(opt, "q="); ok {
			q, _ = strconv.ParseFloat(v, 32)
		}
		values[name] = (float32)(q)
	}
	return
}

const byteUnits = "KMGTPE"

func BytesToUnit(size float64) string {
	if size < 1000 {
		return fmt.Sprintf("%dB", (int)(size))
	}
	var unit rune
	for _, u := range byteUnits {
		unit = u
		size /= 1024
		if size < 1000 {
			break
		}
	}
	return fmt.Sprintf("%.1f%sB", size, string(unit))
}

func ParseCacheControl(str string) (exp int64, ok bool) {
	for _, v := range strings.Split(strings.ToLower(str), ",") {
		v = strings.TrimSpace(v)
		switch v {
		case "private":
			fallthrough
		case "no-store":
			return 0, false
		case "no-cache":
			exp = 0
		default:
			if maxAge, is := strings.CutPrefix(v, "max-age="); is {
				n, err := strconv.ParseInt(maxAge, 10, 64)
				if err != nil {
					return
				}
				exp = n
			}
		}
	}
	ok = true
	return
}
