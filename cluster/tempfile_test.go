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

package cluster_test

import (
	"testing"

	"io"
	"os"
)

var datas = func() [][]byte {
	datas := make([][]byte, 0x7)
	for i := range len(datas) {
		b := make([]byte, 0xff00+i)
		for j := range len(b) {
			b[j] = (byte)(i + j)
		}
		datas[i] = b
	}
	return datas
}()

func BenchmarkCreateAndRemoveFile(b *testing.B) {
	b.ReportAllocs()
	buf := make([]byte, 1024)
	_ = buf
	for i := 0; i < b.N; i++ {
		d := datas[i%len(datas)]
		fd, err := os.CreateTemp("", "*.downloading")
		if err != nil {
			b.Fatalf("Cannot create temp file: %v", err)
		}
		if _, err = fd.Write(d); err != nil {
			b.Errorf("Cannot write file: %v", err)
		} else if err = fd.Sync(); err != nil {
			b.Errorf("Cannot write file: %v", err)
		}
		fd.Close()
		os.Remove(fd.Name())
		if err != nil {
			b.FailNow()
		}
	}
}

func BenchmarkWriteAndTruncateFile(b *testing.B) {
	b.ReportAllocs()
	buf := make([]byte, 1024)
	_ = buf
	fd, err := os.CreateTemp("", "*.downloading")
	if err != nil {
		b.Fatalf("Cannot create temp file: %v", err)
	}
	defer os.Remove(fd.Name())
	for i := 0; i < b.N; i++ {
		d := datas[i%len(datas)]
		if _, err := fd.Write(d); err != nil {
			b.Fatalf("Cannot write file: %v", err)
		} else if err := fd.Sync(); err != nil {
			b.Fatalf("Cannot write file: %v", err)
		} else if err := fd.Truncate(0); err != nil {
			b.Fatalf("Cannot truncate file: %v", err)
		}
	}
}

func BenchmarkWriteAndSeekFile(b *testing.B) {
	b.ReportAllocs()
	buf := make([]byte, 1024)
	_ = buf
	fd, err := os.CreateTemp("", "*.downloading")
	if err != nil {
		b.Fatalf("Cannot create temp file: %v", err)
	}
	defer os.Remove(fd.Name())
	for i := 0; i < b.N; i++ {
		d := datas[i%len(datas)]
		if _, err := fd.Write(d); err != nil {
			b.Fatalf("Cannot write file: %v", err)
		} else if err := fd.Sync(); err != nil {
			b.Fatalf("Cannot write file: %v", err)
		} else if _, err := fd.Seek(io.SeekStart, 0); err != nil {
			b.Fatalf("Cannot seek file: %v", err)
		}
	}
}

func BenchmarkParallelCreateAndRemoveFile(b *testing.B) {
	b.ReportAllocs()
	b.SetParallelism(4)
	buf := make([]byte, 1024)
	_ = buf
	b.RunParallel(func(pb *testing.PB) {
		for i := 0; pb.Next(); i++ {
			d := datas[i%len(datas)]
			fd, err := os.CreateTemp("", "*.downloading")
			if err != nil {
				b.Fatalf("Cannot create temp file: %v", err)
			}
			if _, err = fd.Write(d); err != nil {
				b.Errorf("Cannot write file: %v", err)
			} else if err = fd.Sync(); err != nil {
				b.Errorf("Cannot write file: %v", err)
			}
			fd.Close()
			if err := os.Remove(fd.Name()); err != nil {
				b.Fatalf("Cannot remove file: %v", err)
			}
			if err != nil {
				b.FailNow()
			}
		}
	})
}

func BenchmarkParallelWriteAndTruncateFile(b *testing.B) {
	b.ReportAllocs()
	b.SetParallelism(4)
	buf := make([]byte, 1024)
	_ = buf
	b.RunParallel(func(pb *testing.PB) {
		fd, err := os.CreateTemp("", "*.downloading")
		if err != nil {
			b.Fatalf("Cannot create temp file: %v", err)
		}
		defer os.Remove(fd.Name())
		for i := 0; pb.Next(); i++ {
			d := datas[i%len(datas)]
			if _, err := fd.Write(d); err != nil {
				b.Fatalf("Cannot write file: %v", err)
			} else if err := fd.Sync(); err != nil {
				b.Fatalf("Cannot write file: %v", err)
			} else if err := fd.Truncate(0); err != nil {
				b.Fatalf("Cannot truncate file: %v", err)
			}
		}
	})
}

func BenchmarkParallelWriteAndSeekFile(b *testing.B) {
	b.ReportAllocs()
	b.SetParallelism(4)
	buf := make([]byte, 1024)
	_ = buf
	b.RunParallel(func(pb *testing.PB) {
		fd, err := os.CreateTemp("", "*.downloading")
		if err != nil {
			b.Fatalf("Cannot create temp file: %v", err)
		}
		defer os.Remove(fd.Name())
		for i := 0; pb.Next(); i++ {
			d := datas[i%len(datas)]
			if _, err := fd.Write(d); err != nil {
				b.Fatalf("Cannot write file: %v", err)
			} else if err := fd.Sync(); err != nil {
				b.Fatalf("Cannot write file: %v", err)
			} else if _, err := fd.Seek(io.SeekStart, 0); err != nil {
				b.Fatalf("Cannot seel file: %v", err)
			}
		}
	})
}
