
# Go OpemBmclAPI

Golang 版的 [OpemBmclAPI](https://github.com/bangbang93/openbmclapi)

> 注: 本仓库部分代码按照 <https://github.com/bangbang93/openbmclapi> 编写, **非官方, 不保证稳定性/时效性**

## 特性

- 使用协程/多线程, 超高的文件同步速度
- 不依赖大量的三方包, 体积小巧
- 静态文件, 无需配置任何依赖
- 得益于 Golang 强大的跨平台编译器, 支持大部分主流平台/处理器
- 内置一个实时更新的状态面板
- 支持一些小众的~~奇怪~~需求
- ~~更好的压榨节点~~

## FAQ

Q: 没有适合我的平台的程序怎么办?  
A: 检查 [Go Platform Values](https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63#platform-values), 如果您的系统/架构存在, 那么可以发个 issue

Q: 版本号格式  
A: `v<openbmclapi版本>-<go-openbmclapi 构建计数>`, 例: `v1.6.7-60`

## 安装

### 无依赖直接运行

1. 从 github release 找到**适合您服务器平台**的程序并下载到节点服务器上
2. 配置配置文件, 可以直接使用与bangbang93的openbmclapi相同的环境变量配置, 也可以从`config.json`配置 _(下文有讲)_
3. 运行程序

### 从docker运行

- 可直接运行仓库目录下的`docker-run.sh`文件, 运行之前请确保存在`craftmine/go-openbmclapi:latest`镜像, 或网络连通
- 也可使用`docker build -t craftmine/go-openbmclapi:latest .`手动编译镜像

### 从`systemd`运行 _(仅Linux)_

1. **确保systemd是您的*启动进程***
2. 执行
   ```sh
   sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/LiterMC/go-openbmclapi/HEAD/service/installer.sh)"
   ```
3. 配置`/opt/openbmclapi/config.json`配置文件
4. 使用`systemctl start go-openbmclapi.service`启动服务
   - 使用`systemctl reload go-openbmclapi.service`可重新加载配置文件
   - 使用`systemctl stop go-openbmclapi.service`停止服务
   - 使用`journalctl -f -u go-openbmclapi.service`实时监听日志

### 从源代码运行

1. 下载本仓库 _(可以使用`git clone https://github.com/LiterMC/go-openbmclapi.git`)_
2. 确保您的服务器上装有`go 1.21+`
3. cd进入本仓库
4. 配置配置文件或环境变量
5. 使用`go run ./src`运行本程序

## 配置

### 使用配置文件

#### 注意: 配置文件指定的值会被环境变量的值 _(如果存在)_ 覆盖掉

配置文件应为**运行目录**下的`config.yaml`文件, 使用`yaml`格式解析  
例:
```yaml
# 是否打印调试日志, 默认为否
debug: false
# 是否打印访问信息, 默认为否 (这个选项对于压缩日志文件十分有用)
record_serve_info: false
nohttps: false
# 实际开放的公网主机名, 同 CLUSTER_IP
public_host: example.com
# 实际开放的公网端口, 同 CLUSTER_PUBLIC_PORT
public_port: 8080
# 要监听的本地端口, 同 CLUSTER_PORT
port: 4000
# CLUSTER_ID
cluster_id: ${CLUSTER_ID}
# CLUSTER_SECRET
cluster_secret: ${CLUSTER_SECRET}
# 同步文件时最多打开的连接数量
download_max_conn: 64

## 特殊要求: 重定向到 OSS
oss:
	 # 是否启用该功能
    enable: false
    list:
          # 镜像文件夹路径
        - folder_path: oss_mirror
          # 镜像网络路径
          redirect_base: https://oss.example.com/base/paths
          # 跳过生成measure文件夹
          skip_measure_gen: false

## 特殊要求: 劫持 bmclapi 以加速同一局域网请求
hijack_port:
	 # 是否启用该功能
    enable: false
    # 劫持代理监听的主机名
    server_host: ""
    # 劫持代理监听的端口
    server_port: 8090
    # 在该路径下生成伪bmclapi路径结构
    # 注: 该路径下为指向cache文件夹的软连接, 不会占用大量磁盘空间
    # **不建议手动修改该路径内的内容**
    path: __hijack
    # 用于绕过DNS劫持的DNS服务器 (从bmclapi同步时使用)
    anti_hijack_dns: 8.8.8.8:53
```
