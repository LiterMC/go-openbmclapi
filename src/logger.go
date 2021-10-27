
package main

import (
	// io "io"
	os "os"
	bytes "bytes"
	strings "strings"
	fmt "fmt"
	time "time"
	sync "sync"

	ufile "github.com/KpnmServer/go-util/file"
)

var logdir string = "logs"
var logfile *os.File

var logLock sync.Mutex
var logTimeFormat string = "15:04:05.000"

func logX(x string, args ...interface{}){
	sa := make([]string, len(args))
	for i, _ := range args {
		sa[i] = fmt.Sprint(args[i])
	}
	c := strings.Join(sa, " ")
	buf := bytes.NewBuffer(nil)
	buf.Grow(6 + len(logTimeFormat) + len(x) + len(c)) // (1 + len(x) + 2 + len(logTimeFormat) + 3)
	buf.WriteString("[")
	buf.WriteString(x)
	buf.WriteString("][")
	buf.WriteString(time.Now().Format(logTimeFormat))
	buf.WriteString("]: ")
	buf.WriteString(c)
	logLock.Lock()
	defer logLock.Unlock()
	fmt.Fprintln(os.Stderr, buf.String())
	if logfile != nil {
		logfile.Write(buf.Bytes())
		logfile.Write([]byte{'\n'})
	}
}

func logXf(x string, format string, args ...interface{}){
	c := fmt.Sprintf(format, args...)
	buf := bytes.NewBuffer(nil)
	buf.Grow(6 + len(logTimeFormat) + len(x) + len(c)) // (1 + len(x) + 2 + len(logTimeFormat) + 3)
	buf.WriteString("[")
	buf.WriteString(x)
	buf.WriteString("][")
	buf.WriteString(time.Now().Format(logTimeFormat))
	buf.WriteString("]: ")
	buf.WriteString(c)
	logLock.Lock()
	defer logLock.Unlock()
	fmt.Fprintln(os.Stderr, buf.String())
	if logfile != nil {
		logfile.Write(buf.Bytes())
		logfile.Write([]byte{'\n'})
	}
}

func logDebug(args ...interface{}){
	if DEBUG {
		logX("DBUG", args...)
	}
}

func logDebugf(format string, args ...interface{}){
	if DEBUG {
		logXf("DBUG", format, args...)
	}
}

func logInfo(args ...interface{}){
	logX("INFO", args...)
}

func logInfof(format string, args ...interface{}){
	logXf("INFO", format, args...)
}

func logWarn(args ...interface{}){
	logX("WARN", args...)
}

func logWarnf(format string, args ...interface{}){
	logXf("WARN", format, args...)
}

func logError(args ...interface{}){
	logX("ERRO", args...)
}

func logErrorf(format string, args ...interface{}){
	logXf("ERRO", format, args...)
}

func flushLogfile(){
	if ufile.IsNotExist(logdir){
		ufile.CreateDir(logdir)
	}
	lfile, err := os.OpenFile(ufile.JoinPath(logdir, time.Now().Format("20060102-15.log")),
		os.O_WRONLY | os.O_APPEND | os.O_CREATE, 0666)
	if err != nil {
		logError("Create new log file error:", err)
		return
	}
	logLock.Lock()
	defer logLock.Unlock()
	logfile = lfile
}

func init(){
	flushLogfile()
	go func(){
		for{
			select{
			case <-time.After(time.Duration((time.Now().Unix() / (60 * 60) + 1) * (60 * 60) - time.Now().Unix()) * time.Second):
				flushLogfile()
			}
		}
	}()
}

