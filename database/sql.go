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
	return
}

func (db *SqlDB) ValidJTI(jti string) (has bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var has1 int
	if err = db.jtiStmts.get.QueryRowContext(ctx, jti).Scan(has1); err != nil {
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
	if err = tx.Stmt(db.fileRecordStmts.has).QueryRow(rec.Path).Scan(has); err != nil {
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
