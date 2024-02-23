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

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var logdir string = "logs"
var logfile atomic.Pointer[os.File]
var logStdout atomic.Pointer[io.Writer]

var logTimeFormat string = "15:04:05"

func setLogOutput(out io.Writer) {
	if out == nil {
		logStdout.Store(nil)
	} else {
		logStdout.Store(&out)
	}
}

func logWrite(level LogLevel, buf []byte) {
	if level <= LogLevelDebug && !config.Advanced.DebugLog {
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
	if fd := logfile.Load(); fd != nil {
		fd.Write(buf)
	}
}

type LogLevel int

const (
	_ LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERRO"
	default:
		return "<Unknown LogLevel>"
	}
}

type LogListenerFn = func(ts int64, level LogLevel, log string)

type LogListener struct {
	level LogLevel
	cb    LogListenerFn
}

var logListenMux sync.RWMutex
var logListeners []*LogListener

func RegisterLogMonitor(level LogLevel, cb LogListenerFn) func() {
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

func callLogListeners(level LogLevel, ts int64, log string) {
	logListenMux.RLock()
	defer logListenMux.RUnlock()

	for _, l := range logListeners {
		if level >= l.level {
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

func logXStr(level LogLevel, log string) {
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

func logX(level LogLevel, args ...any) {
	sa := make([]string, len(args))
	for i, _ := range args {
		sa[i] = fmt.Sprint(args[i])
	}
	c := strings.Join(sa, " ")
	logXStr(level, c)
}

func logXf(level LogLevel, format string, args ...any) {
	c := fmt.Sprintf(format, args...)
	logXStr(level, c)
}

func logDebug(args ...any) {
	logX(LogLevelDebug, args...)
}

func logDebugf(format string, args ...any) {
	logXf(LogLevelDebug, format, args...)
}

func logInfo(args ...any) {
	logX(LogLevelInfo, args...)
}

func logInfof(format string, args ...any) {
	logXf(LogLevelInfo, format, args...)
}

func logWarn(args ...any) {
	logX(LogLevelWarn, args...)
}

func logWarnf(format string, args ...any) {
	logXf(LogLevelWarn, format, args...)
}

func logError(args ...any) {
	logX(LogLevelError, args...)
}

func logErrorf(format string, args ...any) {
	logXf(LogLevelError, format, args...)
}

func flushLogfile() {
	if _, err := os.Stat(logdir); errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(logdir, 0755)
	}
	lfile, err := os.OpenFile(filepath.Join(logdir, time.Now().Format("20060102-15.log")),
		os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		logError("Cannot create new log file:", err)
		return
	}

	old := logfile.Swap(lfile)
	if old != nil {
		old.Close()
	}
}

func removeExpiredLogFiles(before string) {
	if files, err := os.ReadDir(logdir); err == nil {
		for _, f := range files {
			n := f.Name()
			if strings.HasSuffix(n, ".log") && n < before {
				p := filepath.Join(logdir, n)
				logDebugf("Remove expired log %q", p)
				os.Remove(p)
			}
		}
	}
}

func startFlushLogFile() {
	flushLogfile()
	logfile.Load().Write(([]byte)("================================================================\n"))
	go func() {
		tma := (time.Now().Unix()/(60*60) + 1) * (60 * 60)
		for {
			select {
			case <-time.After(time.Duration(tma-time.Now().Unix()) * time.Second):
				tma = (time.Now().Unix()/(60*60) + 1) * (60 * 60)
				flushLogfile()
				if config.LogSlots > 0 {
					dur := -time.Hour * 24 * (time.Duration)(config.LogSlots)
					removeExpiredLogFiles(time.Now().Add(dur).Format("20060102"))
				}
			}
		}
	}()
}

var NullLogger = log.New(DevNull, "", log.LstdFlags)
