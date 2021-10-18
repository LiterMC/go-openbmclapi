
package main

import (
	// io "io"
	os "os"
	bytes "bytes"
	strings "strings"
	fmt "fmt"
	time "time"
	sync "sync"
)

var logLock sync.Mutex
var logTimeFormat string = "15:04:05.000"

func logX(x string, args ...interface{}){
	sa := make([]string, len(args))
	for i, _ := range args {
		sa[i] = fmt.Sprint(args[i])
	}
	c := strings.Join(sa, " ")
	buf := bytes.NewBuffer(make([]byte, 6 + len(logTimeFormat) + len(x) + len(c))) // (1 + len(x) + 2 + len(logTimeFormat) + 3)
	buf.WriteString("[")
	buf.WriteString(x)
	buf.WriteString("][")
	buf.WriteString(time.Now().Format(logTimeFormat))
	buf.WriteString("]: ")
	buf.WriteString(c)
	logLock.Lock()
	defer logLock.Unlock()
	fmt.Fprintln(os.Stderr, buf.String())
}

func logXf(x string, format string, args ...interface{}){
	c := fmt.Sprintf(format, args...)
	buf := bytes.NewBuffer(make([]byte, 6 + len(logTimeFormat) + len(x) + len(c))) // (1 + len(x) + 2 + len(logTimeFormat) + 3)
	buf.WriteString("[")
	buf.WriteString(x)
	buf.WriteString("][")
	buf.WriteString(time.Now().Format(logTimeFormat))
	buf.WriteString("]: ")
	buf.WriteString(c)
	logLock.Lock()
	defer logLock.Unlock()
	fmt.Fprintln(os.Stderr, buf.String())
}

func logDebug(args ...interface{}){
	if DEBUG {
		go logX("DBUG", args...)
	}
}

func logDebugf(format string, args ...interface{}){
	if DEBUG {
		go logXf("DBUG", format, args...)
	}
}

func logInfo(args ...interface{}){
	go logX("INFO", args...)
}

func logInfof(format string, args ...interface{}){
	go logXf("INFO", format, args...)
}

func logWarn(args ...interface{}){
	go logX("WARN", args...)
}

func logWarnf(format string, args ...interface{}){
	go logXf("WARN", format, args...)
}

func logError(args ...interface{}){
	go logX("ERRO", args...)
}

func logErrorf(format string, args ...interface{}){
	go logXf("ERRO", format, args...)
}

