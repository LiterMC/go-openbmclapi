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

package database

import (
	"errors"
)

var (
	ErrStopIter = errors.New("stop iteration")
	ErrNotFound = errors.New("no record was found")
)

type Record struct {
	Path string
	Hash string
	Size int64
}

type DB interface {
	// You should not edit the record pointer
	Get(path string) (*Record, error)
	Set(Record) error
	Remove(path string) error

	// if the callback returns ErrStopIter, ForEach must immediately stop and returns a nil error
	// the callback should not edit the record pointer
	ForEach(cb func(*Record) error) error
}
