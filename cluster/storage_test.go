/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2024 Kevin Z <zyxkad@gmail.com>
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

	"bytes"
	"crypto"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
)

var emptyBytes = make([]byte, 1024)

func startServer() string {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{
		IP: net.IPv4(127, 0, 0, 1),
	})
	if err != nil {
		panic(err)
	}
	server := &http.Server{
		Handler: (http.HandlerFunc)(func(rw http.ResponseWriter, req *http.Request) {
			size := 128
			rw.Header().Set("Content-Length", strconv.Itoa(size*len(emptyBytes)))
			rw.WriteHeader(http.StatusOK)
			for range size {
				rw.Write(emptyBytes)
			}
		}),
	}
	go server.Serve(listener)
	return "http://" + listener.Addr().String()
}

var expectedDownloadHash = []byte{0xfa, 0x43, 0x23, 0x9b, 0xce, 0xe7, 0xb9, 0x7c, 0xa6, 0x2f, 0x0, 0x7c, 0xc6, 0x84, 0x87, 0x56, 0xa, 0x39, 0xe1, 0x9f, 0x74, 0xf3, 0xdd, 0xe7, 0x48, 0x6d, 0xb3, 0xf9, 0x8d, 0xf8, 0xe4, 0x71}

func BenchmarkDownlaodWhileVerify(b *testing.B) {
	url := startServer()
	fd, err := os.CreateTemp("", "gotest-")
	if err != nil {
		b.Fatalf("Cannot create temporary file: %v", err)
	}
	defer fd.Close()
	defer os.Remove(fd.Name())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		b.Fatalf("Cannot form new request: %v", err)
	}

	hashMethod := crypto.SHA256
	buf := make([]byte, 1024)
	client := &http.Client{}

	b.ResetTimer()
	for range b.N {
		if _, err := fd.Seek(io.SeekStart, 0); err != nil {
			b.Fatalf("Seek error: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request error: %v", err)
		}
		hw := hashMethod.New()
		if _, err := io.CopyBuffer(io.MultiWriter(fd, hw), resp.Body, buf); err != nil {
			b.Fatalf("Copy error: %v", err)
		}
		resp.Body.Close()
		if hs := hw.Sum(buf[:0]); !bytes.Equal(hs, expectedDownloadHash) {
			b.Fatalf("Hash mismatch: %#v", hs)
		}
	}
}

func BenchmarkDownlaodThenVerify(b *testing.B) {
	url := startServer()
	fd, err := os.CreateTemp("", "gotest-")
	if err != nil {
		b.Fatalf("Cannot create temporary file: %v", err)
	}
	defer fd.Close()
	defer os.Remove(fd.Name())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		b.Fatalf("Cannot form new request: %v", err)
	}

	hashMethod := crypto.SHA256
	buf := make([]byte, 1024)
	client := &http.Client{}

	b.ResetTimer()
	for range b.N {
		if _, err := fd.Seek(io.SeekStart, 0); err != nil {
			b.Fatalf("Seek error: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request error: %v", err)
		}
		if _, err := io.CopyBuffer(fd, resp.Body, buf); err != nil {
			b.Fatalf("Copy error: %v", err)
		}
		resp.Body.Close()
		hw := hashMethod.New()
		if _, err := fd.Seek(io.SeekStart, 0); err != nil {
			b.Fatalf("Seek error: %v", err)
		}
		if _, err := io.CopyBuffer(hw, fd, buf); err != nil {
			b.Fatalf("Copy error: %v", err)
		}
		if hs := hw.Sum(buf[:0]); !bytes.Equal(hs, expectedDownloadHash) {
			b.Fatalf("Hash mismatch: %#v", hs)
		}
	}
}
