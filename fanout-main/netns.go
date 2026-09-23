package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

// netnsResolver 使用公共 DNS，防止在 netns 内读取母机 127.0.0.53 导致解析超时。
var netnsResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 3 * time.Second}
		conn, err := d.DialContext(ctx, "udp4", "8.8.8.8:53")
		if err != nil {
			conn, err = d.DialContext(ctx, "udp4", "1.1.1.1:53")
		}
		return conn, err
	},
}

// resolveTargetAddress 预先在母机网络环境中将目标地址解析为 IPv4。
// 若地址已为 IP:port 则直接返回；若是 domain:port，则优先使用母机系统 DNS 高速解析，
// 失败时回退到公共 DNS (8.8.8.8 / 1.1.1.1)。
// 这样在进入 netns 拨号时绝不会因为 netns 内部无法访问母机 127.0.0.53 而发生 10 秒超时卡死！
func resolveTargetAddress(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if net.ParseIP(host) != nil {
		return addr
	}

	// 1. 优先使用母机系统 DNS（在母机主线程环境中只需 1~2ms 即可瞬间解析）
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	cancel()
	if err == nil && len(ips) > 0 {
		return net.JoinHostPort(ips[0].String(), port)
	}

	// 2. 回退到公共 DNS (8.8.8.8 / 1.1.1.1)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	ips, err = netnsResolver.LookupIP(ctx2, "ip4", host)
	cancel2()
	if err == nil && len(ips) > 0 {
		return net.JoinHostPort(ips[0].String(), port)
	}

	return addr
}

// dialerInNetns 返回一个在指定 netns 内建立出站连接的 dial 函数。
// 每次拨号在独立锁定的 OS 线程中切换 netns，拨号成功后安全恢复或交由 runtime 销毁。
func dialerInNetns(nsName string) func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		resolvedAddr := resolveTargetAddress(addr)

		type result struct {
			conn net.Conn
			err  error
		}
		ch := make(chan result, 1)

		go func() {
			runtime.LockOSThread()

			tid := unix.Gettid()
			origin, err := os.Open(fmt.Sprintf("/proc/self/task/%d/ns/net", tid))
			if err != nil {
				origin, err = os.Open("/proc/thread-self/ns/net")
			}
			if err != nil {
				origin, err = os.Open("/proc/self/ns/net")
			}

			target, err := os.Open("/var/run/netns/" + nsName)
			if err != nil {
				if origin != nil {
					origin.Close()
				}
				runtime.UnlockOSThread()
				ch <- result{nil, err}
				return
			}
			defer target.Close()

			if err := unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
				if origin != nil {
					origin.Close()
				}
				runtime.UnlockOSThread()
				ch <- result{nil, err}
				return
			}

			d := net.Dialer{
				Timeout: 12 * time.Second,
			}
			conn, dialErr := d.Dial(forceIPv4Network(network), resolvedAddr)

			restored := false
			if origin != nil {
				if err := unix.Setns(int(origin.Fd()), unix.CLONE_NEWNET); err == nil {
					restored = true
				}
				origin.Close()
			}

			if restored {
				runtime.UnlockOSThread()
			}
			// 若恢复失败，不调用 UnlockOSThread，当前 OS 线程随着 goroutine 结束由 Go runtime 彻底销毁，绝不污染线程池。

			ch <- result{conn, dialErr}
		}()

		r := <-ch
		return r.conn, r.err
	}
}

// forceIPv4Network 把 tcp/udp 收敛成 tcp4/udp4，已经指定版本的原样返回。
func forceIPv4Network(network string) string {
	switch network {
	case "tcp":
		return "tcp4"
	case "udp":
		return "udp4"
	}
	return network
}
