package subs

import (
	"errors"
	"net"
	"time"
)

// DialLatency 测量到节点 host:port 的 TCP 建连延迟（毫秒）。仅代表端点可达性/握手延迟，不等于真实代理吞吐。
func DialLatency(host, port string, timeout time.Duration) (int64, error) {
	if host == "" || port == "" {
		return 0, errors.New("缺少 host/port")
	}
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return time.Since(start).Milliseconds(), nil
}
