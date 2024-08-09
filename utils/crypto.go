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

package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func ComparePasswd(p1, p2 string) bool {
	a := sha256.Sum256(([]byte)(p1))
	b := sha256.Sum256(([]byte)(p2))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 0
}

func BytesAsSha256(b []byte) string {
	buf := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

// return a URL encoded base64 string
func AsSha256(s string) string {
	buf := sha256.Sum256(([]byte)(s))
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

func AsSha256Hex(s string) string {
	buf := sha256.Sum256(([]byte)(s))
	return hex.EncodeToString(buf[:])
}

func HMACSha256Hex(key, data string) string {
	return (string)(HMACSha256HexBytes(key, data))
}

func HMACSha256HexBytes(key, data string) []byte {
	m := hmac.New(sha256.New, ([]byte)(key))
	m.Write(([]byte)(data))
	buf := m.Sum(nil)
	value := make([]byte, hex.EncodedLen(len(buf)))
	hex.Encode(value, buf[:])
	return value
}

func GenRandB64(n int) (s string, err error) {
	buf := make([]byte, n)
	if _, err = io.ReadFull(rand.Reader, buf); err != nil {
		return
	}
	s = base64.RawURLEncoding.EncodeToString(buf)
	return
}

func LoadOrCreateHmacKey(dataDir string) (key []byte, err error) {
	path := filepath.Join(dataDir, "server.hmac.private_key")
	buf, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return
		}
		var sbuf string
		if sbuf, err = GenRandB64(256); err != nil {
			return
		}
		buf = ([]byte)(sbuf)
		if err = os.WriteFile(path, buf, 0600); err != nil {
			return
		}
	}
	key = make([]byte, base64.RawURLEncoding.DecodedLen(len(buf)))
	if _, err = base64.RawURLEncoding.Decode(key, buf); err != nil {
		return
	}
	return
}

const developorRSAKey = `
-----BEGIN RSA PUBLIC KEY-----
MIIBCgKCAQEAqIvK9cVuDtD/V4w7/xIPI2mnv2VV0CTQfelDaEB4vonsblwIp3VV
1S3oYXY8thyCscBKG/AkryKHS0U1TXoIMPDai3vkLDL5sY4mmh4aFCoGKdRmNNyr
kAjaLo51gnadCMbaoxVNzQ1naJvZjU02ClJvBjtTETUzrqFnx8th04P0bSZHZEwV
vmCniuzzuAcNI92hpoSEqp4WGqINp2hoAFnoHENgnAsb94jJX4VfWbMySR1O+ykz
RkGvslKPhvv/YkCxy0Xi2FgXnb+xw/CXevkTvu7WSVodvZ0OtAHPTp6kCTWt3tun
PN0d0McYg73htD0ItifOcJQWPPnFZKezUQIDAQAB
-----END RSA PUBLIC KEY-----`

var DeveloporPublicKey *rsa.PublicKey

func init() {
	var err error
	DeveloporPublicKey, err = ParseRSAPublicKey(([]byte)(developorRSAKey))
	if err != nil {
		panic(err)
	}
}

func ParseRSAPublicKey(content []byte) (key *rsa.PublicKey, err error) {
	for {
		var blk *pem.Block
		blk, content = pem.Decode(content)
		if blk == nil {
			break
		}
		if blk.Type == "RSA PUBLIC KEY" {
			return x509.ParsePKCS1PublicKey(blk.Bytes)
		}
	}
	return nil, errors.New(`Cannot find "RSA PUBLIC KEY" in pem blocks`)
}

func EncryptStream(w io.Writer, r io.Reader, publicKey *rsa.PublicKey) (err error) {
	var aesKey [16]byte
	if _, err = io.ReadFull(rand.Reader, aesKey[:]); err != nil {
		return
	}
	// rsa.EncryptOAEP(sha256.New, rand.Reader, publicKey, aesKey[:], ([]byte)("aes-stream-key"))
	encryptedAESKey, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, aesKey[:])
	if err != nil {
		return
	}
	blk, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return
	}
	iv := make([]byte, blk.BlockSize())
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return
	}
	sw := &cipher.StreamWriter{
		S: cipher.NewCTR(blk, iv),
		W: w,
	}
	if _, err = w.Write(encryptedAESKey); err != nil {
		return
	}
	if _, err = w.Write(iv); err != nil {
		return
	}
	buf := make([]byte, blk.BlockSize()*256)
	if _, err = io.CopyBuffer(sw, r, buf); err != nil {
		return
	}
	return
}
