# uuplugin 与 daed 透明代理冲突分析报告

## 一、当前网络状态

当前电脑同时运行了网易 UU 主机版插件 `uuplugin` 和 daed 透明代理。

已观察到的状态：

```text
电脑局域网 IP: 192.168.1.3
UU TUN 接口: tun163
UU TUN 地址: 172.19.163.1/24
daed 接口: dae0
UU 服务: uuplugin active
daed 服务: daed active
daed Web UI: http://localhost:2023/
```

策略路由中存在：

```text
from all lookup 179
from all fwmark 0x163 lookup 163
```

其中：

```text
table 163: default via 172.19.163.1 dev tun163
table 179: 大量游戏/服务网段 via 172.19.163.1 dev tun163
```

这说明 UU 会通过 `fwmark` 和策略路由，把被识别为游戏加速的流量送进 `tun163`。

## 二、uuplugin 的工作原理

`uuplugin` 是网易 UU 主机版在 Linux/Steam Deck 环境下使用的本地插件。它的核心作用不是普通 HTTP 代理，而是给主机或局域网设备提供游戏加速通道。

大致链路如下：

```text
手机 / 掌机 / Steam Deck
        |
        | 局域网连接电脑上的 UU 插件
        v
电脑 uuplugin
        |
        | 识别游戏目标、DNS、连接信息
        v
tun163 / 策略路由 / fwmark
        |
        v
UU 加速节点
        |
        v
游戏服务器
```

`uuplugin` 主要做以下工作：

1. 在电脑上监听本地端口，给手机端 UU App 或主机版客户端连接。
2. 创建或使用 `tun163` 这样的虚拟网络接口。
3. 添加策略路由表，例如 `table 163`、`table 179`。
4. 对游戏相关目标 IP 打标或写路由，使其走 UU 加速路径。
5. 自己也会发起 DNS 查询、节点探测、控制连接、保活连接。

因此，`uuplugin` 本身既是服务端，也是会主动联网的客户端。

## 三、daed 透明代理的工作原理

daed 的透明代理通常会通过 `dae0`、nftables、策略路由、DNS 劫持等方式接管系统流量。

常见链路如下：

```text
本机进程 / 局域网转发流量
        |
        v
daed 透明代理规则
        |
        | 根据 domain / dip / pname / geoip 判断
        v
direct 或 proxy
```

当前 daed 规则类似：

```dae
pname(NetworkManager, systemd-resolved, dnsmasq) -> must_direct
dip(geoip:private) -> direct
dip(geoip:cn) -> direct
domain(geosite:cn) -> direct
fallback: proxy
```

这意味着：

- 系统 DNS 相关进程直连。
- 私网 IP 直连。
- 中国 IP 和中国域名直连。
- 其他默认走代理。

这个规则对普通桌面代理是合理的，但对 `uuplugin` 不够。因为 `uuplugin` 的很多连接目标是国外节点、公共 DNS、游戏节点，很容易被 `fallback: proxy` 接管。

## 四、冲突点

实际观察到 daed 日志中有类似内容：

```text
pname=uuplugin
8.8.8.8:53
outbound=proxy
```

这说明 daed 正在代理 `uuplugin` 自己的 DNS 或联网请求。

原本 UU 希望自己控制这条链路：

```text
uuplugin -> UU 节点 / DNS / 游戏服务 -> tun163
```

但 daed 介入后变成：

```text
uuplugin -> daed proxy -> 代理节点 -> UU 节点 / DNS / 游戏服务
```

某些情况下还可能出现：

```text
手机游戏流量 -> uuplugin -> tun163 -> daed -> proxy
```

或者：

```text
uuplugin DNS -> daed DNS -> proxy -> 影响 UU 节点选择
```

这样会导致以下问题：

1. UU 无法准确探测真实网络质量。
2. UU 的节点连接被 daed 二次代理，延迟和路径异常。
3. DNS 被 daed 改写或代理，UU 解析到的节点不一定适合当前网络。
4. 普通网页流量被 daed 接管，但 UU 的策略路由也在系统里存在，路由优先级可能互相影响。
5. 游戏流量仍然正常，是因为它已经被 UU 的 `fwmark` / `table 179` 送进 `tun163`，和普通网页流量不是同一条路径。

所以现象表现为：

```text
游戏正常
普通上网异常
开启 daed 后问题明显
```

这不是单纯 DNS 问题，而是两个透明代理系统同时接管网络导致的路由冲突。

## 五、解决原则

核心原则是：UU 的流量必须绕过 daed，daed 不要代理 `uuplugin`。

目标链路应该拆清楚：

```text
游戏加速流量 -> uuplugin -> tun163 -> UU
普通代理流量 -> daed -> proxy/direct
```

不要让它变成：

```text
uuplugin -> daed -> proxy
```

## 六、推荐修改

在 daed 的流量路由规则最前面加入：

```dae
pname(uuplugin) -> must_direct
dip(172.19.163.0/24) -> must_direct
```

完整规则建议改成：

```dae
pname(uuplugin) -> must_direct
dip(172.19.163.0/24) -> must_direct
pname(NetworkManager, systemd-resolved, dnsmasq) -> must_direct
dip(geoip:private) -> direct
dip(geoip:cn) -> direct
domain(geosite:cn) -> direct
fallback: proxy
```

如果 daed 支持接口匹配，也建议额外绕过 `tun163`：

```dae
iif(tun163) -> must_direct
oif(tun163) -> must_direct
```

具体语法要看当前 daed 版本支持情况。如果 Web UI 不接受 `iif/oif`，先只加 `pname` 和 `dip` 两条。

## 七、DNS 配置说明

以下配置是 DNS 查询规则：

```dae
upstream {
  alidns: 'udp://223.5.5.5:53'
  googledns: 'tcp+udp://8.8.8.8:53'
}
routing {
  request {
    qname(geosite:cn) -> alidns
    fallback: googledns
  }
}
```

它只能决定：

```text
国内域名 -> 阿里 DNS
其他域名 -> Google DNS
```

它不能解决 `uuplugin` 被 daed 代理的问题。

真正要改的是流量 routing，也就是包含以下内容的那一段：

```dae
fallback: proxy
```

## 八、验证方法

改完后，在 daed 日志里观察。

错误状态：

```text
pname=uuplugin outbound=proxy
```

正确状态应该是：

```text
pname=uuplugin outbound=direct
```

或者不再出现 `uuplugin` 被 daed 接管的记录。

同时测试：

1. 手机继续连接电脑上的 UU。
2. 手机游戏加速是否正常。
3. 电脑普通网页是否正常。
4. daed 面板日志里是否还有 `uuplugin -> proxy`。
5. 如果仍异常，重启服务：

```sh
sudo systemctl restart daed
sudo systemctl restart uuplugin
```

建议顺序是先启动 daed，再启动 uuplugin：

```sh
sudo systemctl restart daed
sudo systemctl restart uuplugin
```

## 九、排查与逆向分析命令

以下命令用于分析 `uuplugin` 的运行方式、监听端口、路由规则、DNS 行为和与 daed 的冲突点。默认以只读排查为主，不修改系统网络状态。

### 1. 查看服务和进程

查看 `uuplugin` 与 daed 服务状态：

```sh
systemctl status uuplugin --no-pager
systemctl status daed --no-pager
```

查看相关进程：

```sh
ps -eo pid,ppid,user,comm,args | rg -i 'uuplugin|uu|daed|dae'
```

查看 `uuplugin` 进程树：

```sh
pstree -aps "$(pidof uuplugin | awk '{print $1}')"
```

查看进程启动参数和环境变量：

```sh
tr '\0' ' ' < /proc/$(pidof uuplugin | awk '{print $1}')/cmdline
tr '\0' '\n' < /proc/$(pidof uuplugin | awk '{print $1}')/environ
```

### 2. 查看监听端口和连接

查看 `uuplugin` 监听的 TCP/UDP 端口：

```sh
ss -tunlp | rg -i 'uuplugin|:16363|:14554|:27036|tun163'
```

查看 `uuplugin` 当前连接的远端地址：

```sh
ss -tunp | rg -i 'uuplugin'
```

如果 `ss` 无法显示进程名，可以使用 root 权限：

```sh
sudo ss -tunlp
sudo ss -tunp
```

### 3. 查看网卡、地址和路由

查看网络接口：

```sh
ip -o addr show
```

查看 `tun163`：

```sh
ip addr show tun163
ip link show tun163
```

查看主路由表和策略路由：

```sh
ip route show table main
ip rule show
```

查看 UU 使用的路由表：

```sh
ip route show table 163
ip route show table 179
```

如果 `table 179` 输出非常多，可以只看前几行：

```sh
ip route show table 179 | sed -n '1,80p'
```

查看某个目标 IP 会走哪条路由：

```sh
ip route get 8.8.8.8
ip route get 8.8.8.8 mark 0x163
ip route get 8.8.8.8 mark 0x164
```

### 4. 查看 nftables / iptables 规则

daed 和透明代理通常会写 nftables 或 iptables 规则。查看 nftables：

```sh
sudo nft list ruleset
```

只筛选 daed、dae、tproxy、mark、tun163、uu 相关内容：

```sh
sudo nft list ruleset | rg -i 'dae|daed|tproxy|mark|tun163|uu|163|179'
```

查看 iptables：

```sh
sudo iptables-save
sudo ip6tables-save
```

筛选关键规则：

```sh
sudo iptables-save | rg -i 'dae|daed|tproxy|mark|tun163|uu|163|179'
```

### 5. 查看 daed 是否代理了 uuplugin

查看 daed 日志：

```sh
journalctl -u daed -f
```

筛选 `uuplugin`：

```sh
journalctl -u daed --since '30 min ago' | rg -i 'uuplugin|outbound=proxy|outbound=direct'
```

如果看到类似内容，说明 daed 正在代理 `uuplugin`：

```text
pname=uuplugin outbound=proxy
```

期望结果是：

```text
pname=uuplugin outbound=direct
```

或者不再出现 `uuplugin` 被 daed 接管的记录。

### 6. 抓包分析 DNS 和连接

观察 `uuplugin` 是否访问公共 DNS：

```sh
sudo tcpdump -ni any 'port 53 and (host 8.8.8.8 or host 223.5.5.5)'
```

观察 `tun163` 上的流量：

```sh
sudo tcpdump -ni tun163
```

观察本机局域网接口上的 UU 流量：

```sh
sudo tcpdump -ni enp34s0 'host 192.168.1.3 or net 172.19.163.0/24'
```

如果要同时保存抓包文件供 Wireshark 分析：

```sh
sudo tcpdump -ni any -w uuplugin-daed-debug.pcap
```

停止抓包后，用 Wireshark 打开 `uuplugin-daed-debug.pcap`，重点看：

- DNS 查询目标是否被 daed 改写。
- `uuplugin` 是否直接连接 UU 节点。
- 游戏流量是否进入 `tun163`。
- 是否存在重复代理或异常回环。

### 7. 分析 uuplugin 文件

查看 `uuplugin` 文件位置：

```sh
readlink -f /tmp/uu/uuplugin
ls -lh /tmp/uu/uuplugin /tmp/uu/uu.conf
file /tmp/uu/uuplugin
```

查看动态链接库：

```sh
ldd /tmp/uu/uuplugin
```

查看可见字符串：

```sh
strings -a /tmp/uu/uuplugin | rg -i 'http|https|dns|tun|route|iptables|nft|mark|163|server|proxy|udp|tcp'
```

查看 ELF 基本信息：

```sh
readelf -h /tmp/uu/uuplugin
readelf -d /tmp/uu/uuplugin
readelf -sW /tmp/uu/uuplugin | sed -n '1,120p'
```

查看二进制导入符号：

```sh
objdump -T /tmp/uu/uuplugin | rg -i 'socket|connect|send|recv|getaddrinfo|dns|route|setsockopt'
```

### 8. 跟踪系统调用

使用 `strace` 跟踪网络相关系统调用：

```sh
sudo strace -f -p "$(pidof uuplugin | awk '{print $1}')" -e trace=network
```

跟踪文件、网络、进程相关调用：

```sh
sudo strace -f -p "$(pidof uuplugin | awk '{print $1}')" -e trace=network,file,process
```

保存到文件：

```sh
sudo strace -ff -o uuplugin.strace -p "$(pidof uuplugin | awk '{print $1}')" -e trace=network,file,process
```

重点关注：

- `connect()` 连接到哪些远端 IP。
- `sendto()` / `recvfrom()` 是否访问 DNS。
- 是否读取 `/tmp/uu/uu.conf`。

## 十、落地结论修正

后续验证表明，不能把“`uuplugin` 进程能够在 namespace 中启动”等同于“游戏加速生效”。插件依赖主机网络栈中的 `dae0` 和相应 eBPF 拦截路径；把插件放入独立 namespace 后，它无法直接使用主机侧的拦截点，namespace 流量也不会自然经过该路径。

因此当前项目不再提供 `ns-*` 命令和对应 Web 控制页。已验证的落地方式是让官方 `uuplugin` 在主机网络 namespace 中运行，并在 daed 中为它配置最小直连规则：

```dae
pname(uuplugin) -> must_direct
```

独立 namespace 只能作为尚未完成的研究方向。若将来重新引入，必须先完成并实机证明 veth 转发、NAT 或 policy routing、DNS、回程路径以及 eBPF 实际命中；仅创建 macvlan/ipvlan 和独立 IP 不足以证明功能成立。相关设想记录在 `docs/namespace-routing-plan.md`，不属于当前支持能力。
- 是否操作路由、TUN、iptables 或 nftables。

### 9. 查看打开的文件和 socket

查看 `uuplugin` 打开的文件：

```sh
sudo lsof -p "$(pidof uuplugin | awk '{print $1}')"
```

只看网络 socket：

```sh
sudo lsof -Pan -p "$(pidof uuplugin | awk '{print $1}')" -i
```

查看是否打开了 TUN 设备：

```sh
sudo lsof -p "$(pidof uuplugin | awk '{print $1}')" | rg -i 'tun|net|dev'
```

### 10. 对比开启和关闭 daed 的差异

记录开启 daed 时的状态：

```sh
ip rule show > before.iprule.txt
ip route show table main > before.route-main.txt
ip route show table 163 > before.route-163.txt
ip route show table 179 > before.route-179.txt
sudo nft list ruleset > before.nft.txt
```

关闭或重启 daed 后再次记录：

```sh
ip rule show > after.iprule.txt
ip route show table main > after.route-main.txt
ip route show table 163 > after.route-163.txt
ip route show table 179 > after.route-179.txt
sudo nft list ruleset > after.nft.txt
```

对比差异：

```sh
diff -u before.iprule.txt after.iprule.txt
diff -u before.route-main.txt after.route-main.txt
diff -u before.route-163.txt after.route-163.txt
diff -u before.route-179.txt after.route-179.txt
diff -u before.nft.txt after.nft.txt | sed -n '1,200p'
```

重点关注：

- daed 是否新增了 fwmark 规则。
- daed 是否拦截了 `tun163`。
- daed 是否把 `uuplugin` 的 DNS 或控制连接送到了 `proxy`。
- UU 的 `table 163` 和 `table 179` 是否被覆盖或优先级异常。

## 十、结论

这个问题的根因不是 UU 本身坏了，也不是 daed 不能用，而是两个网络接管系统同时工作时边界没有划清。

最终方案是：

```text
daed 负责普通代理
UU 负责游戏加速
uuplugin / tun163 必须从 daed 透明代理中排除
```

最关键的修复规则是：

```dae
pname(uuplugin) -> must_direct
dip(172.19.163.0/24) -> must_direct
```

把它们放在 daed 路由规则最前面，通常就能避免 UU 被 daed 二次代理。
