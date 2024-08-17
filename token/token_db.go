/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
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

package token

import (
	"time"

	"github.com/LiterMC/go-openbmclapi/api"
	"github.com/LiterMC/go-openbmclapi/database"
)

type DBManager struct {
	basicTokenManager
	db         database.DB
	issuer     string
	apiHmacKey []byte
}

var _ api.TokenManager = (*DBManager)(nil)

func NewDBManager(issuer string, apiHmacKey []byte, db database.DB) *DBManager {
	m := &DBManager{
		db:         db,
		issuer:     issuer,
		apiHmacKey: apiHmacKey,
	}
	m.basicTokenManager.impl = m
	return m
}

func (m *DBManager) Issuer() string {
	return m.issuer
}

func (m *DBManager) HmacKey() []byte {
	return m.apiHmacKey
}

func (m *DBManager) AddJTI(id string, expire time.Time) error {
	return m.db.AddJTI(id, expire)
}

func (m *DBManager) ValidJTI(id string) bool {
	ok, _ := m.db.ValidJTI(id)
	return ok
}

func (m *DBManager) InvalidToken(id string) error {
	return m.db.RemoveJTI(id)
}
