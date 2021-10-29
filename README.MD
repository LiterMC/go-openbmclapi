
> 注: 本仓库代码按照<https://github.com/bangbang93/openbmclapi>编写, **非**官方, **不保证**稳定性/时效性

## 优势

- 使用协程/多线程, 超高的文件同步速度
- 不依赖大量的三方包, 体积小巧
- 静态文件, 无需配置依赖
- ~~更好的压榨节点~~

## 安装

### 直接运行

1. 从 github release 找到**适合您服务器平台**的程序并下载到节点服务器上
2. 配置配置文件, 可以直接使用与bangbang93的openbmclapi相同的环境变量配置, 也可以从`config.json`配置 _(下文有讲)_
3. 运行程序

### 从源代码运行

1. 下载本仓库 _(可以使用`git clone https://github.com/KpnmServer/go-openbmclapi.git`)_
2. 确保您的**节点**服务器上装有`go 1.11+` _(注: 仅测试过**go1.15.14**)_
3. cd进入本仓库
4. 配置配置文件或环境变量
5. 使用`go run ./src`运行本程序

### 从docker运行

#### 1. 可直接运行仓库目录下的`docker-run.sh`文件, 运行之前请确保存在`kupen/go-openbmclapi:latest`镜像, 或网络连通
#### 2. 使用`docker build -t kupen/go-openbmclapi:latest .`手动编译镜像, 然后再使用`docker-run.sh`运行

Dockerfile内容如下:
```Dockerfile
FROM ubuntu:latest

RUN mkdir -p /web && mkdir -p /web/work && cd /web &&\
 apt-get update && apt-get install -y curl &&\
 curl -L -o /web/linux-amd64-openbmclapi\
 "https://github.com/KpnmServer/go-openbmclapi/releases/download/v0.6.0-1/linux-amd64-openbmclapi" &&\
 chmod +x /web/linux-amd64-openbmclapi &&\
 echo -e '#!/bin/sh\ncd /web/work;if [ ! -f "./config.json" ];then echo "{\"debug\":false,\"port\":80}" >./config.json;fi;'\
'exec /web/linux-amd64-openbmclapi' >/web/runner.sh &&\
 chmod +x /web/runner.sh

CMD exec /web/runner.sh
```

### 从`systemd`运行 _(仅Linux)_

1. **确保systemd是您的*启动进程***
2. 在root用户下执行`service/installer.sh`
3. 配置`/var/openmbmclapi/config.json`配置文件
4. 使用`systemctl start go-openbmclapi.service`启动服务

- 使用`systemctl reload go-openbmclapi.service`可重新加载配置文件 __(暂时仅限完全启动后, *即web服务器已启动* )__
- 使用`systemctl stop go-openbmclapi.service`停止服务
- 使用`journalctl -f -u go-openbmclapi.service`监听实时日志

## 配置

### 使用配置文件

#### 注意: 配置文件指定的值会被环境变量的值 _(如果存在)_ 覆盖掉

配置文件应为**运行目录**下的`config.json`文件, 使用`json`格式解析  
例:
```javascript
{
	"debug": false,           // 是否开启调试模式, 默认为关
	"show_serve_info": false, // 是否打印访问信息, 默认为否
	"host": "example.com",    // public host, 同 CLUSTER_IP
	"port": 80,               // 要监听的本地端口, 同 CLUSTER_PORT
	"public_port": 8080,      // 实际开放的公网端口, 同 CLUSTER_PUBLIC_PORT
	"cluster_id": "${CLUSTER_ID}", // CLUSTER_ID
	"cluster_secret": "${CLUSTER_SECRET}" // CLUSTER_SECRET
}
```
