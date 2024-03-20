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
	"context"
	"database/sql"
	"time"

	"github.com/LiterMC/go-openbmclapi/log"
)

type SqlDB struct {
	db *sql.DB

	jtiStmts struct {
		get    *sql.Stmt
		add    *sql.Stmt
		remove *sql.Stmt
	}

	fileRecordStmts struct {
		get       *sql.Stmt
		has       *sql.Stmt
		setInsert *sql.Stmt
		setUpdate *sql.Stmt
		remove    *sql.Stmt
		forEach   *sql.Stmt
	}

	subscribeStmts struct {
		get                 *sql.Stmt
		has                 *sql.Stmt
		setInsert           *sql.Stmt
		setUpdate           *sql.Stmt
		setUpdateScopesOnly *sql.Stmt
		remove              *sql.Stmt
		removeUser          *sql.Stmt
		forEach             *sql.Stmt
	}

	jtiCleaner *time.Timer
}

var _ DB = (*SqlDB)(nil)

func NewSqlDB(driverName string, dataSourceName string) (db *SqlDB, err error) {
	ddb, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return
	}
	ddb.SetConnMaxLifetime(time.Minute * 3)
	ddb.SetMaxOpenConns(16)
	ddb.SetMaxIdleConns(16)

	db = &SqlDB{
		db: ddb,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.setup(ctx); err != nil {
		return
	}
	return
}

func (db *SqlDB) setup(ctx context.Context) (err error) {
	if err = db.db.PingContext(ctx); err != nil {
		return
	}

	if err = db.setupJTI(ctx); err != nil {
		return
	}

	if err = db.setupFileRecords(ctx); err != nil {
		return
	}

	if err = db.setupSubscribe(ctx); err != nil {
		return
	}
	return
}

func (db *SqlDB) Cleanup() (err error) {
	db.jtiCleaner.Stop()
	db.db.Close()
	return
}

func (db *SqlDB) setupJTI(ctx context.Context) (err error) {
	const tableName = "`token_id`"

	const createTable = "CREATE TABLE IF NOT EXISTS " + tableName + " (" +
		" `id` VARCHAR(256) NOT NULL," +
		" `expire` TIMESTAMP NOT NULL," +
		" PRIMARY KEY (`id`)" +
		")"
	if _, err = db.db.ExecContext(ctx, createTable); err != nil {
		return
	}

	const getSelectCmd = "SELECT 1 FROM " + tableName +
		" WHERE `id`=? AND `expire` > CURRENT_TIMESTAMP"
	if db.jtiStmts.get, err = db.db.PrepareContext(ctx, getSelectCmd); err != nil {
		return
	}

	const addInsertCmd = "INSERT INTO " + tableName +
		" (`id`,`expire`) VALUES" +
		" (?,?)"
	if db.jtiStmts.add, err = db.db.PrepareContext(ctx, addInsertCmd); err != nil {
		return
	}

	const removeDeleteCmd = "DELETE FROM " + tableName +
		" WHERE `id`=?"
	if db.jtiStmts.remove, err = db.db.PrepareContext(ctx, removeDeleteCmd); err != nil {
		return
	}

	const cleanDeleteCmd = "DELETE FROM " + tableName +
		" WHERE `expire` < CURRENT_TIMESTAMP"
	var cleanStmt *sql.Stmt
	if cleanStmt, err = db.db.PrepareContext(ctx, cleanDeleteCmd); err != nil {
		return
	}

	db.jtiCleaner = time.NewTimer(time.Minute * 10)
	go func(timer *time.Timer, cleanStmt *sql.Stmt) {
		defer cleanStmt.Close()
		for range timer.C {
			ctx, cancel := context.WithTimeout(ctx, time.Second*15)
			_, err := cleanStmt.ExecContext(ctx)
			cancel()
			if err != nil {
				log.Errorf("Error when cleaning expired tokens: %v", err)
			}
		}
	}(db.jtiCleaner, cleanStmt)
	return
}

func (db *SqlDB) ValidJTI(jti string) (has bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var has1 int
	if err = db.jtiStmts.get.QueryRowContext(ctx, jti).Scan(&has1); err != nil {
		return
	}
	if has1 == 0 {
		return false, nil
	}
	return true, nil
}

func (db *SqlDB) AddJTI(jti string, expire time.Time) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err = db.jtiStmts.add.ExecContext(ctx, jti, expire); err != nil {
		return
	}
	return
}

func (db *SqlDB) RemoveJTI(jti string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err = db.jtiStmts.remove.ExecContext(ctx, jti); err != nil {
		if err == sql.ErrNoRows {
			err = ErrNotFound
		}
		return
	}
	return
}

func (db *SqlDB) setupFileRecords(ctx context.Context) (err error) {
	const tableName = "`file_records`"

	const createTable = "CREATE TABLE IF NOT EXISTS " + tableName + " (" +
		" `path` VARCHAR(256) NOT NULL," +
		" `hash` VARCHAR(256) NOT NULL," +
		" `size` INTEGER NOT NULL," +
		" PRIMARY KEY (`path`)" +
		")"
	if _, err = db.db.ExecContext(ctx, createTable); err != nil {
		return
	}

	const getSelectCmd = "SELECT `hash`,`size` FROM " + tableName +
		" WHERE `path`=?"
	if db.fileRecordStmts.get, err = db.db.PrepareContext(ctx, getSelectCmd); err != nil {
		return
	}

	const hasSelectCmd = "SELECT 1 FROM " + tableName +
		" WHERE `path`=?"
	if db.fileRecordStmts.has, err = db.db.PrepareContext(ctx, hasSelectCmd); err != nil {
		return
	}

	const setInsertCmd = "INSERT INTO " + tableName +
		" (`path`,`hash`,`size`) VALUES" +
		" (?,?,?)"
	const setUpdateCmd = "UPDATE " + tableName + " SET" +
		" `hash`=?, `size`=?" +
		" WHERE `path`=?"
	if db.fileRecordStmts.setInsert, err = db.db.PrepareContext(ctx, setInsertCmd); err != nil {
		return
	}
	if db.fileRecordStmts.setUpdate, err = db.db.PrepareContext(ctx, setUpdateCmd); err != nil {
		return
	}

	const removeDeleteCmd = "DELETE FROM " + tableName +
		" WHERE `path`=?"
	if db.fileRecordStmts.remove, err = db.db.PrepareContext(ctx, removeDeleteCmd); err != nil {
		return
	}

	const forEachSelectCmd = "SELECT `path`,`hash`,`size` FROM " + tableName
	if db.fileRecordStmts.forEach, err = db.db.PrepareContext(ctx, forEachSelectCmd); err != nil {
		return
	}
	return err
}

func (db *SqlDB) GetFileRecord(path string) (rec *FileRecord, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	rec = new(FileRecord)
	rec.Path = path
	if err = db.fileRecordStmts.get.QueryRowContext(ctx, path).Scan(&rec.Hash, &rec.Size); err != nil {
		if err == sql.ErrNoRows {
			err = ErrNotFound
		}
		return
	}
	return
}

func (db *SqlDB) SetFileRecord(rec FileRecord) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var has int
	if err = tx.Stmt(db.fileRecordStmts.has).QueryRow(rec.Path).Scan(&has); err != nil && err != sql.ErrNoRows {
		return
	}
	if has == 0 {
		if _, err = tx.Stmt(db.fileRecordStmts.setInsert).Exec(rec.Path, rec.Hash, rec.Size); err != nil {
			return
		}
	} else {
		if _, err = tx.Stmt(db.fileRecordStmts.setUpdate).Exec(rec.Hash, rec.Size, rec.Path); err != nil {
			return
		}
	}
	if err = tx.Commit(); err != nil {
		return
	}
	return
}

func (db *SqlDB) RemoveFileRecord(path string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err = db.fileRecordStmts.remove.ExecContext(ctx, path); err != nil {
		if err == sql.ErrNoRows {
			err = ErrNotFound
		}
		return
	}
	return
}

func (db *SqlDB) ForEachFileRecord(cb func(*FileRecord) error) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var rows *sql.Rows
	if rows, err = db.fileRecordStmts.remove.QueryContext(ctx); err != nil {
		return
	}
	defer rows.Close()
	var rec FileRecord
	for rows.Next() {
		if err = rows.Scan(&rec.Path, &rec.Hash, &rec.Size); err != nil {
			return
		}
		cb(&rec)
	}
	if err = rows.Err(); err != nil {
		return
	}
	return
}

func (db *SqlDB) setupSubscribe(ctx context.Context) (err error) {
	const tableName = "`subscribes`"

	const createTable = "CREATE TABLE IF NOT EXISTS " + tableName + " (" +
		" `user` VARCHAR(128) NOT NULL," +
		" `client` VARCHAR(128) NOT NULL," +
		" `endpoint` VARCHAR(256) NOT NULL," +
		" `scopes` INTEGER NOT NULL," +
		" PRIMARY KEY (`user`,`client`)" +
		")"
	if _, err = db.db.ExecContext(ctx, createTable); err != nil {
		return
	}

	const getSelectCmd = "SELECT `endpoint`,`scopes` FROM " + tableName +
		" WHERE `user`=? AND `client`=?"
	if db.subscribeStmts.get, err = db.db.PrepareContext(ctx, getSelectCmd); err != nil {
		return
	}

	const hasSelectCmd = "SELECT 1 FROM " + tableName +
		" WHERE `user`=? AND `client`=?"
	if db.subscribeStmts.has, err = db.db.PrepareContext(ctx, hasSelectCmd); err != nil {
		return
	}

	const setInsertCmd = "INSERT INTO " + tableName +
		" (`user`,`client`,`endpoint`,`scopes`) VALUES" +
		" (?,?,?,?)"
	const setUpdateCmd = "UPDATE " + tableName + " SET" +
		" `endpoint`=?, `scopes`=?" +
		" WHERE `user`=? AND `client`=?"
	const setUpdateScopesOnlyCmd = "UPDATE " + tableName + " SET" +
		" scopes`=?" +
		" WHERE `user`=? AND `client`=?"
	if db.subscribeStmts.setInsert, err = db.db.PrepareContext(ctx, setInsertCmd); err != nil {
		return
	}
	if db.subscribeStmts.setUpdate, err = db.db.PrepareContext(ctx, setUpdateCmd); err != nil {
		return
	}
	if db.subscribeStmts.setUpdateScopesOnly, err = db.db.PrepareContext(ctx, setUpdateScopesOnlyCmd); err != nil {
		return
	}

	const removeDeleteCmd = "DELETE FROM " + tableName +
		" WHERE `user`=? AND `client`=?"
	if db.subscribeStmts.remove, err = db.db.PrepareContext(ctx, removeDeleteCmd); err != nil {
		return
	}

	const removeUserDeleteCmd = "DELETE FROM " + tableName +
		" WHERE `user`=?"
	if db.subscribeStmts.removeUser, err = db.db.PrepareContext(ctx, removeUserDeleteCmd); err != nil {
		return
	}

	const forEachSelectCmd = "SELECT `user`,`client`,`endpoint`,`scopes` FROM " + tableName
	if db.subscribeStmts.forEach, err = db.db.PrepareContext(ctx, forEachSelectCmd); err != nil {
		return
	}
	return err
}

func (db *SqlDB) GetSubscribe(user string, client string) (rec *SubscribeRecord, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	rec = new(SubscribeRecord)
	rec.User = user
	rec.Client = client
	if err = db.subscribeStmts.get.QueryRowContext(ctx, user, client).Scan(&rec.EndPoint, &rec.Scopes); err != nil {
		if err == sql.ErrNoRows {
			err = ErrNotFound
		}
		return
	}
	return
}

func (db *SqlDB) SetSubscribe(rec SubscribeRecord) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if rec.EndPoint == "" {
		if _, err = tx.Stmt(db.subscribeStmts.setUpdateScopesOnly).Exec(rec.Scopes, rec.User, rec.Client); err != nil {
			if err == sql.ErrNoRows {
				err = ErrNotFound
			}
			return
		}
	} else {
		var has int
		if err = tx.Stmt(db.subscribeStmts.has).QueryRow(rec.User, rec.Client).Scan(&has); err != nil && err != sql.ErrNoRows {
			return
		}
		if has == 0 {
			if _, err = tx.Stmt(db.subscribeStmts.setInsert).Exec(rec.User, rec.Client, rec.EndPoint, rec.Scopes); err != nil {
				return
			}
		} else {
			if _, err = tx.Stmt(db.subscribeStmts.setUpdate).Exec(rec.EndPoint, rec.Scopes, rec.User, rec.Client); err != nil {
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return
	}
	return
}

func (db *SqlDB) RemoveSubscribe(user string, client string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err = db.subscribeStmts.remove.ExecContext(ctx, user, client); err != nil {
		if err == sql.ErrNoRows {
			err = ErrNotFound
		}
		return
	}
	return
}

func (db *SqlDB) ForEachSubscribe(cb func(*SubscribeRecord) error) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var rows *sql.Rows
	if rows, err = db.subscribeStmts.remove.QueryContext(ctx); err != nil {
		return
	}
	defer rows.Close()
	var rec SubscribeRecord
	for rows.Next() {
		if err = rows.Scan(&rec.User, &rec.Client, &rec.EndPoint, &rec.Scopes); err != nil {
			return
		}
		cb(&rec)
	}
	if err = rows.Err(); err != nil {
		return
	}
	return
}
