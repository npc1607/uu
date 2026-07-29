# Namespace 游戏流量通过主机 eBPF 拦截方案（归档设计草案）

> 本文是未完成、未验证的研究草案，不代表当前实现。项目已移除 `ns-*` 命令，因为原实现只能证明 namespace 和进程被创建，不能证明游戏流量经过主机 `dae0`/eBPF 加速路径。重新引入前还需要补全并实机验证转发、NAT/policy routing、DNS、回程路径和 eBPF 命中。

## 1. 问题背景

### 1.1 核心问题
uuplugin 的 eBPF 加速机制依赖主机的 `dae0` 设备。在 namespace 模式下：
- uuplugin 在 namespace 内运行时，无法访问主机的 dae0
- namespace 内的游戏流量无法被 eBPF 拦截加速

### 1.2 架构分析
```
当前架构（有问题）:
┌─────────────────────────────────────────────────────────────┐
│ 主机                                                         │
│ ┌─────────────────────┐  ┌─────────────────────────────┐   │
│ │ uuplugin/monitor    │  │ dae0 (eBPF 拦截点)          │   │
│ │ (在 namespace 内)   │  │                             │   │
│ └─────────────────────┘  └─────────────────────────────┘   │
│ ┌─────────────────────────────────────────────────────────┐│
│ │ network namespace (uu-ns)                               ││
│ │ ┌──────────────┐                                        ││
│ │ │ 游戏/Steam    │──traffic──X──> 无法到达 dae0          ││
│ │ └──────────────┘                                        ││
│ │              ↑                                           ││
│ │         ipvlan/macvlan                                   ││
│ └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘

目标架构（本方案）:
┌─────────────────────────────────────────────────────────────┐
│ 主机                                                         │
│ ┌─────────────────────┐  ┌─────────────────────────────┐   │
│ │ uuplugin/monitor    │  │ dae0 (eBPF 拦截点)          │   │
│ │ (在主机上运行)       │  │                             │   │
│ └─────────────────────┘  └─────────────────────────────┘   │
│                              ↑                               │
│ ┌─────────────────────────────────────────────────────────┐│
│ │ network namespace (uu-ns)                               ││
│ │ ┌──────────────┐                                        ││
│ │ │ 游戏/Steam    │──traffic──> veth ──> 主机 ──> dae0    ││
│ │ └──────────────┘                                        ││
│ │                                                       ││
│ │ 默认路由: default via 169.254.127.1 dev uu_uu-ns_ns   ││
│ └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### 1.3 解决方案思路
**混合架构** - uuplugin 在主机运行，namespace 流量通过 veth 路由到主机被拦截

## 2. 详细设计

### 2.1 整体流程
```
启动流程:
1. createNamespace()          - 创建 network namespace
2. setupNamespaceProxyRouting() - 创建 veth pair 并配置路由
3. startNamespaceMonitor()    - 在主机启动 monitor/uuplugin
4. waitNamespacePlugin()      - 等待 uuplugin 启动完成
5. setupSocatForwarding()     - 配置 Web UI 端口转发

停止流程:
1. stopSocatForwarding()      - 停止 socat 转发
2. 删除 veth pair             - 清理网络资源
3. namespacePIDs()            - 获取 namespace 进程
4. 终止进程                   - 停止所有相关进程
5. 删除 namespace             - 清理 namespace
```

### 2.2 网络配置详解

#### Veth Pair 配置
```bash
# 主机端
veth 名称: uu_uu-ns_host
IP 地址: 169.254.127.1/24
状态: UP

# Namespace 端
veth 名称: uu_uu-ns_ns
IP 地址: 169.254.127.2/24
状态: UP

# 路由配置
主机路由: 169.254.127.0/24 dev uu_uu-ns_host
Namespace 默认路由: default via 169.254.127.1 dev uu_uu-ns_ns
```

#### 流量路径
```
游戏进程 -> namespace 网络栈 -> uu_uu-ns_ns (veth)
    -> uu_uu-ns_host (veth, 主机)
    -> 主机网络栈 -> dae0 (eBPF 拦截)
    -> 加速后的路由 -> 互联网
```

### 2.3 代码实现位置

#### 文件: internal/app/namespace.go

##### 1. namespaceStartLocked() - 调用 veth 设置
**位置**: Line 167-172
**修改**: 在 createNamespace() 后添加 setupNamespaceProxyRouting() 调用

```go
if err := i.createNamespace(cfg); err != nil {
    return err
}
// 新增: 创建 veth pair 并配置路由
if err := i.setupNamespaceProxyRouting(cfg); err != nil {
    return err
}
if err := i.startNamespaceMonitor(cfg); err != nil {
    _ = i.namespaceStopConfig(cfg)
    return err
}
```

##### 2. setupNamespaceProxyRouting() - 创建 veth 并配置路由
**位置**: Line 426-468
**功能**: 完整的 veth pair 创建和路由配置

```go
func (i *App) setupNamespaceProxyRouting(cfg namespaceConfig) error {
    vethHost := "uu_" + cfg.Name + "_host"  // 例如: uu_uu-ns_host
    vethNs := "uu_" + cfg.Name + "_ns"      // 例如: uu_uu-ns_ns

    // 步骤 1: 创建 veth pair
    if err := i.runCommand("ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethNs); err != nil {
        return fmt.Errorf("create veth pair: %w", err)
    }

    // 步骤 2: 将 namespace 端移入 namespace
    if err := i.runCommand("ip", "link", "set", vethNs, "netns", cfg.Name); err != nil {
        _ = i.runSilent("ip", "link", "delete", vethHost)
        return fmt.Errorf("move veth to namespace: %w", err)
    }

    // 步骤 3: 启动两端接口
    if err := i.runCommand("ip", "link", "set", vethHost, "up"); err != nil {
        return fmt.Errorf("bring up host veth: %w", err)
    }
    if err := i.runCommand("ip", "-n", cfg.Name, "link", "set", vethNs, "up"); err != nil {
        return fmt.Errorf("bring up namespace veth: %w", err)
    }

    // 步骤 4: 分配 IP 地址
    if err := i.runCommand("ip", "addr", "add", "169.254.127.1/24", "dev", vethHost); err != nil {
        return fmt.Errorf("assign IP to host veth: %w", err)
    }
    if err := i.runCommand("ip", "-n", cfg.Name, "addr", "add", "169.254.127.2/24", "dev", vethNs); err != nil {
        return fmt.Errorf("assign IP to namespace veth: %w", err)
    }

    // 步骤 5: 添加主机路由
    if err := i.runCommand("ip", "route", "add", "169.254.127.0/24", "dev", vethHost); err != nil {
        i.logf("add route for veth warning: %v", err)
    }

    // 步骤 6: 替换 namespace 默认路由 (关键!)
    // 将所有 namespace 流量路由到 veth
    if err := i.runCommand("ip", "-n", cfg.Name, "route", "replace", "default",
        "via", "169.254.127.1", "dev", vethNs); err != nil {
        return fmt.Errorf("add default route via veth: %w", err)
    }

    i.logf("namespace proxy routing configured: %s <-> %s", vethHost, vethNs)
    return nil
}
```

##### 3. namespaceStopConfig() - 清理 veth
**位置**: Line 204-216
**修改**: 在停止 socat 转发后添加 veth 清理

```go
func (i *App) namespaceStopConfig(cfg namespaceConfig) error {
    // Stop socat forwarding first
    _ = i.stopSocatForwarding()

    // 新增: 清理 veth pair
    vethHost := "uu_" + cfg.Name + "_host"
    if err := i.runSilent("ip", "link", "delete", vethHost); err != nil {
        i.logf("delete veth %s warning: %v", vethHost, err)
    } else {
        i.logf("removed veth pair: %s", vethHost)
    }

    // ... 后续清理代码 ...
}
```

##### 4. startNamespaceMonitor() - 在主机运行
**位置**: Line 470-473
**修改**: monitor 在主机运行（不在 namespace 内）

```go
func (i *App) startNamespaceMonitor(cfg namespaceConfig) error {
    // 在主机上运行 monitor，不是在 namespace 内
    cmd := []string{i.params.monitorFile}
    return i.startDetached("/bin/sh", cmd...)
}
```

##### 5. waitNamespacePlugin() - 等待主机上的 uuplugin
**位置**: Line 481-493
**修改**: 等待主机上而不是 namespace 内的 uuplugin

```go
func (i *App) waitNamespacePlugin(name string, timeout time.Duration) error {
    // 等待主机上的 uuplugin 启动
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        pids, err := plugin.PIDsByPattern(plugin.Executable)
        if err == nil && len(pids) > 0 {
            return nil
        }
        time.Sleep(time.Second)
    }
    return fmt.Errorf("%s did not start on host", plugin.Executable)
}
```

## 3. 验证步骤

### 3.1 启动验证
```bash
# 1. 启动 namespace
sudo ./uu ns-start

# 2. 检查 veth 接口是否存在
ip link show | grep uu_
# 预期输出: uu_uu-ns_host@if15: <BROADCAST,MULTICAST,UP,LOWER_UP>

# 3. 检查 veth IP 配置
ip addr show uu_uu-ns_host
# 预期: inet 169.254.127.1/24

sudo ip netns exec uu-ns ip addr show uu_uu-ns_ns
# 预期: inet 169.254.127.2/24

# 4. 检查 namespace 路由表
sudo ip netns exec uu-ns ip route show
# 预期: default via 169.254.127.1 dev uu_uu-ns_ns

# 5. 检查 dae0 是否存在 (uuplugin 创建)
ip link show dae0
# 预期: dae0@ifX: <BROADCAST,MULTICAST,UP,LOWER_UP>

# 6. 检查进程
ps aux | grep uuplugin | grep -v grep
# 预期: uuplugin 进程在运行
```

### 3.2 连接性验证
```bash
# 1. 在 namespace 内测试 DNS
sudo ip netns exec uu-ns nslookup google.com
# 预期: 能解析域名

# 2. 测试外网连接
sudo ip netns exec uu-ns ping -c 3 8.8.8.8
# 预期: 能 ping 通

# 3. 测试 HTTP 连接
sudo ip netns exec uu-ns curl -I https://www.google.com
# 预期: 能访问
```

### 3.3 流量验证
```bash
# 1. 在主机上抓包
sudo tcpdump -i uu_uu-ns_host -n

# 2. 在 namespace 内发起连接
sudo ip netns exec uu-ns curl https://www.google.com

# 3. 检查是否在 veth 上看到流量
# 预期: tcpdump 应该显示流量
```

### 3.4 停止验证
```bash
# 1. 停止 namespace
sudo ./uu ns-stop

# 2. 检查 veth 是否已删除
ip link show | grep uu_
# 预期: 无输出

# 3. 检查 namespace 是否已删除
sudo ip netns list
# 预期: 没有 uu-ns

# 4. 检查进程是否已停止
ps aux | grep uuplugin | grep -v grep
# 预期: 无 uuplugin 进程
```

## 4. 故障排查

### 4.1 veth 创建失败
**症状**: 启动失败，日志显示 "create veth pair"
**原因**: 权限不足或系统限制
**解决**:
```bash
# 检查是否有 root 权限
sudo -v

# 检查是否已存在同名 veth
ip link show | grep uu_
sudo ip link delete uu_uu-ns_host 2>/dev/null
```

### 4.2 路由配置失败
**症状**: namespace 内无法连接外网
**检查**:
```bash
# 检查 namespace 路由
sudo ip netns exec uu-ns ip route show
# 应该看到: default via 169.254.127.1 dev uu_uu-ns_ns

# 手动添加路由测试
sudo ip netns exec uu-ns ip route replace default via 169.254.127.1 dev uu_uu-ns_ns
```

### 4.3 uuplugin 未启动
**症状**: waitNamespacePlugin 超时
**检查**:
```bash
# 检查 monitor 日志
tail -f /tmp/monitor.log

# 检查 uuplugin 日志
tail -f /home/pcong/project/uu/bin/runtime/uuplugin.log

# 手动启动测试
sudo /home/pcong/project/uu/bin/uuplugin_monitor.sh
```

### 4.4 dae0 不存在
**症状**: uuplugin 启动但 dae0 不存在
**原因**: uuplugin 版本问题或配置问题
**解决**:
```bash
# 检查 uuplugin 版本
strings /home/pcong/project/uu/bin/runtime/uuplugin | grep dae

# 重新下载 uuplugin
rm /home/pcong/project/uu/bin/runtime/uu.tar.gz
sudo ./uu ns-start
```

## 5. 技术细节说明

### 5.1 为什么使用 169.254.127.0/24
- 169.254.0.0/16 是 link-local 地址范围，不需要路由
- 避免与用户网络冲突
- 容易识别和调试

### 5.2 为什么需要 route replace
- ipvlan 设备可能自动继承父设备的路由
- 使用 `replace` 确保默认路由指向 veth
- 覆盖可能存在的旧路由

### 5.3 为什么 monitor 在主机运行
- uuplugin 需要创建 dae0 设备和 eBPF 程序
- 这些操作需要主机网络权限
- namespace 内无法访问主机的网络设备

### 5.4 为什么需要 socat 转发
- Web UI (127.0.0.1:8088) 在主机上监听
- uuplugin 在主机上运行，但需要转发 namespace 内的请求
- socat 实现 host -> namespace 的端口转发

## 6. 相关文件清单

| 文件路径 | 行数 | 功能 |
|---------|------|------|
| internal/app/namespace.go | 170-172 | 调用 veth 设置 |
| internal/app/namespace.go | 426-468 | veth 创建和路由配置 |
| internal/app/namespace.go | 204-216 | veth 清理 |
| internal/app/namespace.go | 470-473 | monitor 主机运行 |
| internal/app/namespace.go | 481-493 | 等待主机 uuplugin |
| internal/app/namespace.go | 1080-1120 | socat 转发设置 |
| internal/app/namespace.go | 1122-1135 | socat 停止 |
