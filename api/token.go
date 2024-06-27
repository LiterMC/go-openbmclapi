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
	"net/url"
)

type TokenVerifier interface {
	VerifyAuthToken(clientId string, token string) (tokenId string, userId string, err error)
	VerifyAPIToken(clientId string, token string, path string, query url.Values) (userId string, err error)
}

type TokenManager interface {
	TokenVerifier
	GenerateAuthToken(clientId string, userId string) (token string, err error)
	GenerateAPIToken(clientId string, userId string, path string, query map[string]string) (token string, err error)
}
