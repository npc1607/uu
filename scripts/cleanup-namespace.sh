#!/bin/bash
# Namespace 模式清理脚本
# 删除 uu-ns namespace 及所有相关副产物

set -e

# 默认 namespace 名称
NAMESPACE="${1:-uu-ns}"
LINK_NAME="${2:-uu-ipvlan0}"
VETH_HOST="uu_${NAMESPACE}_host"

echo "=== Namespace 清理脚本 ==="
echo "Namespace: $NAMESPACE"
echo "Link: $LINK_NAME"
echo ""

# 1. 停止 systemd 服务（如果存在）
echo "1. 停止 systemd 服务..."
if systemctl is-active --quiet uu-ns 2>/dev/null; then
    sudo systemctl stop uu-ns
    echo "  已停止 uu-ns 服务"
fi
if systemctl is-enabled --quiet uu-ns 2>/dev/null; then
    sudo systemctl disable uu-ns
    echo "  已禁用 uu-ns 服务"
fi

# 2. 终止相关进程
echo "2. 终止相关进程..."

# 终止 socat 转发进程
if [ -f "/var/run/uuplugin-socat.pid" ]; then
    while read -r pid; do
        if kill -0 "$pid" 2>/dev/null; then
            sudo kill "$pid" 2>/dev/null || true
            echo "  已终止 socat 进程 (pid=$pid)"
        fi
    done < /var/run/uuplugin-socat.pid
    sudo rm -f /var/run/uuplugin-socat.pid
fi

# 终止 namespace 内的所有进程
if sudo ip netns list | grep -q "^${NAMESPACE}"; then
    PIDS=$(sudo ip netns pids "${NAMESPACE}" 2>/dev/null || true)
    if [ -n "$PIDS" ]; then
        echo "  终止 namespace 内进程: $PIDS"
        for pid in $PIDS; do
            sudo kill "$pid" 2>/dev/null || true
        done
        sleep 1
        # 强制终止
        for pid in $PIDS; do
            sudo kill -9 "$pid" 2>/dev/null || true
        done
    fi
fi

# 终止 uuplugin monitor（主机上的）
if pgrep -f "uuplugin_monitor.sh" > /dev/null; then
    sudo pkill -f "uuplugin_monitor.sh" || true
    echo "  已终止 monitor 进程"
fi

# 3. 删除 veth pair
echo "3. 删除 veth pair..."
if ip link show "$VETH_HOST" &>/dev/null; then
    sudo ip link delete "$VETH_HOST"
    echo "  已删除 veth: $VETH_HOST"
fi

# 4. 删除 network namespace
echo "4. 删除 network namespace..."
if sudo ip netns list | grep -q "^${NAMESPACE}"; then
    sudo ip netns delete "${NAMESPACE}"
    echo "  已删除 namespace: $NAMESPACE"
fi

# 5. 删除 ipvlan/macvlan 接口
echo "5. 删除网络接口..."
if ip link show "$LINK_NAME" &>/dev/null; then
    sudo ip link delete "$LINK_NAME"
    echo "  已删除接口: $LINK_NAME"
fi

# 6. 删除 /etc/netns 配置目录
echo "6. 删除配置目录..."
if [ -d "/etc/netns/${NAMESPACE}" ]; then
    sudo rm -rf "/etc/netns/${NAMESPACE}"
    echo "  已删除 /etc/netns/${NAMESPACE}"
fi

# 7. 删除 systemd 服务文件（如果存在）
echo "7. 清理 systemd 服务..."
SERVICE_FILE="/etc/systemd/system/uu-ns.service"
if [ -f "$SERVICE_FILE" ]; then
    sudo rm -f "$SERVICE_FILE"
    sudo systemctl daemon-reload
    echo "  已删除服务文件: $SERVICE_FILE"
fi

# 8. 清理运行时文件
echo "8. 清理运行时文件..."
sudo rm -f /var/run/uuplugin.pid
sudo rm -f /var/run/uuplugin-socat.pid

echo ""
echo "=== 清理完成 ==="
echo ""
echo "验证："
echo "  Namespace: $(sudo ip netns list | grep "^${NAMESPACE}" || echo '已删除')"
echo "  Veth: $(ip link show "$VETH_HOST" 2>/dev/null || echo '已删除')"
echo "  接口: $(ip link show "$LINK_NAME" 2>/dev/null || echo '已删除')"
