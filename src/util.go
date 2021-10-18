
package main

import (
	time "time"
	strings "strings"

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
