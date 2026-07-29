#!/bin/sh


DATE_TIME=$(date +%s)
LOG_FILE="/tmp/uu_steam_deck_install_${DATE_TIME}.log"
rm /tmp/uu_steam_deck_install_*.log 1>/dev/null 2>&1
exec 2>${LOG_FILE} || true
set -x

# Online URL
URL_PREFIX="https://"
UNINSTALL_DOWNLOAD_URL="router.uu.163.com/api/script/uninstall?type="
MONITOR_DOWNLOAD_URL="router.uu.163.com/api/script/monitor?type="

ROUTER="steam-deck-plugin"
MODEL="x86_64"

BASEDIR=$(dirname "$0")
UNINSTALL_FILE="${BASEDIR}/uninstall.sh"
INSTALL_DIR=""
MONITOR_FILE=""
MONITOR_CONFIG=""

STEAM_DECK_PLUGIN="steam-deck-plugin"

# 显示整体安装进度条。参数：<百分比 0-100> <描述>
# 进度输出到 stdout（终端可见）；stderr 已重定向到日志。
progress() {
    pct="$1"
    msg="$2"
    width=30
    filled=$((pct * width / 100))
    bar=""
    i=0
    while [ "$i" -lt "$width" ]; do
        if [ "$i" -lt "$filled" ]; then
            bar="${bar}#"
        else
            bar="${bar}-"
        fi
        i=$((i + 1))
    done
    printf "\r[%s] %3d%% %s\033[K" "$bar" "$pct" "$msg"
    if [ "$pct" -ge 100 ]; then
        printf "\n"
    fi
}

get_steam_deck_install_dir() {
    local sd_install_dir="/home/deck"
    if [ -d ${sd_install_dir} ] ; then
        echo "${sd_install_dir}/uu/"
        return 0
    fi

    sd_install_dir="/home/bazzite"
    if [ -d ${sd_install_dir} ] ; then
        chmod a+xr ${sd_install_dir}
        echo "${sd_install_dir}/uu/"
        return 0
    fi

    sd_install_dir="$(pwd)"
    install_dir_prefix=${str:0:6}
    if [ x"${install_dir_prefix}" = x"/home/" ] ; then
        chmod a+xr ${sd_install_dir}
        echo "${sd_install_dir}/uu/"
        return 0
    fi

    sd_install_dir="/home/uu"
    chmod a+xr ${sd_install_dir}
    echo "${sd_install_dir}/uu/"
    return 0
}

init_param() {
    local router="${ROUTER}"
    local monitor_filename="uuplugin_monitor.sh"

    case "${router}" in
    ${STEAM_DECK_PLUGIN})
        URL_PREFIX="https://"
        INSTALL_DIR=$(get_steam_deck_install_dir)
        MONITOR_FILE="${INSTALL_DIR}/${monitor_filename}"
        MONITOR_CONFIG="${INSTALL_DIR}/uuplugin_monitor.config"
        UNINSTALL_DOWNLOAD_URL="${URL_PREFIX}${UNINSTALL_DOWNLOAD_URL}${STEAM_DECK_PLUGIN}"
        MONITOR_DOWNLOAD_URL="${URL_PREFIX}${MONITOR_DOWNLOAD_URL}${STEAM_DECK_PLUGIN}"
        return 0
        ;;
    *)
        return 1
        ;;
    esac
}

# Return: 0 means success.
check_dir() {
    if [ ! -d "${INSTALL_DIR}" ];then
        mkdir -p "${INSTALL_DIR}"
        [ "$?" != "0" ] && return 1
    fi

    return 0
}

clean_up() {
    [ ! -f "${UNINSTALL_FILE}" ] && return 1

    chmod u+x "${UNINSTALL_FILE}"
    /bin/sh "${UNINSTALL_FILE}" "${ROUTER}" "${MODEL}" 1>/dev/null 2>&1
    [ "$?" != "0" ] && return 1

    return 0
}

# Return: 0 means success.
download() {
    local url="$1"
    local file="$2"
    local plugin_info=$(curl -s -H "Accept:text/plain" "${url}" || \
        wget -q -O - "${url}&output=text"
    )

    [ "$?" != "0" ] && return 1
    [ -z "$plugin_info" ] && return 1

    local plugin_url=$(echo "$plugin_info" | cut  -d ',' -f 1)
    local plugin_md5=$(echo "$plugin_info" | cut  -d ',' -f 2)

    [ -z "${plugin_url}" ] && return 1
    [ -z "${plugin_md5}" ] && return 1

    curl -s "$plugin_url" -o "${file}" >/dev/null 2>&1 || \
        wget -q "$plugin_url" -O "${file}" >/dev/null 2>&1

    if [ "$?" != "0" ];then
        [ -f "${file}" ] && rm "${file}"
        return 1
    fi

    if [ -f "${file}" ];then
        local download_md5=$(md5sum "${file}")
        local download_md5=$(echo "$download_md5" | sed 's/[ ][ ]*/ /g' | cut -d' ' -f1)
        if [ "$download_md5" != "$plugin_md5" ];then
            rm "${file}"
            return 1
        fi
        return 0
    else
        return 1
    fi
}

# Return: 0 means success.
start_monitor() {
    [ ! -f  "${MONITOR_FILE}" ] && return 1

    {
        echo "router=${ROUTER}";
        echo "model=${MODEL}"
    } > ${MONITOR_CONFIG}

    [ "$?" != "0" ] && return 1

    chmod u+x "${MONITOR_FILE}"
    /bin/sh "${MONITOR_FILE}" 1>/dev/null 2>&1 &
    return 0
}

# Return: 0 means running.
check_running() {
    local PID_FILE="/var/run/uuplugin.pid"
    local PLUGIN_EXE="uuplugin"
    for i in {1..90};do
        if [ -f "$PID_FILE" ];then
            local pid=$(cat $PID_FILE)
            local running_pid=$(ps | sed 's/^[ \t]*//g;s/[ \t]*$//g' | \
                sed 's/[ ][ ]*/#/g' | grep "${PLUGIN_EXE}" | \
                grep -v "grep" | cut -d'#' -f1 | grep -e "^${pid}$")

            if [ "${running_pid}" = "" ];then
                running_pid=$(ps -ax -o pid,cmd | sed 's/^[ \t]*//g;s/[ \t]*$//g' | \
                    sed 's/[ ][ ]*/#/g' | grep "${PLUGIN_EXE}" | \
                    grep -v "grep" | cut -d'#' -f1 | grep -e "^${pid}$")
            fi

            if [ "$pid" = "${running_pid}" ];then
                return 0
            else
                sleep 1
            fi
        else
            sleep 1
        fi
    done

    return 1
}

# Return: 0 means success.
config_steam_deck_bootup() {
    config_steam_deck_systemd
    return $?
}

config_steam_deck_systemd() {
    local uuplugin_service="/etc/systemd/system/uuplugin.service"

    {
        echo "[Unit]"
        echo "Description=UU Plugin"
        echo "Wants=network-online.target"
        echo "After=network.target network-online.target"
        echo ""
        echo "[Service]"
        echo "ExecStart=/bin/sh ${INSTALL_DIR}uuplugin_monitor.sh"
        echo ""
        echo "[Install]"
        echo "WantedBy=default.target"
    } > "${uuplugin_service}"

    systemctl daemon-reload
    systemctl enable uuplugin
    systemctl start uuplugin
}

# Return: 0 means success.
config_bootup() {
    local router="${ROUTER}"
    case "${router}" in
    ${STEAM_DECK_PLUGIN})
        config_steam_deck_bootup
        return $?
        ;;
    *)
        return 1
        ;;
    esac
}

# Return: 0 means success.
config_router() {
    local router="${ROUTER}"
    case "${router}" in
    ${STEAM_DECK_PLUGIN})
        return 0
        ;;
    *)
        return 1
        ;;
    esac
}

print_sn() {
    local interface=""
    case "${ROUTER}" in
        *)
            return 1
            ;;
    esac

    local info=$(ip addr show ${interface})
    local mac=$(echo "${info}" | grep "link/ether" | awk '{print $2}')
    echo "sn=${mac}"
    return 0
}

create_uninstall() {
    local unfile=${INSTALL_DIR}/uninstall.sh
    if [ -f "${UNINSTALL_FILE}" ];then
        cp -p ${UNINSTALL_FILE} ${unfile}
        sed -i 's/^ROUTER=\${1:-asuswrt-merlin}$/ROUTER=\${1:-steam-deck-plugin}/g' ${unfile}
    else
        echo "uninstall file:${UNINSTALL_FILE} not exist"
    fi
    return 0
}

install() {
    progress 0 "开始安装..."

    progress 10 "初始化参数..."
    init_param
    [ "$?" != "0" ] && return 9

    progress 20 "检查系统参数..."
    config_router
    [ "$?" != "0" ] && return 1

    progress 30 "检查安装目录..."
    check_dir
    [ "$?" != "0" ] && return 2 

    progress 45 "下载卸载脚本..."
    download "${UNINSTALL_DOWNLOAD_URL}" "${UNINSTALL_FILE}"
    [ "$?" != "0" ] && return 3

    progress 55 "清理旧版本..."
    clean_up
    [ "$?" != "0" ] && return 4

    progress 70 "下载守护程序..."
    download "${MONITOR_DOWNLOAD_URL}" "${MONITOR_FILE}"
    if [ "$?" != "0" ];then
        [ -f "${MONITOR_FILE}" ] && rm "${MONITOR_FILE}"
        return 5
    fi
    chmod a+x ${MONITOR_FILE}

    if [ "${ROUTER}" = "${STEAM_DECK_PLUGIN}" ];then
        {
            echo "router=${ROUTER}";
            echo "model=x86_64"
        } > ${MONITOR_CONFIG}
        progress 85 "配置开机启动..."
        config_bootup

        progress 95 "启动插件..."
        check_running
        create_uninstall
        if [ "$?" != "0" ];then
            #echo "Installation failed!"
            return 6
        fi

        progress 100 "安装完成..."
        return 0
    fi

    progress 85 "启动守护程序..."
    start_monitor
    [ "$?" != "0" ] && return 6

    progress 95 "等待插件运行..."
    check_running
    [ "$?" != "0" ] && return 7

    config_bootup
    [ "$?" != "0" ] && return 8

    print_sn
    [ "$?" != "0" ] && return 10

    progress 100 "安装完成..."
    return 0
}

# Start to install.
QRFILE="/tmp/uu/.steam_deck_sn_qrcode"
[ -f "${QRFILE}" ] && rm "${QRFILE}"

install
status_code=$?

if [ "${status_code}" -eq "0" ];then
    #watch -t -n1 cat /tmp/uu/.steam_deck_sn_qrcode < /dev/tty
    i=0
    while [ "$i" -lt 30 ]; do
        echo "插件安装成功，正在加载..."
        if [ -f "${QRFILE}" ]; then
            break
        fi
        i=$((i + 1))
        sleep 1
    done

    if [ -f ${QRFILE} ] ; then
        printf '\033[2J'
        while [ -f "${QRFILE}" ]; do
            {
                printf '\033[H'
                echo "插件安装成功。请使用UU主机加速器App扫码并完成绑定。"
                echo "绑定成功后，可关闭 Kconsole (终端) 。"
                cat ${QRFILE}
                printf '\033[J'
            }
            sleep 1
        done
        [ -f "${UNINSTALL_FILE}" ] && rm "${UNINSTALL_FILE}"
        exit 0
    else
        echo "插件安装成功。请使用UU主机加速器App进行局域网绑定。"
        [ -f "${UNINSTALL_FILE}" ] && rm "${UNINSTALL_FILE}"
        exit 0
    fi
fi

if [ ${status_code} -gt 4 ];then
    if [ -f "${UNINSTALL_FILE}" ];then
        echo "安装失败，正在清理已下载文件..."
        clean_up
    fi
fi

[ -f "${UNINSTALL_FILE}" ] && rm "${UNINSTALL_FILE}"
exit $status_code
