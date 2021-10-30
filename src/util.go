
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
		running = false
		exitch <- struct{}{}
		exitch = nil
		return true
	}
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
	select{
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

