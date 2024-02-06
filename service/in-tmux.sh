#!/bin/bash
case "`uname -m`" in
    x86_64)
        GOARCH="amd64"
    ;;
    i386|i686)
        GOARCH="386"
    ;;
    aarch64)
        GOARCH="arm64"
    ;;
    armv7l)
        GOARCH="arm"
    ;;
    *)
        echo "【8Mi & BMCLAPI】 未知处理器架构: `uname -m`,请将该架构报告到 https://github.com/LiterMC/go-openbmclapi/issues"
        exit
esac

FILE_NAME="openbmclapi-go"
#REPROXY_URL="https://mirror.ghproxy.com/"
REPROXY_URL=""
URL="https://github.com/LiterMC/go-openbmclapi/releases/latest/download/go-openbmclapi-linux-$GOARCH"

while true; do
    echo "【8Mi & BMCLAPI】 检测更新中"
    while true; do
        REMOTE_FILE_SIZE=` wget --spider -S "$REPROXY_URL$URL" 2>&1 | grep "Content-Length"  | awk '{print $2}' | grep -v '^0$' `
        if [ -n "$REMOTE_FILE_SIZE" ] && [ "$REMOTE_FILE_SIZE" -gt 0 ]; then
            break
        fi
        sleep 1
    done
    
    if [ -f "$FILE_NAME" ]; then
        LOCAL_FILE_SIZE=$(stat -c %s "$FILE_NAME")
    else
        LOCAL_FILE_SIZE=0
    fi
    
    if [ "$REMOTE_FILE_SIZE" -ne "$LOCAL_FILE_SIZE" ]; then
        echo "【8Mi & BMCLAPI】 有更新，正在更新中"
        while true; do
            wget --show-progress -qO "$FILE_NAME" "$REPROXY_URL$URL"
            if [ $? -eq 0 ]; then
                break
            fi
            sleep 1
        done
    else
        echo "【8Mi & BMCLAPI】 文件大小匹配，无需更新."
    fi

    chmod +x ./openbmclapi-go
    ./openbmclapi-go
    sleep 1
done