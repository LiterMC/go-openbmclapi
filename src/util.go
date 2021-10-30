
package main

import (
	fmt "fmt"
	time "time"
	strings "strings"
	context "context"

	ufile "github.com/KpnmServer/go-util/file"
)

func hashToFilename(hash string)(string){
	return ufile.JoinPath(hash[0:2], hash)
}

func createInterval(do func(), delay time.Duration)(cancel func()(bool)){
	running := false
	exitch := make(chan struct{}, 1)
	cancel = func()(bool) {
		if !running {
			return false
		}
		logDebug("Canceled interval:", exitch)
		running = false
		exitch <- struct{}{}
		exitch = nil
		return true
	}
	logDebug("Created interval:", exitch)
	go func(){
		running = true
		for running {
			select{
			case <-exitch:
				return
			case <-time.After(delay):
				do()
			}
		}
	}()
	return
}

func httpToWs(origin string)(string){
	if strings.HasPrefix(origin, "http") {
		return "ws" + origin[4:]
	}
	return origin
}

func bytesToUnit(size float32)(string){
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

func withContext(ctx context.Context, call func())(bool){
	if ctx == nil {
		call()
		return true
	}
	done := make(chan struct{}, 1)
	go func(){
		call()
		done <- struct{}{}
	}()
	select{
	case <-ctx.Done():
		return false
	case <-done:
		return true
	}
}

func checkContext(ctx context.Context)(bool){
	if ctx == nil {
		return false
	}
	select{
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

const BUF_SIZE = 1024 * 512 // 512KB
const BUF_COUNT = 1024 // 512MB
var BUFFER_PAGE = make([][]byte, BUF_COUNT)
var buf_trigger = make(chan int, BUF_COUNT)
func init(){
	for i, _ := range BUFFER_PAGE {
		BUFFER_PAGE[i] = make([]byte, BUF_SIZE)
		buf_trigger <- i
	}
}

func mallocBuf()(buf []byte, i int){
	logDebug("Waiting free buf")
	i = <- buf_trigger
	logDebug("Got free buf", i)
	buf = BUFFER_PAGE[i]
	return
}

func releaseBuf(i int){
	logDebug("Release buf", i)
	buf_trigger <- i
}

