
# Go OpenBmclAPI

Golang 版的 [OpenBmclAPI](https://github.com/bangbang93/openbmclapi)

> 注: 本仓库部分代码按照 <https://github.com/bangbang93/openbmclapi> 编写, **非官方, 不保证稳定性/时效性**

*如果本项目有用, 就给个 star ⭐️ 吧 :)*

欢迎进行 PR.

-> [完整贡献者列表](./CONTRIBUTORS.md)

## 特性

- 使用协程/多线程, 超高的文件同步速度
- 不依赖大量的三方包, 体积小巧
- 静态文件, 无需配置任何依赖
- 得益于 Golang 强大的跨平台编译器, 支持大部分主流平台/处理器
- 内置一个实时更新的状态面板
- 支持一些小众的~~奇怪~~需求
- ~~更好的压榨节点~~

## FAQ

Q: 支持`.env`文件吗?  
A: 不支持, Go-OpenBmclAPI 使用 yaml 配置文件, 该文件应位于程序的运行目录. 初次运行本程序时会自动创建该配置文件

Q: 没有适合我的平台的程序怎么办?  
A: 检查 [Go Platform Values](https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63#platform-values), 如果您的系统/架构存在, 那么可以发个 issue

Q: 版本号格式  
A: `v<openbmclapi 版本>-<go-openbmclapi 构建计数>`, 例: `v1.6.7-60`

## 安装

### 无依赖直接运行

1. 从 github release 找到**适合您服务器平台**的程序并下载到节点服务器上
2. 配置配置文件, 可以直接使用与bangbang93的openbmclapi相同的环境变量配置, 也可以从`config.yaml`进行配置 _(下文有讲)_
3. 运行程序

### 从docker运行

- 可直接运行仓库目录下的 [`docker-run.sh`](./docker-run.sh) 文件, 运行之前请确保存在`craftmine/go-openbmclapi:latest`镜像, 或网络连通
- 也可使用`docker build -t craftmine/go-openbmclapi:latest .`手动编译镜像

### 从`systemd`运行 _(仅Linux)_

1. **确保systemd是您的*启动进程***
2. 执行
   ```sh
   sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/LiterMC/go-openbmclapi/HEAD/service/installer.sh)"
   ```
   > 注意, 新版新增使用 `openbmclapi` 用户执行程序, 可能需要执行 `sudo chown -R openbmclapi /opt/openbmclapi` 指令修复权限
3. 配置`/opt/openbmclapi/config.yaml`配置文件
4. 使用`systemctl start go-openbmclapi.service`启动服务
   - 使用`systemctl reload go-openbmclapi.service`可重新加载配置文件
   - 使用`systemctl stop go-openbmclapi.service`停止服务
   - 使用`systemctl enable go-openbmclapi.service`让服务自启动
   - 使用`systemctl disable go-openbmclapi.service`禁止服务自启动
   - 使用`journalctl -f --output cat -u go-openbmclapi.service`实时监听日志

### 从源代码运行

1. 下载本仓库 _(可以使用`git clone https://github.com/LiterMC/go-openbmclapi.git`)_
2. 确保您的服务器上装有`go 1.21+`
3. cd进入本仓库
4. 配置配置文件或环境变量
5. 使用`go run .`运行本程序

### 第三方自更新+无依赖直接运行脚本

源代码位于 <https://github.com/8Mi-Tech/Shell/blob/main/go-openbmclapi-in-tmux.sh>

```
cd /opt/openbmclapi
wget -O start.sh https://raw.githubusercontent.com/8Mi-Tech/Shell/main/go-openbmclapi-in-tmux.sh
bash ./start.sh
```

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
# 跳过第一次同步, 直接启动节点 (debug用)
skip-first-sync: false
# 是否在连接断开后直接退出
exit-when-disconnected: false
# 日志最长保存时间 (天). 设置为 0 禁用清理过期日志
log-slots: 7
# 是否禁用 bmclapi 分发的证书, 同 CLUSTER_BYOC
byoc: false
# 是否仅从主服务器下载文件
noopen: false
# 跳过文件哈希值校验
no_heavy_check: false
# 是否信任 X-Forwarded-For 标头 (有反代时启用)
trusted-x-forwarded-for: false
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
# 文件同步间隔 (分钟)
sync_interval: 10
# 发送心跳包的超时限制 (秒), 网不好就调高点
keepalive_timeout: 10
# 同步文件时最多打开的连接数量
download_max_conn: 64
# 是否启用 gzip 压缩
use_gzip: false
# 服务器上行限制
serve_limit:
  # 是否启用上行限制
  enable: false
  # 最大连接数量
  max_conn: 16384
  # 上行速率限制 (KiB/s), 0 表示无限制
  upload_rate: 0

# 内置的仪表板
dashboard:
  # 是否启用
  enable: true
  # PWA 的名称, 在桌面设备上显示
  pwa-name: GoOpenBmclApi Dashboard
  # PWA 短名称, 在移动设备上显示
  pwa-short_name: GOBA Dash
  # PWA 描述
  pwa-description: Go-Openbmclapi Internal Dashboard

## 特殊要求: 重定向到 OSS
oss:
    # 是否启用该功能
    enable: false
    list:
          # 镜像文件夹路径
        - folder_path: oss_mirror
          # 镜像网络路径
          redirect_base: https://oss.example.com/base/paths
          # 启动之前生成 measure 文件夹 (默认为动态生成)
          pre-create-measures: false
          # 使用该子节点的概率 (非负整数)
          possibility: 0

## 特殊要求: 劫持 bmclapi 以加速同一局域网请求
hijack:
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

## 子命令

Go-OpenBmclAPI 提供了一组子命令:
```
Sub commands:
  help
        显示帮助消息

  main | serve | <empty>
        执行主程序

  license
        打印完整许可证

  version
        打印程序版本

  zip-cache [options ...]
        使用 gzip 压缩 cache 文件夹内的文件 (迁移用)

    Options:
      verbose | v : 显示正在压缩的文件
      all | a : 压缩所有文件 (默认不会压缩10KB以下的文件)
      overwrite | o : 覆盖存在的已压缩的目标文件
      keep | k : 不删除压缩过的文件

  unzip-cache [options ...]
         解压缩 cache 文件夹内的文件 (迁移用)

    Options:
      verbose | v : 显示正在解压缩的文件
      overwrite | o : 覆盖存在的未压缩的目标文件
      keep | k : 不删除解压缩过的文件
```

## 致谢

[OpenBMCLAPI-TypeScript](https://github.com/bangbang93/openbmclapi)（[@bangbang93](https://github.com/bangbang93)）
