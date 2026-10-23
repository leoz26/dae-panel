//go:build !linux

package traffic

import (
	"errors"
	"runtime"
)

// NewDNSSource 在非 Linux 平台不可用（AF_PACKET 仅 Linux 支持）。
func NewDNSSource(iface string) (Source, error) {
	return nil, errors.New("DNS 旁路抓包仅支持 linux（当前 " + runtime.GOOS + "）")
}
