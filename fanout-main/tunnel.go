package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SocksCred 是一条隧道的 SOCKS5 访问凭据。
//
// 每条隧道一套独立凭据：泄露一条不会连累其他出口，
// 换节点时也能只重置这一条而不影响已分发的其他配置。
type SocksCred struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

// Tunnel 是一条运行中的隧道：一个 netns + 一个 openvpn 进程 + 一个本地 SOCKS5 端口。
type Tunnel struct {
	Slot         int       `json:"slot"`
	Port         int       `json:"port"`
	Node         Node      `json:"node"`
	TargetRegion string    `json:"target_region,omitempty"` // 锁定目标国家代码，故障时优先重连同国节点
	Status       string    `json:"status"`                  // starting | up | failed | stopped
	ExitIP       string    `json:"exit_ip"`
	Err          string    `json:"err,omitempty"`
	Since        time.Time `json:"since"`
	Cred         SocksCred `json:"cred"`

	ns       string
	listener net.Listener
	ovpn     *exec.Cmd
	dialer   func(network, addr string) (net.Conn, error)
	mu       sync.Mutex
}

func (t *Tunnel) nsName() string { return fmt.Sprintf("fo%d", t.Slot) }
func (t *Tunnel) subnet() string { return fmt.Sprintf("10.99.%d", t.Slot) }

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runQuiet 执行清理类命令，忽略"本来就不存在"这类错误。
func runQuiet(name string, args ...string) {
	_ = exec.Command(name, args...).Run()
}

// setupNetns 建立 netns 与 veth 链路，并配好 NAT 与转发放行。
func (t *Tunnel) setupNetns() error {
	ns, sub := t.nsName(), t.subnet()
	veth, peer := fmt.Sprintf("fov%d", t.Slot), fmt.Sprintf("fop%d", t.Slot)

	t.teardownNetns()

	if err := run("ip", "netns", "add", ns); err != nil {
		return err
	}
	if err := run("ip", "netns", "exec", ns, "ip", "link", "set", "lo", "up"); err != nil {
		return err
	}
	if err := run("ip", "link", "add", veth, "type", "veth", "peer", "name", peer); err != nil {
		return err
	}
	if err := run("ip", "link", "set", peer, "netns", ns); err != nil {
		return err
	}
	if err := run("ip", "addr", "add", sub+".1/30", "dev", veth); err != nil {
		return err
	}
	if err := run("ip", "link", "set", veth, "up"); err != nil {
		return err
	}
	if err := run("ip", "netns", "exec", ns, "ip", "addr", "add", sub+".2/30", "dev", peer); err != nil {
		return err
	}
	if err := run("ip", "netns", "exec", ns, "ip", "link", "set", peer, "up"); err != nil {
		return err
	}
	if err := run("ip", "netns", "exec", ns, "ip", "route", "add", "default", "via", sub+".1"); err != nil {
		return err
	}

	// netns 内的 DNS，仅用于 openvpn 解析远端主机名
	nsDir := filepath.Join("/etc/netns", ns)
	if err := os.MkdirAll(nsDir, 0755); err != nil {
		return fmt.Errorf("创建 %s 失败: %w", nsDir, err)
	}
	if err := os.WriteFile(filepath.Join(nsDir, "resolv.conf"), []byte("nameserver 8.8.8.8\n"), 0644); err != nil {
		return fmt.Errorf("写 resolv.conf 失败: %w", err)
	}

	ensureGlobalSubnetRules()
	runQuiet("ip", "netns", "exec", ns, "iptables", "-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	runQuiet("ip", "netns", "exec", ns, "iptables", "-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	return nil
}

var iptablesOnce sync.Once

// ensureGlobalSubnetRules 一次性对整个 10.99.0.0/16 网段配置 NAT 与转发，避免频繁加锁阻塞 iptables
func ensureGlobalSubnetRules() {
	iptablesOnce.Do(func() {
		ensureRule("nat", "POSTROUTING", "-s", "10.99.0.0/16", "-j", "MASQUERADE")
		ensureRuleInsert("filter", "FORWARD", "-s", "10.99.0.0/16", "-j", "ACCEPT")
		ensureRuleInsert("filter", "FORWARD", "-d", "10.99.0.0/16", "-j", "ACCEPT")
		ensureRule("mangle", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	})
}

// ensureRule 幂等追加一条 iptables 规则。
func ensureRule(table, chain string, spec ...string) {
	check := append([]string{"-w", "5", "-t", table, "-C", chain}, spec...)
	if exec.Command("iptables", check...).Run() == nil {
		return
	}
	add := append([]string{"-w", "5", "-t", table, "-A", chain}, spec...)
	runQuiet("iptables", add...)
}

// ensureRuleInsert 幂等插入规则到链首。
// FORWARD 链末尾常有兜底 REJECT，必须插到最前面才生效。
func ensureRuleInsert(table, chain string, spec ...string) {
	check := append([]string{"-w", "5", "-t", table, "-C", chain}, spec...)
	if exec.Command("iptables", check...).Run() == nil {
		return
	}
	ins := append([]string{"-w", "5", "-t", table, "-I", chain, "1"}, spec...)
	runQuiet("iptables", ins...)
}

func (t *Tunnel) teardownNetns() {
	ns := t.nsName()
	runQuiet("ip", "netns", "del", ns)
	runQuiet("ip", "link", "del", fmt.Sprintf("fov%d", t.Slot))
}

// startOpenVPN 在 netns 内拉起 openvpn，并等待 tun0 拿到地址。
func (t *Tunnel) startOpenVPN(dir string) error {
	ns := t.nsName()
	cfgPath := filepath.Join(dir, ns+".ovpn")
	if err := os.WriteFile(cfgPath, []byte(t.Node.Config), 0600); err != nil {
		return fmt.Errorf("写配置失败: %w", err)
	}
	authPath := filepath.Join(dir, "auth.txt")
	if err := os.WriteFile(authPath, []byte("vpn\nvpn\n"), 0600); err != nil {
		return fmt.Errorf("写凭据失败: %w", err)
	}

	logPath := filepath.Join(dir, ns+".log")
	_ = os.Remove(logPath)
	cmd := exec.Command("ip", "netns", "exec", ns, "openvpn",
		"--config", cfgPath,
		"--auth-user-pass", authPath,
		"--auth-nocache",
		"--dev", "tun0",
		"--connect-retry-max", "1",
		"--connect-timeout", "12",
		"--data-ciphers", "AES-128-CBC:AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305",
		"--verb", "1",
		"--log", logPath,
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 openvpn 失败: %w", err)
	}
	t.ovpn = cmd
	go cmd.Wait() // 回收子进程，避免僵尸

	// openvpn 建好 tun0 前 SOCKS5 无法正常出网，这里等它就绪
	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		if out, err := exec.Command("ip", "netns", "exec", ns, "ip", "-4", "addr", "show", "tun0").Output(); err == nil {
			if strings.Contains(string(out), "inet ") {
				return nil
			}
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return fmt.Errorf("openvpn 提前退出，详见 %s", logPath)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("等待 tun0 就绪超时，详见 %s", logPath)
}

// serve 在母机上监听 SOCKS5 端口，出站连接则在 netns 内建立。
// 监听必须留在母机侧：netns 内的 loopback 与母机彼此独立，
// 监听在 netns 里的话外部根本连不上。
func (t *Tunnel) serve() error {
	// 端口要尽量保持不变，否则用户已经分发出去的客户端配置会失效。
	// 进程刚重启时旧监听可能还在 TIME_WAIT，这里给几秒重试窗口。
	var ln net.Listener
	var err error
	for i := 0; i < 6; i++ {
		ln, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", t.Port))
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		// 确实被别的进程长期占用了，才换端口
		port, perr := freeRandomPort(map[int]bool{t.Port: true})
		if perr != nil {
			return fmt.Errorf("监听 %d 失败且无备用端口: %w", t.Port, err)
		}
		ln, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			return fmt.Errorf("监听 %d 失败: %w", port, err)
		}
		t.Port = port
	}
	t.listener = ln
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// 每次连接现取凭据与拨号器：换节点或改口令后不必重建监听，新连接立刻按新配置生效
			cred := t.credential()
			dial := t.getDialer()
			go serveSocks(conn, &cred, dial)
		}
	}()
	return nil
}

// getDialer 动态获取当前有效的出站拨号器
func (t *Tunnel) getDialer() func(network, addr string) (net.Conn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dialer != nil {
		return t.dialer
	}
	return dialerInNetns(t.nsName())
}

// credential 取一份凭据副本，避免读写并发。
func (t *Tunnel) credential() SocksCred {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Cred
}

// setCredential 换掉这条隧道的 SOCKS5 凭据。已建立的连接不受影响，
// 新连接立即按新凭据校验。
func (t *Tunnel) setCredential(c SocksCred) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Cred = c
}

// probeExitIP 通过隧道真实发起 HTTP 请求检测公网连通性并提取出口 IP。
// 必须确保真正能出网才返回成功，探测失败时报错以触发管理器切换下一个候选节点。
func (t *Tunnel) probeExitIP() (string, error) {
	// 优先检测 OpenVPN netns 隧道
	if t.Node.Config != "" {
		// 1. 在 netns 内执行 curl（独立进程，直接走 tun0，抗干扰）
		out, err := exec.Command("ip", "netns", "exec", t.nsName(),
			"curl", "-s", "--max-time", "8", "http://1.1.1.1/cdn-cgi/trace").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "ip=") {
					ip := strings.TrimSpace(strings.TrimPrefix(line, "ip="))
					if net.ParseIP(ip) != nil {
						return ip, nil
					}
				}
			}
		}

		out2, err2 := exec.Command("ip", "netns", "exec", t.nsName(),
			"curl", "-s", "--max-time", "8", "http://api.ipify.org").Output()
		if err2 == nil {
			ip := strings.TrimSpace(string(out2))
			if net.ParseIP(ip) != nil {
				return ip, nil
			}
		}

		// 严格模式：tun0 起来不等于真的能出网。不能使用节点元数据里的 IP 作为伪成功结果，
		// 否则会把“OpenVPN 进程启动但没有真实出网”的坏节点加入正式出口。
		return "", fmt.Errorf("查询出口 IP 失败 (curl1: %v, curl2: %v)", err, err2)
	}

	// 上游公开代理（SOCKS5 / HTTP 直连模式）
	if t.dialer != nil {
		client := &http.Client{
			Transport: &http.Transport{
				Dial: t.dialer,
			},
			Timeout: 6 * time.Second,
		}

		// 1. 直连 IP 接口
		resp, err := client.Get("http://1.1.1.1/cdn-cgi/trace")
		if err == nil && resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			for _, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(line, "ip=") {
					ip := strings.TrimSpace(strings.TrimPrefix(line, "ip="))
					if net.ParseIP(ip) != nil {
						return ip, nil
					}
				}
			}
		}
		if resp != nil {
			resp.Body.Close()
		}

		// 2. 备用接口：api.ipify.org
		resp2, err2 := client.Get("http://api.ipify.org")
		if err2 == nil && resp2.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()
			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
				return ip, nil
			}
		}
		if resp2 != nil {
			resp2.Body.Close()
		}

		if err != nil && err2 != nil {
			return "", fmt.Errorf("上游代理连通测试失败 (trace: %v, ipify: %v)", err, err2)
		}
		return "", fmt.Errorf("上游代理未返回有效公网 IP (trace: %v, ipify: %v)", err, err2)
	}

	return "", fmt.Errorf("无法确定出口 IP")
}

// stop 停止这条隧道并清理它占用的所有资源。
func (t *Tunnel) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.listener != nil {
		t.listener.Close()
		t.listener = nil
	}
	if t.ovpn != nil && t.ovpn.Process != nil {
		_ = t.ovpn.Process.Kill()
		t.ovpn = nil
	}
	if t.ns != "" || t.Node.Config != "" {
		t.teardownNetns()
	}
	t.dialer = nil
	t.Status = "stopped"
}

type bufferedConn struct {
	net.Conn
	r io.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

// makeUpstreamDialer 为上游公开 SOCKS5 或 HTTP 代理构造出站直连拨号器
func makeUpstreamDialer(proto, upstreamAddr string, timeout time.Duration) func(network, addr string) (net.Conn, error) {
	if proto == "http" {
		return func(network, addr string) (net.Conn, error) {
			conn, err := net.DialTimeout("tcp", upstreamAddr, timeout)
			if err != nil {
				return nil, err
			}
			_ = conn.SetDeadline(time.Now().Add(timeout))

			targetAddr := addr
			h, p, splitErr := net.SplitHostPort(addr)
			if splitErr == nil && net.ParseIP(h) == nil {
				// 尝试解析域名为 IPv4，极大增强公网 HTTP 代理的访问兼容性
				if ips, lErr := net.LookupIP(h); lErr == nil {
					for _, ipItem := range ips {
						if ip4 := ipItem.To4(); ip4 != nil {
							targetAddr = net.JoinHostPort(ip4.String(), p)
							break
						}
					}
				}
			}

			req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Connection: Keep-Alive\r\n\r\n", targetAddr, targetAddr)
			if _, err := conn.Write([]byte(req)); err != nil {
				conn.Close()
				return nil, err
			}
			br := bufio.NewReader(conn)
			statusLine, err := br.ReadString('\n')
			if err != nil {
				conn.Close()
				return nil, err
			}
			if !strings.Contains(statusLine, "200") {
				conn.Close()
				return nil, fmt.Errorf("http proxy connect failed: %s", strings.TrimSpace(statusLine))
			}
			for {
				line, err := br.ReadString('\n')
				if err != nil || strings.TrimSpace(line) == "" {
					break
				}
			}
			_ = conn.SetDeadline(time.Time{})
			if br.Buffered() > 0 {
				return &bufferedConn{Conn: conn, r: io.MultiReader(br, conn)}, nil
			}
			return conn, nil
		}
	}

	// 默认 SOCKS5 上游拨号协议
	return func(network, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout("tcp", upstreamAddr, timeout)
		if err != nil {
			return nil, err
		}
		_ = conn.SetDeadline(time.Now().Add(timeout))

		// 1. 协商无认证
		if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
			conn.Close()
			return nil, err
		}
		resp := make([]byte, 2)
		if _, err := io.ReadFull(conn, resp); err != nil {
			conn.Close()
			return nil, err
		}
		if resp[0] != 0x05 || resp[1] != 0x00 {
			conn.Close()
			return nil, fmt.Errorf("upstream socks5 auth rejected: %v", resp)
		}

		// 2. CONNECT 请求
		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			conn.Close()
			return nil, err
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			conn.Close()
			return nil, err
		}

		// 先本地解析域名为 IPv4，绝大多数公网 SOCKS5 代理仅支持 IPv4 (ATYP 0x01)，不支持域名 (ATYP 0x03)
		var targetIP net.IP
		if ip := net.ParseIP(host); ip != nil {
			targetIP = ip
		} else {
			if ips, err := net.LookupIP(host); err == nil {
				for _, ipItem := range ips {
					if ip4 := ipItem.To4(); ip4 != nil {
						targetIP = ip4
						break
					}
				}
			}
		}

		var buf []byte
		if targetIP != nil && targetIP.To4() != nil {
			buf = append([]byte{0x05, 0x01, 0x00, 0x01}, targetIP.To4()...)
		} else if targetIP != nil && targetIP.To16() != nil {
			buf = append([]byte{0x05, 0x01, 0x00, 0x04}, targetIP.To16()...)
		} else {
			// 无法解析时降级传域名
			buf = append([]byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}, []byte(host)...)
		}
		buf = append(buf, byte(port>>8), byte(port&0xff))
		if _, err := conn.Write(buf); err != nil {
			conn.Close()
			return nil, err
		}

		// 3. 读取应答头 [0x05, rep, 0x00, atyp, ...]
		repHead := make([]byte, 4)
		if _, err := io.ReadFull(conn, repHead); err != nil {
			conn.Close()
			return nil, err
		}
		if repHead[1] != 0x00 {
			conn.Close()
			return nil, fmt.Errorf("upstream socks5 connect rep: %d", repHead[1])
		}
		var skipLen int
		switch repHead[3] {
		case 0x01: // IPv4
			skipLen = 4 + 2
		case 0x04: // IPv6
			skipLen = 16 + 2
		case 0x03: // 域名
			l := make([]byte, 1)
			if _, err := io.ReadFull(conn, l); err != nil {
				conn.Close()
				return nil, err
			}
			skipLen = int(l[0]) + 2
		default:
			conn.Close()
			return nil, fmt.Errorf("unknown atyp in upstream socks5 reply: %d", repHead[3])
		}
		discard := make([]byte, skipLen)
		if _, err := io.ReadFull(conn, discard); err != nil {
			conn.Close()
			return nil, err
		}

		// 握手成功，清空超时，转入全双工流转发
		_ = conn.SetDeadline(time.Time{})
		return conn, nil
	}
}
