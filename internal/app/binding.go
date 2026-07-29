package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	steamDeckQRCodeName    = ".steam_deck_sn_qrcode"
	steamDeckQRCodeRetries = 30
)

func (i *App) showSteamDeckBindingQRCode(sleep func(time.Duration)) {
	qrFile := filepath.Join(i.steamDeckRuntimeDir(), steamDeckQRCodeName)

	for attempt := 0; attempt < steamDeckQRCodeRetries; attempt++ {
		fmt.Fprintln(i.stdout, "插件安装成功，正在加载...")
		if fileExists(qrFile) {
			break
		}
		sleep(time.Second)
	}

	if !fileExists(qrFile) {
		fmt.Fprintln(i.stdout, "插件安装成功。请使用UU主机加速器App进行局域网绑定。")
		return
	}

	fmt.Fprint(i.stdout, "\033[2J")
	for {
		qrCode, err := os.ReadFile(qrFile)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			i.logf("read binding QR code failed: %v", err)
			fmt.Fprintln(i.stdout, "插件安装成功。请使用UU主机加速器App进行局域网绑定。")
			return
		}

		fmt.Fprint(i.stdout, "\033[H")
		fmt.Fprintln(i.stdout, "插件安装成功。请使用UU主机加速器App扫码并完成绑定。")
		fmt.Fprintln(i.stdout, "绑定成功后，可关闭终端。")
		fmt.Fprint(i.stdout, string(qrCode))
		fmt.Fprint(i.stdout, "\033[J")
		sleep(time.Second)
	}
}
