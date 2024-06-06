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
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

func ParseRSAPrivateKey(content []byte) (key *rsa.PrivateKey, err error) {
	for {
		var blk *pem.Block
		blk, content = pem.Decode(content)
		if blk == nil {
			break
		}
		if blk.Type == "RSA PRIVATE KEY" {
			k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
			if err != nil {
				return nil, err
			}
			return k.(*rsa.PrivateKey), nil
		}
	}
	return nil, errors.New(`Cannot find "RSA PRIVATE KEY" in pem blocks`)
}

func DecryptStream(w io.Writer, r io.Reader, key *rsa.PrivateKey) (err error) {
	encryptedAESKey := make([]byte, key.Size())
	if _, err = io.ReadFull(r, encryptedAESKey); err != nil {
		return
	}
	aesKey, err := rsa.DecryptPKCS1v15(nil, key, encryptedAESKey[:])
	if err != nil {
		return
	}
	blk, err := aes.NewCipher(aesKey)
	if err != nil {
		return
	}
	iv := make([]byte, blk.BlockSize())
	if _, err = io.ReadFull(r, iv); err != nil {
		return
	}
	sr := &cipher.StreamReader{
		S: cipher.NewCTR(blk, iv),
		R: r,
	}
	buf := make([]byte, blk.BlockSize()*256)
	if _, err = io.CopyBuffer(w, sr, buf); err != nil {
		return
	}
	return
}

func main() {
	flag.Parse()
	keyFile, logFile := flag.Arg(0), flag.Arg(1)
	enableGzip := strings.Contains(logFile, ".gz.")
	keyContent, err := os.ReadFile(keyFile)
	if err != nil {
		fmt.Println("Cannot read", keyFile, ":", err)
		os.Exit(1)
	}
	key, err := ParseRSAPrivateKey(keyContent)
	if err != nil {
		fmt.Println("Cannot parse key from", keyFile, ":", err)
		os.Exit(1)
	}
	fd, err := os.Open(logFile)
	if err != nil {
		fmt.Println("Cannot open", logFile, ":", err)
		os.Exit(1)
	}
	defer fd.Close()
	dst, err := createNewFile(logFile)
	if err != nil {
		fmt.Println("Cannot create new file:", err)
		os.Exit(1)
	}
	dstName := dst.Name()
	defer dst.Close()
	var w io.WriteCloser = dst
	var wg sync.WaitGroup
	if enableGzip {
		pr, pw := io.Pipe()
		wg.Add(1)
		go func(w io.WriteCloser) {
			defer wg.Done()
			defer w.Close()
			gr, err := gzip.NewReader(pr)
			if err != nil {
				dst.Close()
				os.Remove(dstName)
				fmt.Println("Cannot decompress data:", err)
				os.Exit(2)
			}
			defer gr.Close()
			if _, err = io.Copy(w, gr); err != nil {
				dst.Close()
				os.Remove(dstName)
				fmt.Println("Cannot copy decompressed data:", err)
				os.Exit(2)
			}
		}(w)
		w = pw
	}
	if err := DecryptStream(w, fd, key); err != nil {
		dst.Close()
		os.Remove(dstName)
		fmt.Println("Decrypt error:", err)
		os.Exit(2)
	}
	w.Close()
	wg.Wait()
}

func createNewFile(name string) (fd *os.File, err error) {
	basename, _, _ := strings.Cut(name, ".")
	fd, err = os.OpenFile(basename+".log", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			return
		}
	}
	for i := 2; i < 100; i++ {
		fd, err = os.OpenFile(fmt.Sprintf("%s.%d.log", basename, i), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			return
		}
		if !errors.Is(err, os.ErrExist) {
			return
		}
	}
	return
}
