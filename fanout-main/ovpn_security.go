package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Remote profiles are data, never privileged instructions. Reject unknown
// options, nested files, scripts, plugins and arbitrary filesystem access.
func validateOpenVPNConfig(config string) error {
	if len(config) == 0 || len(config) > 512<<10 || strings.ContainsRune(config, 0) {
		return fmt.Errorf("OpenVPN 配置为空或过大")
	}
	allowed := "client dev dev-type proto remote remote-random remote-random-hostname resolv-retry nobind persist-key persist-tun cipher data-ciphers data-ciphers-fallback auth auth-nocache auth-user-pass remote-cert-tls verify-x509-name ns-cert-type tls-client tls-version-min tls-version-max tls-cipher tls-ciphersuites key-direction comp-lzo compress allow-compression tun-mtu link-mtu mssfix fragment explicit-exit-notify reneg-sec keepalive ping ping-restart ping-timer-rem connect-retry connect-retry-max connect-timeout server-poll-timeout verb mute route redirect-gateway route-delay route-metric route-nopull pull pull-filter sndbuf rcvbuf fast-io float replay-window socket-flags setenv opt tls-timeout tran-window hand-window mute-replay-warnings topology ifconfig ifconfig-nowarn ifconfig-noexec route-noexec auth-retry tls-exit remote-cert-ku remote-cert-eku tls-cert-profile auth-token"
	opts := map[string]bool{}
	for _, k := range strings.Fields(allowed) {
		opts[k] = true
	}
	inline := ""
	scan := bufio.NewScanner(strings.NewReader(config))
	scan.Buffer(make([]byte, 4096), 512<<10)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if inline != "" {
			if line == "</"+inline+">" {
				inline = ""
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "<") {
			tag := strings.TrimSuffix(strings.TrimPrefix(line, "<"), ">")
			switch tag {
			case "ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2":
				inline = tag
				continue
			}
			return fmt.Errorf("不允许的 OpenVPN 内联块: %s", line)
		}
		fields := strings.Fields(line)
		option := strings.TrimPrefix(fields[0], "--")
		if !opts[option] {
			return fmt.Errorf("不允许的 OpenVPN 选项: %s", option)
		}
		// Only the application's own credentials may be read from disk.
		if option == "auth-user-pass" && len(fields) != 1 {
			return fmt.Errorf("auth-user-pass 不允许指定文件")
		}
		if option == "setenv" && (len(fields) < 2 || fields[1] != "CLIENT_CERT") {
			return fmt.Errorf("不允许的 setenv 环境变量")
		}
	}
	if inline != "" {
		return fmt.Errorf("OpenVPN 内联块未闭合: %s", inline)
	}
	return scan.Err()
}

// Handshake only: dev=null and noexec prohibit host interface/route changes.
func probeOpenVPNHandshake(config string, timeout time.Duration) error {
	if err := validateOpenVPNConfig(config); err != nil {
		return err
	}
	bin, err := exec.LookPath("openvpn")
	if err != nil {
		return fmt.Errorf("真实握手需要 openvpn: %w", err)
	}
	dir, err := os.MkdirTemp("", "fanout-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cfg, auth, logFile := filepath.Join(dir, "probe.ovpn"), filepath.Join(dir, "auth"), filepath.Join(dir, "probe.log")
	if err = os.WriteFile(cfg, []byte(config), 0600); err != nil {
		return err
	}
	if err = os.WriteFile(auth, []byte("vpn\nvpn\n"), 0600); err != nil {
		return err
	}
	if timeout < 8*time.Second {
		timeout = 8 * time.Second
	}
	if timeout > 15*time.Second {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--config", cfg, "--dev", "null", "--ifconfig-noexec", "--route-noexec", "--script-security", "1", "--auth-user-pass", auth, "--auth-nocache", "--verb", "3", "--log", logFile)
	if err = cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return fmt.Errorf("OpenVPN 握手未完成: %v", err)
		case <-ctx.Done():
			<-done
			return fmt.Errorf("OpenVPN 握手超时")
		case <-ticker.C:
			b, _ := os.ReadFile(logFile)
			if strings.Contains(string(b), "Initialization Sequence Completed") {
				cancel()
				<-done
				return nil
			}
		}
	}
}
