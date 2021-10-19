
> 注: 本仓库代码按照<https://github.com/bangbang93/openbmclapi>编写, **非**官方, **不保证**稳定性/时效性

## 安装

### 直接运行

1. 从 github release 找到**适合您服务器平台**的程序并下载到节点服务器上
2. 配置配置文件, 可以直接使用与bangbang93的openbmclapi相同的环境变量配置, 也可以从`config.json`配置 _(下文有讲)_
3. 运行程序

### 从源代码运行

1. 下载本仓库 _(可以使用`git clone https://github.com/KpnmServer/go-openbmclapi.git`)_
2. 确保您的服务器上装有`go 1.11+` _(注: 仅测试过**go1.15.14**)_
3. cd进入本仓库
4. 配置配置文件或环境变量
5. 使用`go run ./src`运行本程序

### 从docker运行

1. TODO

## 配置

### 使用配置文件

配置文件应为**运行目录**下的`config.json`文件, 使用`json`格式解析  
例:
```javascript
{
	"debug": false,        // 是否开启调试模式, 默认为关
	"host": "example.com", // public host, 同 CLUSTER_IP
	"port": 80,            // 要监听的本地端口, 同 CLUSTER_PORT
	"public_port": 8080,   // 实际开放的公网端口, 同 CLUSTER_PUBLIC_PORT
	"cluster_id": "${CLUSTER_ID}", // CLUSTER_ID
	"cluster_secret": "${CLUSTER_SECRET}", // CLUSTER_SECRET
	"version": "0.6.0"     // 节点版本, 勿动(除更新)
}
```
