package router

const (
	ASUSWRTMerlin = "asuswrt-merlin"
	Xiaomi        = "xiaomi"
	HiWiFi        = "hiwifi"
	OpenWRT       = "openwrt"
	SteamDeck     = "steam-deck-plugin"
	Default       = SteamDeck
	DefaultModel  = "x86_64"
)

var supported = map[string]struct{}{
	ASUSWRTMerlin: {},
	Xiaomi:        {},
	HiWiFi:        {},
	OpenWRT:       {},
	SteamDeck:     {},
}

func Supported(name string) bool {
	_, ok := supported[name]
	return ok
}
