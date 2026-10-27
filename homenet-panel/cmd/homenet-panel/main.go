package main

import (
	"bufio"
	crand "crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"homenet-panel/internal/api"
	"homenet-panel/internal/audit"
	"homenet-panel/internal/auth"
	"homenet-panel/internal/configtx"
	"homenet-panel/internal/dae"
	"homenet-panel/internal/preflight"
	"homenet-panel/internal/runtimecheck"
	"homenet-panel/internal/subs"
)

const version = "0.2.0-p0"

func usage() {
	fmt.Print("homenet-panel " + version + " (P0) - dae 配置事务核心 + 内容层\n\n" +
		"用法: homenet-panel <命令> [参数]\n\n" +
		"命令:\n" +
		"  detect       检测 dae 版本\n" +
		"  runtimecheck 内核/BPF/memlock 检测\n" +
		"  status       运行状态(pid/服务/进度)\n" +
		"  preflight    双写检测 + 订阅预检   -c <path>\n" +
		"  validate     校验配置         -c <path>\n" +
		"  apply        应用新配置(预检+备份+校验+reload+失败回滚) --target <path> --from <newfile> [--allow-goose]\n" +
		"  rollback     回滚到最近备份   --target <path>\n" +
		"  outline      导出 outline\n" +
		"  suspend      应急 suspend\n" +
		"  setpass      设置登录账号密码  --user <name> --password <pass> [--auth-path <file>]\n" +
		"  serve        启动 REST API + Web 控制台  --addr 127.0.0.1:8787 [--token]\n")
}

func mustJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func makePreflight(allowGoose bool) func(path string, content []byte) error {
	return func(path string, content []byte) error {
		r := preflight.Detect(path, content)
		reasons := r.Reasons()
		if r.Blocking() {
			if allowGoose && len(r.GooseRunning) == 0 && len(r.OtherWriters) == 0 && len(r.SubErrors) == 0 {
				return nil
			}
			return fmt.Errorf("预检阻断: %v", reasons)
		}
		return nil
	}
}

func randPass(n int) string {
	const chars = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	rb := make([]byte, n)
	_, _ = crand.Read(rb)
	for i := range b {
		b[i] = chars[int(rb[i])%len(chars)]
	}
	return string(b)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	a := dae.New()
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfg := fs.String("c", dae.DefaultConfig, "dae 配置文件路径")
	target := fs.String("target", dae.DefaultConfig, "目标配置文件路径")
	from := fs.String("from", "", "新配置内容文件")
	backupDir := fs.String("backup-dir", "/var/lib/homenet-panel/backups", "备份目录")
	allowGoose := fs.Bool("allow-goose", false, "允许覆盖 goose 生成的配置(仅豁免双写头，不豁免活进程)")
	addr := fs.String("addr", "127.0.0.1:8787", "serve 监听地址")
	token := fs.String("token", "", "serve 静态鉴权 token(可选，留空则仅用账号密码登录)")
	tlsCert := fs.String("tls-cert", "", "serve HTTPS 证书路径(与 --tls-key 同时提供则启用 TLS)")
	tlsKey := fs.String("tls-key", "", "serve HTTPS 私钥路径")
	trafficIface := fs.String("traffic-iface", "", "serve 流量统计旁路抓包网卡(如 ens2；空=关闭)")
	authPath := fs.String("auth-path", auth.DefaultPath, "账号密码凭证文件")
	auditPath := fs.String("audit-path", "/var/lib/homenet-panel/audit.log", "审计日志(JSONL)路径")
	subsrcPath := fs.String("subsrc-path", "/var/lib/homenet-panel/subsrc.json", "订阅源登记表路径")
	subsrcHours := fs.Int("subsrc-check-hours", 6, "订阅新鲜度巡检周期(小时，0=关闭)")
	subUA := fs.String("sub-ua", "", "拉取订阅用的默认 User-Agent(空=Go 默认；部分机场只认特定 UA)")
	bytesInterval := fs.Int("bytes-interval", 10, "serve 字节级流量统计采样周期(秒，0=关闭)")
	bytesIface := fs.String("bytes-iface", "dae0", "serve 字节统计读哪块网卡的收发总量(dae 的拦截侧设备)")
	auditMaxMB := fs.Int("audit-max-mb", 5, "审计日志单文件上限(MB)，超过后轮转")
	auditKeep := fs.Int("audit-keep", 3, "审计日志保留几份历史(路由器建议 2)")
	sessionPath := fs.String("session-path", "/var/lib/homenet-panel/sessions.json", "登录会话持久化文件(重启后不用重新登录；空=仅内存)")
	geoDir := fs.String("geo-dir", "/usr/share/dae", "dae 的 geo 数据目录（geosite.dat/geoip.dat 所在处）")
	geoKeep := fs.Int("geo-keep", 3, "geo 数据更新时保留几份历史备份")
	allowPlain := fs.Bool("allow-plain-http", false, "TLS 开启时也允许明文 HTTP 访问(内网便利；默认只做 301 跳转到 https)")
	userF := fs.String("user", "admin", "setpass 登录用户名")
	passF := fs.String("password", "", "setpass 登录密码(留空则自动生成)")
	mustChange := fs.Bool("must-change", false, "setpass 标记为初始密码：下次登录必须先改密码")
	_ = fs.Parse(os.Args[2:])

	if *token == "" {
		*token = os.Getenv("HOMENET_PANEL_TOKEN")
	}
	subs.SetUserAgent(*subUA)

	switch cmd {
	case "detect":
		out, err := a.Detect()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(out)

	case "runtimecheck":
		mustJSON(runtimecheck.Run())

	case "status":
		pid := a.RunningPID()
		prog, perr := a.Progress()
		st := map[string]any{"pid": pid, "service": a.ServiceActive(), "configPath": a.ConfigPath}
		if perr == nil {
			st["progressCode"] = prog.Describe()
			st["progressMsg"] = prog.Message
		} else {
			st["progressErr"] = perr.Error()
		}
		mustJSON(st)

	case "preflight":
		content, err := os.ReadFile(*cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取配置失败:", err)
			os.Exit(1)
		}
		r := preflight.Detect(*cfg, content)
		mustJSON(map[string]any{"report": r, "blocking": r.Blocking(), "reasons": r.Reasons()})
		if r.Blocking() {
			os.Exit(3)
		}

	case "validate":
		vr := a.Validate(*cfg)
		mustJSON(map[string]any{"ok": vr.OK, "exitCode": vr.ExitCode, "output": vr.Output})
		if !vr.OK {
			os.Exit(1)
		}

	case "apply":
		if *from == "" {
			fmt.Fprintln(os.Stderr, "apply 需要 --from <newfile>")
			os.Exit(2)
		}
		data, err := os.ReadFile(*from)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取新配置失败:", err)
			os.Exit(1)
		}
		m := configtx.New(a, *backupDir)
		m.Preflight = makePreflight(*allowGoose)
		res, err := m.Apply(configtx.Change{Path: *target, NewContent: data})
		mustJSON(res)
		if err != nil {
			fmt.Fprintln(os.Stderr, "apply 失败:", err)
			os.Exit(1)
		}

	case "rollback":
		m := configtx.New(a, *backupDir)
		res, err := m.Rollback(*target)
		mustJSON(res)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rollback 失败:", err)
			os.Exit(1)
		}

	case "outline":
		out, err := a.Outline()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(out)

	case "suspend":
		code, out := a.Suspend(a.RunningPID())
		mustJSON(map[string]any{"exitCode": code, "output": out})
		if code != 0 {
			os.Exit(1)
		}

	case "setpass":
		pass := *passF
		generated := false
		if pass == "" {
			pass = randPass(16)
			generated = true
		}
		if err := auth.SetWithFlag(*authPath, *userF, pass, *mustChange); err != nil {
			fmt.Fprintln(os.Stderr, "写入凭证失败:", err)
			os.Exit(1)
		}
		out := map[string]any{"ok": true, "user": *userF, "authPath": *authPath, "mustChange": *mustChange}
		if generated {
			out["password"] = pass
		}
		mustJSON(out)
		if *mustChange {
			fmt.Println("已标记为初始密码：下次登录会强制要求修改。")
		}

	case "serve":
		if !auth.Exists(*authPath) {
			pass := randPass(16)
			if err := auth.SetWithFlag(*authPath, "admin", pass, true); err != nil {
				fmt.Fprintln(os.Stderr, "初始化凭证失败:", err)
				os.Exit(1)
			}
			fmt.Println("==================================================================")
			fmt.Println("首次启动：已自动创建登录账号（初始密码，登录后必须修改）")
			fmt.Printf("  用户名: admin\n  密码:   %s\n", pass)
			fmt.Printf("  (已写入 %s，可用 setpass 修改)\n", *authPath)
			fmt.Println("==================================================================")
		}
		tlsOn := *tlsCert != "" && *tlsKey != ""
		srv := api.New(api.Config{
			Dae:          a,
			EntryPath:    *cfg,
			BackupDir:    *backupDir,
			Token:        *token,
			AuthPath:     *authPath,
			TLS:          tlsOn,
			TrafficIface: *trafficIface,
			AuditPath:    *auditPath,
			SubsrcPath:   *subsrcPath,
			SessionPath:  *sessionPath,
			CertPath:     *tlsCert,
			GeoPath:      filepath.Join(*geoDir, "geosite.dat"),
			GeoDir:       *geoDir,
			GeoKeep:      *geoKeep,
		})
		mux := http.NewServeMux()
		srv.Register(mux)
		audit.SetRetention(int64(*auditMaxMB)<<20, *auditKeep)
		fmt.Printf("审计日志: %s\n", audit.Describe())
		srv.RestoreAccessTables()
		if *subsrcHours > 0 {
			srv.SetCheckHours(*subsrcHours)
			srv.StartBackground()
			fmt.Printf("订阅巡检: 每 %d 小时检查一次上游（只报告，不自动改配置）\n", *subsrcHours)
		}
		if *bytesInterval > 0 {
			srv.StartBytes(time.Duration(*bytesInterval)*time.Second, *bytesIface)
			fmt.Printf("字节统计: 每 %d 秒采样一次（dae 的 socket 计数器 + %s 网卡总量）\n", *bytesInterval, *bytesIface)
		} else {
			fmt.Println("字节统计: 未启用 (--bytes-interval 0)")
		}
		scheme := "http"
		if tlsOn {
			scheme = "https"
		}
		fmt.Printf("HomeNet Panel 控制台启动: %s://%s\n", scheme, *addr)
		if *trafficIface != "" {
			fmt.Printf("流量统计: DNS 旁路抓包 @ %s\n", *trafficIface)
		} else {
			fmt.Println("流量统计: 未启用 (--traffic-iface 关闭)")
		}
		if *token != "" {
			fmt.Println("鉴权: 账号密码登录 + 静态 Bearer token(自动化)")
		} else {
			fmt.Println("鉴权: 账号密码登录")
		}
		var err error
		if tlsOn {
			if *allowPlain {
				fmt.Println("TLS: 已启用，且允许同端口明文 HTTP 访问（--allow-plain-http，密码会明文过网）")
			} else {
				fmt.Println("TLS: 已启用；明文请求自动 301 跳到 https")
			}
			err = serveDual(*addr, mux, *tlsCert, *tlsKey, *allowPlain)
		} else {
			httpSrv := &http.Server{Addr: *addr, Handler: mux}
			err = httpSrv.ListenAndServe()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "服务启动失败:", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintln(os.Stderr, "未知命令:", cmd)
		usage()
		os.Exit(2)
	}
}

var _ = errors.New

// ---- 同端口 TLS / 明文 HTTP 分流 ----

// connChan 把连接通道包装成 net.Listener。
type connChan struct {
	ch   chan net.Conn
	addr net.Addr
}

func (c *connChan) Accept() (net.Conn, error) {
	nc, ok := <-c.ch
	if !ok {
		return nil, net.ErrClosed
	}
	return nc, nil
}

func (c *connChan) Close() error   { return nil }
func (c *connChan) Addr() net.Addr { return c.addr }

// bufferedConn 保留被 Peek 过的那一个字节。
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }

// splitConns 读首字节判断协议：0x16 是 TLS 握手，其余当明文 HTTP。
func splitConns(ln net.Listener, tlsCh, plainCh *connChan) {
	for {
		c, err := ln.Accept()
		if err != nil {
			close(tlsCh.ch)
			close(plainCh.ch)
			return
		}
		go func(c net.Conn) {
			br := bufio.NewReader(c)
			_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
			b, err := br.Peek(1)
			_ = c.SetReadDeadline(time.Time{})
			if err != nil {
				_ = c.Close()
				return
			}
			bc := &bufferedConn{Conn: c, r: br}
			if b[0] == 0x16 {
				tlsCh.ch <- bc
			} else {
				plainCh.ch <- bc
			}
		}(c)
	}
}

// serveDual 在同一个端口上同时接受 TLS 与明文 HTTP：明文默认 301 跳到 https。
func serveDual(addr string, mux http.Handler, certFile, keyFile string, allowPlain bool) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	base, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(base.Addr().String())
	if err != nil {
		return err
	}
	tlsL := &connChan{ch: make(chan net.Conn, 64), addr: base.Addr()}
	plainL := &connChan{ch: make(chan net.Conn, 64), addr: base.Addr()}
	go splitConns(base, tlsL, plainL)

	plainHandler := mux
	if !allowPlain {
		plainHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if h, _, e := net.SplitHostPort(host); e == nil {
				host = h
			}
			if strings.HasPrefix(host, "[") && !strings.HasSuffix(host, "]") {
				host = "[" + host + "]"
			}
			u := "https://" + net.JoinHostPort(strings.Trim(host, "[]"), port) + r.URL.RequestURI()
			http.Redirect(w, r, u, http.StatusMovedPermanently)
		})
	}
	plainSrv := &http.Server{Handler: plainHandler}
	go func() { _ = plainSrv.Serve(plainL) }()

	tlsSrv := &http.Server{
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	return tlsSrv.Serve(tls.NewListener(tlsL, tlsSrv.TLSConfig))
}
