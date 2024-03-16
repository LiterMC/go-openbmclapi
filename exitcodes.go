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

package main

const (
	CodeClientError      = 0x01
	CodeServerError      = 0x02
	CodeEnvironmentError = 0x04
	CodeUnexpectedError  = 0x08
)

const (
	CodeClientOrServerError     = CodeClientError | CodeServerError
	CodeClientOrEnvionmentError = CodeClientError | CodeEnvironmentError
	CodeClientUnexpectedError   = CodeUnexpectedError | CodeClientError
	CodeServerOrEnvionmentError = CodeServerError | CodeEnvironmentError
	CodeServerUnexpectedError   = CodeUnexpectedError | CodeServerError
)
