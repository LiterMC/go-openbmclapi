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

type UserManager interface {
	GetUsers() []*User
	GetUser(id string) *User
	AddUser(*User) error
	RemoveUser(id string) error
	UpdateUserPassword(username string, password string) error
	UpdateUserPermissions(username string, permissions PermissionFlag) error

	VerifyUserPassword(userId string, comparator func(password string) bool) error
}

type PermissionFlag uint32

const (
	// BasicPerm includes majority client side actions, such as login, which do not have a significant impact on the server
	BasicPerm PermissionFlag = 1 << iota
	// SubscribePerm allows the user to subscribe server status & other posts
	SubscribePerm
	// LogPerm allows the user to view non-debug logs & download access logs
	LogPerm
	// DebugPerm allows the user to access debug settings and download debug logs
	DebugPerm
	// FullConfigPerm allows the user to access all config values
	FullConfigPerm
	// ClusterPerm allows the user to configure clusters' settings & stop/start clusters
	ClusterPerm
	// StoragePerm allows the user to configure storages' settings & decides to manually start storages' sync process
	StoragePerm
	// BypassLimitPerm allows the user to ignore API access limit
	BypassLimitPerm
	// RootPerm user can add/remove users, reset their password, and change their permission flags
	RootPerm PermissionFlag = 1 << 31

	AllPerm = ^(PermissionFlag)(0)
)

type User struct {
	Username string
	Password string // as sha256
	Permissions PermissionFlag
}
