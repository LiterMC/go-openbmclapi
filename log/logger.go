/**
 * OpenBmclAPI (Golang Edition)
 * Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
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

package log

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	logDir    string = "logs"
	logSlots  int    = 7
	logfile   atomic.Pointer[os.File]
	logStdout atomic.Pointer[io.Writer]

	logTimeFormat string = "15:04:05"

	accessLogFileName = filepath.Join(logDir, "access.log")
	accessLogFile     atomic.Pointer[os.File]

	maxAccessLogFileSize int64 = 1024 * 1024 * 10 // 10MB
	accessLogSlots       int   = 16
)

type Level int32

const (
	_ Level = iota
	LevelTrace
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelPanic

	LevelMask = 1<<8 - 1
)

const (
	LogConsoleOnly Level = 1 << (8 + iota)
	LogNotToFile
)

func (l Level) String() string {
	switch l & LevelMask {
	case LevelTrace:
		return "TRAC"
	case LevelDebug:
		return "DBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERRO"
	case LevelPanic:
		return "PANI"
	default:
		return "<Unknown logger.Level>"
	}
}

func SetLogOutput(out io.Writer) {
	if out == nil {
		logStdout.Store(nil)
	} else {
		logStdout.Store(&out)
	}
}

func SetLogSlots(slots int) {
	logSlots = slots
}

func SetAccessLogSlots(slots int) {
	accessLogSlots = slots
}

var minimumLogLevel atomic.Int32

func init() {
	SetLevel(LevelInfo)
}

func SetLevel(l Level) {
	minimumLogLevel.Store((int32)(l & LevelMask))
}

func logWrite(level Level, buf []byte) {
	if level&LevelMask < (Level)(minimumLogLevel.Load()) {
		return
	}
	{
		out := logStdout.Load()
		if out != nil {
			(*out).Write(buf)
		} else {
			os.Stdout.Write(buf)
		}
	}
	if level&LogConsoleOnly == 0 && level&LogNotToFile == 0 {
		if fd := logfile.Load(); fd != nil {
			fd.Write(buf)
		}
	}
}

type LogListenerFn = func(ts int64, level Level, log string)

type LogListener struct {
	level Level
	cb    LogListenerFn
}

var logListenMux sync.RWMutex
var logListeners []*LogListener

func RegisterLogMonitor(level Level, cb LogListenerFn) func() {
	l := &LogListener{
		level: level,
		cb:    cb,
	}

	logListenMux.Lock()
	defer logListenMux.Unlock()

	logListeners = append(logListeners, l)
	var canceled atomic.Bool
	return func() {
		if canceled.Swap(true) {
			return
		}
		logListenMux.Lock()
		defer logListenMux.Unlock()

		for i, ll := range logListeners {
			if ll == l {
				end := len(logListeners) - 1
				logListeners[i] = logListeners[end]
				logListeners = logListeners[:end]
				return
			}
		}
	}
}

func callLogListeners(level Level, ts int64, log string) {
	if level&LogConsoleOnly != 0 {
		return
	}

	logListenMux.RLock()
	defer logListenMux.RUnlock()

	for _, l := range logListeners {
		if level&LevelMask >= l.level {
			l.cb(ts, level, log)
		}
	}
}

const LOG_BUF_SIZE = 1024

var logBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, LOG_BUF_SIZE)
		return &buf
	},
}

func logXStr(level Level, log string) {
	now := time.Now()
	lvl := level.String()

	buf0 := logBufPool.Get().(*[]byte)
	defer logBufPool.Put(buf0)
	buf := bytes.NewBuffer((*buf0)[:0])
	buf.Grow(1 + len(lvl) + 2 + len(logTimeFormat) + 3 + len(log) + 1)
	buf.WriteString("[")
	buf.WriteString(lvl)
	buf.WriteString("][")
	buf.WriteString(now.Format(logTimeFormat))
	buf.WriteString("]: ")
	buf.WriteString(log)
	buf.WriteByte('\n')
	// write log to console and log file
	logWrite(level, buf.Bytes())
	// send log to monitors
	callLogListeners(level, now.UnixMilli(), log)
}

func logX(level Level, args ...any) {
	sa := make([]string, len(args))
	for i, _ := range args {
		sa[i] = fmt.Sprint(args[i])
	}
	c := strings.Join(sa, " ")
	logXStr(level, c)
}

func logXf(level Level, format string, args ...any) {
	c := fmt.Sprintf(format, args...)
	logXStr(level, c)
}

func Debug(args ...any) {
	logX(LevelDebug, args...)
}

func Debugf(format string, args ...any) {
	logXf(LevelDebug, format, args...)
}

func Info(args ...any) {
	logX(LevelInfo, args...)
}

func Infof(format string, args ...any) {
	logXf(LevelInfo, format, args...)
}

func Warn(args ...any) {
	logX(LevelWarn, args...)
}

func Warnf(format string, args ...any) {
	logXf(LevelWarn, format, args...)
}

func Error(args ...any) {
	logX(LevelError, args...)
}

func Errorf(format string, args ...any) {
	logXf(LevelError, format, args...)
}

func Panic(err any) {
	logX(LevelPanic, err)
	panic(err)
}

func Panicf(format string, args ...any) {
	err := fmt.Errorf(format, args...)
	logX(LevelPanic, err)
	panic(err)
}

func RecordPanic() {
	if err := recover(); err != nil {
		stack := debug.Stack()
		logXf(LevelPanic, "panic: %v\n%s", err, stack)
		panic(err)
	}
}

func RecoverPanic(then func(err any)) {
	if err := recover(); err != nil {
		stack := debug.Stack()
		logXf(LevelPanic, "[recover]: panic: %v\n%s", err, stack)
		if then != nil {
			then(err)
		}
	}
}

func BaseDir() string {
	return logDir
}

func ListLogs() (files []string) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		base, _, ok := strings.Cut(entry.Name(), ".log")
		if !ok {
			continue
		}
		_ = base
		// if _, err := time.Parse("20060102-15", base); err == nil
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return
}

func flushLogfile() {
	if _, err := os.Stat(logDir); errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(logDir, 0755)
	}
	lfile, err := os.OpenFile(filepath.Join(logDir, time.Now().Format("20060102-15.log")),
		os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		Error("Cannot create new log file:", err)
		return
	}

	old := logfile.Swap(lfile)
	if old != nil {
		old.Close()
	}
}

func removeExpiredLogFiles(before string) {
	if files, err := os.ReadDir(logDir); err == nil {
		for _, f := range files {
			n := f.Name()
			if strings.HasSuffix(n, ".log") && n < before {
				p := filepath.Join(logDir, n)
				Debugf("Remove expired log %q", p)
				os.Remove(p)
			}
		}
	}
}

func moveAccessLogs(src string, n int) {
	if n > accessLogSlots {
		os.Remove(src)
		return
	}
	dst := filepath.Join(filepath.Dir(src), fmt.Sprintf("access.%d.log", n))
	if n > 1 {
		dst += ".gz"
	}
	if _, err := os.Stat(dst); err == nil || !errors.Is(err, os.ErrNotExist) {
		moveAccessLogs(dst, n+1)
	}
	if n == 2 {
		srcFd, err := os.Open(src)
		if err != nil {
			Errorf("Cannot open log file at %s: %v", src, err)
			return
		}
		defer srcFd.Close()
		dstFd, err := os.Create(dst)
		if err != nil {
			Errorf("Cannot create file at %s: %v", dst, err)
			return
		}
		defer dstFd.Close()
		w := gzip.NewWriter(dstFd)
		defer w.Close()
		_, err = io.Copy(w, srcFd)
		if err != nil {
			Errorf("Cannot compress log file to %s: %v", dst, err)
		}
	} else {
		err := os.Rename(src, dst)
		if err != nil {
			Errorf("Cannot rename log file: %v", err)
		}
	}
}

func flushAccessLogFile() {
	var (
		err     error
		fd      = accessLogFile.Load()
		changed = false
	)
	if fd == nil {
		if fd, err = os.OpenFile(accessLogFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644); err != nil {
			Errorf("Cannot open log file at %s: %v", accessLogFileName, err)
			return
		}
		changed = true
	}
	if stat, err := fd.Stat(); err == nil && stat.Size() >= maxAccessLogFileSize {
		fd.Close()
		moveAccessLogs(accessLogFileName, 1)
		if fd, err = os.OpenFile(accessLogFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644); err != nil {
			Errorf("Cannot create log file at %s: %v", accessLogFileName, err)
			return
		}
		changed = true
	}
	if changed {
		accessLogFile.Store(fd)
	}
}

func StartFlushLogFile() {
	flushLogfile()
	if accessLogSlots > 0 {
		flushAccessLogFile()
	}

	logfile.Load().Write(([]byte)("================================================================\n"))
	go func() {
		tma := (time.Now().Unix()/(60*60) + 1) * (60 * 60)
		for {
			select {
			case <-time.After(time.Duration(tma-time.Now().Unix()) * time.Second):
				tma = (time.Now().Unix()/(60*60) + 1) * (60 * 60)
				flushLogfile()
				if logSlots > 0 {
					dur := -time.Hour * 24 * (time.Duration)(logSlots)
					removeExpiredLogFiles(time.Now().Add(dur).Format("20060102"))
				}
			}
		}
	}()
}

func LogAccess(level Level, data any) {
	if accessLogSlots < 0 {
		return
	}

	flushAccessLogFile()

	var buf [512]byte
	bts := bytes.NewBuffer(buf[:0])
	e := json.NewEncoder(bts)
	e.SetEscapeHTML(false)
	e.Encode(data)

	var s string
	if fs, ok := data.(fmt.Stringer); ok {
		s = fs.String()
	} else {
		s = (string)(bts.Bytes()[:bts.Len()-1]) // we don't want the newline character
	}
	logX(level|LogNotToFile, s)
	fd := accessLogFile.Load()
	fd.Write(bts.Bytes())
}
