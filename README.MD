
Go OpenBmclAPI
[![License](https://img.shields.io/github/license/LiterMC/go-openbmclapi)](https://github.com/LiterMC/go-openbmclapi/blob/master/LICENSE)
[![Build Status](https://img.shields.io/github/actions/workflow/status/LiterMC/go-openbmclapi/build-tag.yml)](https://github.com/LiterMC/go-openbmclapi/actions)
[![Downloads](https://img.shields.io/github/downloads/LiterMC/go-openbmclapi/total)](https://github.com/LiterMC/go-openbmclapi/releases)
==============

Golang 版的 [OpenBmclAPI](https://github.com/bangbang93/openbmclapi)

> 注: 本仓库部分代码按照 <https://github.com/bangbang93/openbmclapi> 编写, **非官方, 不保证时效性**

*如果本项目有用, 就给个 star ⭐️ 吧 :)*

欢迎进行 PR.

-> [完整贡献者列表](./CONTRIBUTORS.md)

## 特性

- 支持代理 BMCLAPI 请求, 并使用本地缓存加速请求
- 使用协程/多线程, 超高的文件同步速度
- ~~不依赖大量的三方包, 体积小巧~~
- 静态文件, 无需配置任何依赖
- 得益于 Golang 强大的跨平台编译器, 支持大部分主流平台/处理器
- 内置一个统计面板
- 支持在面板内实时查看日志 (需登录)
- 支持自动化打洞程序
- 支持在节点上线/下线时发送通知
- 支持一些小众的~~奇怪~~需求
- ~~更好的压榨节点~~

## FAQ

Q: 如何运行 / 配置该程序?  
A: 请查看 [`docs`](./docs) 文件夹下的文档

Q: 支持`.env`文件吗?  
A: 不支持, Go-OpenBmclAPI 使用 yaml 配置文件, 该文件应位于程序的运行目录. 初次运行本程序时会自动创建该配置文件

Q: 没有适合我的平台的程序怎么办?  
A: 检查 [Go Platform Values](https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63#platform-values), 如果您的系统/架构存在, 那么可以发个 issue

Q: 版本号格式  
A: `v<openbmclapi 版本>-<go-openbmclapi 构建计数>`, 例: `v1.6.7-60`

Q: 面板在哪里访问?  
A: 服务端启动时会输出 `https://<public-host>:<public-port>`, 访问这个地址就可以了
   > 注: 面板与 openbmclapi 下载服务共用一个端口, 并非单独配置

Q: 检查文件的时候过慢怎么办?  
A: 如果您确定您的存储稳定, 将 `config.yaml` 下的 `no-heavy-check` 选项设为 `true` 即可

Q: 程序无法运行 / 直接退出怎么办?  
A: 请将运行目录下 `logs` 文件夹内最新的日志上传到 [Github Issue](https://github.com/LiterMC/go-openbmclapi/issues/new)

Q: 面板日志一直无法连接?  
A: 请确认中间所有的反向代理都支持 WebSocket

## 安装

注意: LiterMC-RootCA 的 SHA1 指纹为 `A2:CB:34:70:25:CF:5D:F6:57:1C:67:81:35:49:7D:6D:A7:26:F7:0E`

### 无依赖直接运行

1. 从 [Github Release](https://github.com/LiterMC/go-openbmclapi/releases/latest) 找到**适合您服务器平台**的程序并下载到节点服务器上
2. 配置配置文件, 可以直接使用与bangbang93的openbmclapi相同的环境变量配置, 也可以从`config.yaml`进行配置 _(下文有讲)_
3. 运行程序

### 从docker运行

- 可直接运行仓库目录下的 [`scripts/docker-run.sh`](./scripts/docker-run.sh) 文件, 运行之前请确保存在`craftmine/go-openbmclapi:latest`镜像, 或网络连通
- 也可使用`docker build -t craftmine/go-openbmclapi:latest .`手动编译镜像

### 从`systemd`运行 _(仅Linux)_

1. **确保systemd是您的*启动进程***
2. 执行
   ```sh
   curl -fsSL https://raw.githubusercontent.com/LiterMC/go-openbmclapi/HEAD/installer/service/installer.sh | sudo bash -s
   ```
   > 注意, 新版新增使用 `openbmclapi` 用户执行程序, 可能需要执行 `sudo chown -R openbmclapi /opt/openbmclapi` 指令修复权限

   国内对 Github 的支持较差, 可以使用 ghproxy 等镜像站运行脚本, 本例中使用了 [crashmc.com](https://crashmc.com) 提供的 CDN:
   ```sh
   MIRROR_PREFIX=https://cdn.crashmc.com/
   curl -fsSL ${MIRROR_PREFIX}https://raw.githubusercontent.com/LiterMC/go-openbmclapi/HEAD/installer/service/installer.sh | sudo bash -s --mirror "${MIRROR_PREFIX}"
   ```

   如果需要下载指定版本, 只需设置 `-t|--tag` 标志即可
   ```sh
   curl -fsSL https://raw.githubusercontent.com/LiterMC/go-openbmclapi/HEAD/installer/service/installer.sh | sudo bash -s --tag v1.9.7-8
   ```
3. 配置`/opt/openbmclapi/config.yaml`配置文件
4. 使用`systemctl start go-openbmclapi.service`启动服务
   - 使用`systemctl reload go-openbmclapi.service`可重新加载配置文件
   - 使用`systemctl stop go-openbmclapi.service`停止服务
   - 使用`systemctl enable go-openbmclapi.service`让服务自启动
   - 使用`systemctl disable go-openbmclapi.service`禁止服务自启动
   - 使用`journalctl -f --output cat -u go-openbmclapi.service`实时监听日志

### 从源代码运行

1. 确保您的服务器上装有 `go 1.21+` 以及 `node & npm`
2. 下载本仓库 _(可以使用`git clone https://github.com/LiterMC/go-openbmclapi.git`)_
3. cd 进入本仓库
4. 配置配置文件或环境变量
5. 使用 `go generate . && go run .` 运行本程序

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
# 是否不打印访问信息
no-access-log: false
# 最多输出几个 1MB 的访问日志
access-log-slots: 16
# 日志最长保存时间 (天). 设置为 0 禁用清理过期日志
log-slots: 7
# 是否不使用 bmclapi 分发的证书, 同 CLUSTER_BYOC
byoc: false
# 是否提供自己的证书. 启用后同时需要设置下面的 certificates 字段
use-cert: false
# 是否信任 X-Forwarded-For 标头 (有反代时启用)
trusted-x-forwarded-for: false
# 实际开放的公网主机名, 同 CLUSTER_IP
public-host: example.com
# 实际开放的公网端口, 同 CLUSTER_PUBLIC_PORT
public-port: 8080
# 要监听的本地端口, 同 CLUSTER_PORT
port: 4000
# openbmclapi 的 CLUSTER_ID
cluster-id: ${CLUSTER_ID}
# openbmclapi 的 CLUSTER_SECRET
cluster-secret: ${CLUSTER_SECRET}
# 文件同步间隔 (分钟)
sync-interval: 10
# 仅在程序开始时清理文件
only-gc-when-start: false
# 按需同步时最大可打开的连接数量.
download-max-conn: 64
# 连接主控的最大重试数, 0 表示不重试, -1 表示无限制
max-reconnect-count: 10

# 证书列表. 仅当 use-cert 为 true 时才会加载. 不受 byoc 影响
certificates:
  - cert: /path/to/cert.pem # 证书路径
    key: /path/to/key.pem   # 私钥路径

# 打洞程序配置
tunneler:
  # 是否使用打洞
  enable: false
  # 打洞程序/脚本路径. 注意: 暂不支持传递参数. 若要指定参数, 请在脚本内带参调用目标程序
  tunnel-program: ./path/to/tunnel/program
  # 打洞程序 host & port 输出, 使用 regex 格式
  output-regex: \bNATedAddr\s+(?<host>[0-9.]+|\[[0-9a-f:]+\]):(?<port>\d+)$
  # 打洞超时, 暂无用处
  tunnel-timeout: 0

# 缓存
cache:
  # 缓存类型:
  #   nocache: 不缓存
  #   inmem: 程序内内存缓存
  #   redis: Redis 缓存
  type: redis
  # 如果使用 Redis 缓存则还需要配置用户名密码等:
  data:
    network: tcp
    addr: "redis.server.hostname.example.com:6379"
    client-name: "go-openbmclapi"
    username: redis-username
    password: redis-password

# 服务器上行限制
serve-limit:
  # 是否启用上行限制
  enable: false
  # 最大连接数量, -1 表示无限制
  max-conn: 16384
  # 上行速率限制 (KiB/s), 0 表示无限制
  upload-rate: 0

# API 速率限制. 注意: 该功能仅限制对 /api 路径的访问, 不会影响 openbmclapi 基本功能
api-rate-limit:
  # 未登录用户的速率限制, 以访问 IP 为准 (会被 trusted-x-forwarded-for 标志影响)
  anonymous:
    per-minute: 10 # 每分钟最多请求数, 0 表示不限制
    per-hour: 120  # 每小时最多请求数, 0 表示不限制
  # 已登录用户的速率限制, 以用户名为准
  logged:
    per-minute: 120
    per-hour: 6000

# 通知设置
notification:
  # 启用邮件通知
  enable-email: false
  # 邮件 SMTP 服务器及其端口
  email-smtp: smtp.example.com:25
  # SMTP 服务器加密类型
  # - none : 无加密
  # - ssl  : 使用 SSL/TLS 加密
  # - tls  : 使用 STARTTLS 加密
  email-smtp-encryption: tls
  # 通知者邮箱用户名
  email-sender: noreply@example.com
  # 通知者邮箱密码
  email-sender-password: example-password
  # 启用 Webhook (TODO)
  enable-webhook: true

# 内置的仪表板
dashboard:
  # 是否启用
  enable: true
  # 登录用的用户名, 留空表示禁止登录
  username: ""
  # 登录用的密码, 留空表示禁止登录
  password: ""
  # PWA 的名称, 在桌面设备上显示
  pwa-name: GoOpenBmclApi Dashboard
  # PWA 短名称, 在移动设备上显示
  pwa-short_name: GOBA Dash
  # PWA 描述
  pwa-description: Go-Openbmclapi Internal Dashboard
  # 签名推送 JWT 时的 subject, 请将其设为 "mailto:<您的邮箱>" 或 "https://<您的域名>"
  notification-subject: mailto:user@example.com

# Github API 客户端配置
github-api:
  # 更新检测间隔
  update-check-interval: 1h0m0s
  # Github API 访问令牌, 绕过IP速率限制
  # 请访问 <https://github.com/settings/tokens> 生成新的无权限令牌
  authorization: Bearer ghp_xxxx

# 数据库
database:
  # 数据库驱动, 可选值有:
  # - memory # 内存数据库, 无持久化, 不需 DSN
  # - sqlite   # sqlite 文件存储. DSN 为文件路径
  # - mysql    # MySQL, DSN 见 <https://github.com/go-sql-driver/mysql?tab=readme-ov-file#dsn-data-source-name>
  # - postgres # PostgreSQL, DSN 见 <https://pkg.go.dev/github.com/lib/pq#hdr-Connection_String_Parameters>
  driver: sqlite
  # 数据库的 DSN. 请参考上文
  data-source-name: files.db

# BMCLAPI 代理, 会处理所有文件下载请求, 并将其他请求转发到 BMCLAPI 主服务器
hijack: # 注: 虽然名字是叫(hijack)劫持, 但其实它就是个代理
  # 是否启用代理. 代理会在 /bmclapi/ 子路径下开启服务.
  enable: false
  # 是否启用本地缓存 (这样就可以离线访问访问过的端点了)
  enable-local-cache: false
  # 本地缓存保存位置
  local-cache-path: hijack_cache
  # 是否需要登录才能访问
  require-auth: true
  # 用户名密码对
  auth-users:
    - username: example-username
      password: example-password # ❌ 请不要保持该密码 ❌

# 子存储节点列表
# 注意: measure 测量请求总是以第一个存储为准
storages:
  # local 为本地存储
  - type: local
    # 节点 ID
    id: local-storage-1
    # 使用该子节点的概率 (非负整数)
    weight: 100
    # 节点附加数据
    data:
      # cache 文件夹的路径
      cache-path: cache
      # 压缩方式 (目前未使用)
      compressor: ""
  # mount 为网络存储 (与旧版 oss 选项含义大致相同)
  - type: mount
    # 节点 ID
    id: mount-storage-1
    # 使用该子节点的概率 (非负整数)
    # 设为 0 将使该子节点成为备选节点 (若该节点前一个节点失效才会使用该节点), 如果所有子节点均为 0 则平均分配
    weight: 0
    # 节点附加数据
    data:
      # 文件夹路径
      path: oss_mirror
      # 对应的网络URL路径
      redirect-base: https://oss.example.com/base/paths
      # 启动之前在 measure 子文件夹内生成 1-200MB 的测速文件 (默认为动态生成)
      pre-gen-measures: false
  # webdav 使用 webdav 存储
  - type: webdav
    # 节点 ID
    id: webdav-storage-1
    # 使用该子节点的概率 (非负整数)
    weight: 100
    # 节点附加数据
    data:
      # 最多同时发起的连接数
      max-conn: 24
      # 最大上传速率 (KiB/s), 0 表示无限制
      max-upload-rate: 0
      # 最大下载速率 (KiB/s), 0 表示无限制
      max-download-rate: 0
      # 启动之前生成 1-200MB 的测速文件 (默认为动态生成)
      pre-gen-measures: false
      # 设置为 true 后将跟踪 302 请求 (即不会将最终用户重定向到网盘)
      follow-redirect: false
      # 重定向链接的缓存时间, 仅当 follow-redirect 为 false 时有用. 0 表示不缓存重定向链接
      redirect-link-cache: 0s
      # 链接到下方 webdav-users 的键值对
      alias: example-user
      # 相对于 alias 中的 Webdav 入口 URL **注意⚠️: 不要使用非 ascii (包括中文) 路径**
      endpoint: ../optional/another/endpoint/
      # [可选] 覆盖 alias 中的用户名
      username: optional-another-username
      # [可选] 覆盖 alias 中的密码
      password: optional-another-password

webdav-users:
    example-user:
        # Webdav 入口 URL **注意⚠️: 不要使用非 ascii (包括中文) 路径**
        endpoint: https://webdav.example.com/path/to/endpoint/
        # 用户名
        username: example-username
        # 密码
        password: example-password

# 以下为高级选项, 通常用于调试. **❌ 如果不理解其工作原理请不要碰 ❌**
advanced:
  # 是否打印调试日志
  debug-log: false
  # 是否记录 socket.io 流的日志 (仅在打开 debug-log 后才会输出到标准输出)
  socket-io-log: false
  # 跳过文件哈希值校验
  no-heavy-check: true
  # 不删除未使用的文件对象 **注意⚠️: 该选项打开后磁盘使用率会随时间增长**
  no-gc: false
  # 两次哈希校验之间间隔几次简单检查
  heavy-check-interval: 120
  # 发送心跳包的超时限制 (秒), 网不好就调高点
  keepalive-timeout: 10
  # 跳过第一次同步, 直接启动节点. **⚠️ 请保持该选项为 false ⚠️**
  skip-first-sync: false
  # 不检查请求签名. **⚠️ 请保持该选项为 false ⚠️**
  skip-signature-check: false
  # 是否在连接断开后直接退出
  exit-when-disconnected: false
  # 不执行快速上线
  no-fast-enable: false
  # 上线前等待几秒
  wait-before-enable: 0
  # ⚠️ 不进行安全域名重定向
  do-NOT-redirect-https-to-SECURE-hostname: false
  # ⚠️ 当您阅读完全部 README 后, 将该选项设为 true 即可避免打开浏览器
  do-not-open-faq-on-windows: false

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

  upload-webdav
        将本地 cache 文件夹上传到 webdav 存储
        上传之前请确保 config.yaml 下存在至少一个 local 存储和至少一个 webdav 存储
```

## 致谢

[OpenBMCLAPI-TypeScript](https://github.com/bangbang93/openbmclapi)（[@bangbang93](https://github.com/bangbang93)）
