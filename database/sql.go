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

	getStmt       *sql.Stmt
	hasStmt       *sql.Stmt
	setInsertStmt *sql.Stmt
	setUpdateStmt *sql.Stmt
	removeStmt    *sql.Stmt
	forEachStmt   *sql.Stmt
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
	if db.getStmt, err = db.db.PrepareContext(ctx, getSelectCmd); err != nil {
		return
	}

	const hasSelectCmd = "SELECT 1 FROM " + tableName +
		" WHERE `path`=?"
	if db.hasStmt, err = db.db.PrepareContext(ctx, hasSelectCmd); err != nil {
		return
	}

	const setInsertCmd = "INSERT INTO " + tableName +
		" (`path`,`hash`,`size`) VALUES" +
		" (?,?,?)"
	const setUpdateCmd = "UPDATE " + tableName + " SET" +
		" `hash`=?, `size`=?" +
		" WHERE `path`=?"
	if db.setInsertStmt, err = db.db.PrepareContext(ctx, setInsertCmd); err != nil {
		return
	}
	if db.setUpdateStmt, err = db.db.PrepareContext(ctx, setUpdateCmd); err != nil {
		return
	}

	const removeDeleteCmd = "DELETE FROM " + tableName +
		" WHERE `path`=?"
	if db.removeStmt, err = db.db.PrepareContext(ctx, removeDeleteCmd); err != nil {
		return
	}

	const forEachSelectCmd = "SELECT `path`,`hash`,`size` FROM " + tableName
	if db.forEachStmt, err = db.db.PrepareContext(ctx, forEachSelectCmd); err != nil {
		return
	}
	return err
}

func (db *SqlDB) Get(path string) (rec *Record, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	rec = new(Record)
	if err = db.getStmt.QueryRowContext(ctx, path).Scan(&rec.Hash, &rec.Size); err != nil {
		if err == sql.ErrNoRows {
			err = ErrNotFound
		}
		return
	}
	return
}

func (db *SqlDB) Set(rec Record) (err error) {
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
	if err = tx.Stmt(db.hasStmt).QueryRow(rec.Path).Scan(has); err != nil {
		return
	}
	if has == 0 {
		if _, err = tx.Stmt(db.setInsertStmt).Exec(rec.Path, rec.Hash, rec.Size); err != nil {
			return
		}
	} else {
		if _, err = tx.Stmt(db.setUpdateStmt).Exec(rec.Hash, rec.Size, rec.Path); err != nil {
			return
		}
	}
	if err = tx.Commit(); err != nil {
		return
	}
	return
}

func (db *SqlDB) Remove(path string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err = db.removeStmt.ExecContext(ctx, path); err != nil {
		if err == sql.ErrNoRows {
			err = ErrNotFound
		}
		return
	}
	return
}

func (db *SqlDB) ForEach(cb func(*Record) error) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var rows *sql.Rows
	if rows, err = db.removeStmt.QueryContext(ctx); err != nil {
		return
	}
	defer rows.Close()
	var rec Record
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
