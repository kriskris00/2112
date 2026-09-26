package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const updateRepo = "kriskris00/2112"

// releaseInfo 是 GitHub Releases API 里我们关心的字段。
type releaseInfo struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// UpdateStatus 回给界面：当前版本、最新版本、有没有新版、更新内容。
type UpdateStatus struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"has_update"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`
}

// goarch 把 runtime.GOARCH 映射成 release 资产用的名字。
func assetArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

// fetchLatestRelease 拉取最新 release 元数据。
func fetchLatestRelease() (*releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "fanout-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// fetchBranchRevision 直接读取 GitHub main 分支最新提交。
// 这样即使仓库没有 GitHub Release，面板 F11 也能发现源码更新。
func fetchBranchRevision() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", updateRepo, "main")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "fanout-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	var v struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	if strings.TrimSpace(v.SHA) == "" {
		return "", fmt.Errorf("GitHub 未返回 main 提交 SHA")
	}
	return strings.TrimSpace(v.SHA), nil
}

// checkUpdate 同时检查 GitHub Release 和 main 分支提交。源码仓库有新提交时，
// 即使没有创建 Release，面板也会显示更新，可直接通过 F11 安装最新源码。
func checkUpdate() (*UpdateStatus, error) {
	cur := strings.TrimSpace(version)
	if cur == "" {
		cur = "dev"
	}
	st := &UpdateStatus{Current: cur, Latest: cur, HasUpdate: false, URL: "https://github.com/" + updateRepo}

	latestSHA, shaErr := fetchBranchRevision()
	if shaErr == nil && strings.TrimSpace(buildRevision) != "" && !strings.EqualFold(strings.TrimSpace(buildRevision), latestSHA) {
		st.Latest = "main@" + latestSHA[:12]
		st.HasUpdate = true
		st.Notes = "GitHub main 分支检测到新源码。F11 更新将直接从 GitHub 拉取完整源码、重新编译并重启服务。"
		return st, nil
	}

	// 如果当前版本没有 revision（例如旧版二进制），只要 GitHub 能访问就允许一次源码更新。
	if shaErr == nil && strings.TrimSpace(buildRevision) == "" {
		st.Latest = "main@" + latestSHA[:12]
		st.HasUpdate = true
		st.Notes = "当前版本没有记录 GitHub 提交号，可通过 F11 同步到 main 最新源码。"
		return st, nil
	}

	// Release 仍作为兼容信息来源。
	if rel, err := fetchLatestRelease(); err == nil && rel != nil {
		st.Latest = strings.TrimSpace(rel.TagName)
		st.Notes = strings.TrimSpace(rel.Body)
		st.URL = rel.HTMLURL
		st.HasUpdate = versionLess(cur, st.Latest)
	}
	if shaErr != nil && st.Latest == cur {
		return st, nil
	}
	return st, nil
}

// versionLess 判断 cur 是否比 latest 旧。解析 vX.Y.Z 做数值比较；
// dev 或无法解析时保守认为"有更新"（让用户能装上正式版）。
func versionLess(cur, latest string) bool {
	if latest == "" {
		return false
	}
	if cur == "" || cur == "dev" {
		return true
	}
	cn, cok := parseSemver(cur)
	ln, lok := parseSemver(latest)
	if !cok || !lok {
		return cur != latest
	}
	for i := 0; i < 3; i++ {
		if cn[i] != ln[i] {
			return cn[i] < ln[i]
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// 去掉预发布/构建后缀
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// applyUpdate 下载最新版对应架构的包、校验、替换当前二进制，然后重启服务。
// 成功后本进程会被 init 系统拉起成新版本，所以正常情况下这里返回后进程即被替换。
func applyUpdate() error {
	// F11 优先走 GitHub main 源码安装，而不是依赖 Release。
	// install.sh 会重新拉取最新完整源码、编译并安装，默认保留 settings/password/basepath。
	go func() {
		time.Sleep(800 * time.Millisecond)
		cmd := "curl -fsSL https://raw.githubusercontent.com/kriskris00/2112/main/fanout-main/install.sh | bash"
		_ = exec.Command("bash", "-c", cmd).Run()
	}()
	return nil
}

func downloadFile(url, dst string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "fanout-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func verifyChecksum(path, name, sumsURL string) error {
	sums := filepath.Join(filepath.Dir(path), "checksums.txt")
	if err := downloadFile(sumsURL, sums); err != nil {
		return fmt.Errorf("下载校验和失败: %w", err)
	}
	want, err := sha256FromList(sums, name)
	if err != nil {
		return err
	}
	got, err := sha256File(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(want, got) {
		return fmt.Errorf("校验和不匹配，包可能损坏")
	}
	return nil
}

func sha256FromList(listPath, name string) (string, error) {
	blob, err := os.ReadFile(listPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(blob), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("校验和列表里没有 %s", name)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinary 从 tar.gz 里取出指定文件名的成员写到 dst。
func extractBinary(tarGz, member, dst string) error {
	f, err := os.Open(tarGz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hd, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("包里没有 %s", member)
		}
		if err != nil {
			return err
		}
		if filepath.Base(hd.Name) != member {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, tr); err != nil {
			return err
		}
		return nil
	}
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// restartSelf 通过 init 系统重启 fanout 服务，拉起刚替换的新二进制。
// systemd / openrc 各一套；都不可用时退回直接自我 exec。
func restartSelf() {
	if hasCmd("systemctl") && dirExists("/run/systemd/system") {
		_ = exec.Command("systemctl", "restart", "fanout").Start()
		return
	}
	if hasCmd("rc-service") {
		_ = exec.Command("rc-service", "fanout", "restart").Start()
		return
	}
	// 没有 init 系统托管：直接退出，让外部守护（若有）拉起；
	// 没有守护就只能等下次手动启动。日志留个痕。
	fmt.Println("fanout: 已替换二进制，但未检测到 systemd/openrc，请手动重启服务")
}

func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
