package main

import (
	"context"
	"crypto"
	"fmt"
	"strings"
	"sync"
	"time"

	ufile "github.com/KpnmServer/go-util/file"
)

func hashToFilename(hash string) string {
	return ufile.JoinPath(hash[0:2], hash)
}

func createInterval(ctx context.Context, do func(), delay time.Duration) {
	logDebug("Interval created:", ctx)
	go func() {
		ticker := time.NewTicker(delay)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logDebug("Interval stopped:", ctx)
				return
			case <-ticker.C:
				do()
			}
		}
	}()
	return
}

func httpToWs(origin string) string {
	if strings.HasPrefix(origin, "http") {
		return "ws" + origin[4:]
	}
	return origin
}

func bytesToUnit(size float64) string {
	unit := "Byte"
	if size >= 1000 {
		size /= 1024
		unit = "KB"
		if size >= 1000 {
			size /= 1024
			unit = "MB"
			if size >= 1000 {
				size /= 1024
				unit = "GB"
				if size >= 1000 {
					size /= 1024
					unit = "TB"
				}
			}
		}
	}
	return fmt.Sprintf("%.1f", size) + unit
}

func withContext(ctx context.Context, call func()) bool {
	if ctx == nil {
		call()
		return true
	}
	done := make(chan struct{}, 0)
	go func() {
		defer close(done)
		call()
	}()
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return true
	}
}

const BUF_SIZE = 1024 * 512 // 512KB
var bufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, BUF_SIZE)
		return &buf
	},
}

func getHashMethod(l int) (hashMethod crypto.Hash, err error) {
	switch l {
	case 32:
		hashMethod = crypto.MD5
	case 40:
		hashMethod = crypto.SHA1
	default:
		err = fmt.Errorf("Unknown hash length %d", l)
	}
	return
}
