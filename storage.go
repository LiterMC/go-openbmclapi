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

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Storage interface {
	fmt.Stringer

	// Options should return the pointer of the storage options
	//  which should be able to marshal/unmarshal with yaml format
	Options() any
	// SetOptions will be called with the same type of the Options() result
	SetOptions(any)
	// Init will be called before start to use a storage
	Init() error

	Size(hash string) (int64, error)
	Open(hash string) (io.ReadCloser, error)
	Create(hash string) (io.WriteCloser, error)
	Remove(hash string) error
	WalkDir(func(hash string) error) error

	ServeDownload(rw http.ResponseWriter, req *http.Request, hash string) error
	ServeMeasure(rw http.ResponseWriter, req *http.Request, size int) error
}

const (
	StorageLocal  = "local"
	StorageMount  = "mount"
	StorageWebdav = "webdav"
)

type StorageFactory struct {
	New       func() any
	NewConfig func() any
}

var storageFactories = make(map[string]func() any, 3)

func AddStorageFactory(typ string, inst StorageFactory) {
	if inst.New == nil || inst.NewConfig == nil {
		panic("nil function")
	}
	if _, ok := storageFactories[typ]; ok {
		panic(fmt.Errorf("Storage %q is already exists", typ))
	}
	storageFactories[typ] = inst
}

type UnexpectedStorageTypeError struct {
	Type string
}

func (e *UnexpectedStorageTypeError) Error() string {
	types := make([]string, 0, len(storageFactories))
	for t, _ := range storageFactories {
		types = append(types, t)
	}
	sort.Strings(types)
	return fmt.Errorf("Unexpected storage type %q, must be one of %s", e.Type, strings.Join(",", types))
}

type StorageOption struct {
	Type   string
	Weight uint
	Data   any
}

func (o *StorageOption) UnmarshalYAML(n *yaml.Node) (err error) {
	var basicOpts struct {
		Type   string `yaml:"type"`
		Weight uint   `yaml:"weight"`
	}
	if err = n.Decode(&basicOpts); err != nil {
		return
	}
	o.Type = basicOpts.Type
	o.Weight = basicOpts.Weight
	f, ok := storageFactories[o.Type]
	if !ok {
		return &UnexpectedStorageTypeError{o.Type}
	}
	o.Data = f.NewConfig()
	return n.Decode(o.Data)
}
