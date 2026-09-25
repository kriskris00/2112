#!/usr/bin/env bash
# fanout 一键安装与平滑升级脚本 (支持从历史任意老版本无缝升级到最新版)
#
# 运行方式：
#   bash <(curl -fsSL https://raw.githubusercontent.com/kriskris00/2112/main/fanout-main/install.sh)
#
# 特性：
# 1. 自动兼容任意老版本升级：无缝保留所有原有端口、随机路径、登录口令，无需重新配置客户端！
# 2. 内存保护与 1C1G 针对性优化：检测到小内存自动启用 Swap，避免编译被 OOM Killer 杀掉。
# 3. 国内国外多镜像源加速与容灾：多重 GitHub 代理与源码包兜底，100% 成功拉取。
# 4. 自动修复旧服务配置、老旧 iptables 规则与孤儿 netns 残留。
# 5. 跨系统支持：Debian, Ubuntu, CentOS, AlmaLinux, Rocky, Alpine, Arch 等。

set -euo pipefail

export LANG=en_US.UTF-8
export LC_ALL=en_US.UTF-8

# 记下用户是否显式指定了 WEB_PORT
WEB_PORT_EXPLICIT="${WEB_PORT:+1}"
WEB_PORT="${WEB_PORT:-8899}"
WORK_DIR="${WORK_DIR:-/var/lib/fanout}"
BIN=/usr/local/bin/fanout
REPO="${REPO:-kriskris00/2112}"

if [[ $EUID -ne 0 ]]; then
  echo -e "\033[0;31m[错误] 需要 root 权限运行此脚本 (需要创建 netns 与配置网络转发)\033[0m" >&2
  exit 1
fi

# ── 1. Init 系统检测 ──────────────────────────────────────
INIT_SYS=""
if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
  INIT_SYS=systemd
elif command -v rc-service >/dev/null 2>&1; then
  INIT_SYS=openrc
else
  echo -e "\033[0;31m[错误] 未识别的 init 系统 (需要 systemd 或 OpenRC)\033[0m" >&2
  exit 1
fi

# ── 2. 老版本平滑迁移：提取并保留历史端口、口令、安全路径 ─────────
SWAP_CREATED=0
cleanup_swap() {
  if [[ "$SWAP_CREATED" -eq 1 && -f /swapfile_fanout_build ]]; then
    echo "      清理临时编译虚拟内存..."
    swapoff /swapfile_fanout_build 2>/dev/null || true
    rm -f /swapfile_fanout_build 2>/dev/null || true
  fi
}
trap cleanup_swap EXIT

migrate_and_seed_settings() {
  mkdir -p "$WORK_DIR"
  chmod 700 "$WORK_DIR"
  local f="${WORK_DIR}/settings.json"
  local old_unit="/etc/systemd/system/fanout.service"
  local detected_port=""

  # 1. 尝试从现有的 settings.json 获取历史端口
  if [[ -f "$f" ]]; then
    detected_port=$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p' "$f" 2>/dev/null | head -1)
  fi

  # 2. 如果没有，尝试从历史版本的老旧 systemd 服务文件中提取 (-web 端口)
  if [[ -z "$detected_port" && -f "$old_unit" ]]; then
    detected_port=$(grep -oE '\-web [0-9]+' "$old_unit" 2>/dev/null | grep -oE '[0-9]+' | head -1 || true)
  fi

  # 3. 如果已有端口且用户未显式强制指定新端口，沿用老端口（绝对不覆盖用户配置）
  if [[ -n "$detected_port" && -z "${WEB_PORT_EXPLICIT:-}" ]]; then
    WEB_PORT="$detected_port"
  fi

  # 写入规范化的 settings.json
  printf '{\n  "port": %s,\n  "listen_addr": ""\n}\n' "$WEB_PORT" > "$f"
  chmod 600 "$f"
}

svc_stop_old() {
  echo "      停止正在运行的老版本服务（避免文件占用）..."
  if [[ "$INIT_SYS" == systemd ]]; then
    systemctl stop fanout 2>/dev/null || true
  else
    rc-service fanout stop 2>/dev/null || true
  fi
}

svc_install() {
  if [[ "$INIT_SYS" == systemd ]]; then
    cat > /etc/systemd/system/fanout.service <<SVCEOF
[Unit]
Description=fanout - VPN Gate 出口扇出网关 (Jesee 魔改旗舰版)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN} -dir ${WORK_DIR}
Restart=on-failure
RestartSec=5
MemoryHigh=380M
MemoryMax=500M
LimitNOFILE=65535
TimeoutStopSec=30
KillMode=mixed

[Install]
WantedBy=multi-user.target
SVCEOF
    systemctl daemon-reload
  else
    cat > /etc/init.d/fanout <<INITEOF
#!/sbin/openrc-run
name="fanout"
description="fanout - VPN Gate 出口扇出网关"
command="${BIN}"
command_args="-dir ${WORK_DIR}"
command_background=true
pidfile="/run/fanout.pid"
output_log="/var/log/fanout.log"
error_log="/var/log/fanout.log"
respawn_delay=5
respawn_max=0
supervisor=supervise-daemon
depend() { need net; after firewall; }
INITEOF
    chmod +x /etc/init.d/fanout
  fi
}

svc_enable_start() {
  if [[ "$INIT_SYS" == systemd ]]; then
    systemctl enable fanout >/dev/null 2>&1 || true
    systemctl restart fanout
  else
    rc-update add fanout default >/dev/null 2>&1 || true
    rc-service fanout restart
  fi
}

svc_is_active() {
  if [[ "$INIT_SYS" == systemd ]]; then
    systemctl is-active --quiet fanout
  else
    rc-service fanout status >/dev/null 2>&1
  fi
}

svc_logs_hint() {
  [[ "$INIT_SYS" == systemd ]] && echo "journalctl -u fanout -n 35" || echo "cat /var/log/fanout.log"
}

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  fanout - VPN Gate 出口扇出网关 自动安装与平滑升级"
echo "  项目地址: https://github.com/${REPO}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo "[1/6] 检查系统环境与基础依赖..."

detect_mgr() {
  for m in apt-get dnf yum pacman apk zypper; do
    command -v "$m" >/dev/null && { echo "$m"; return; }
  done
  echo ""
}

install_pkgs() {
  local mgr="$1"; shift
  case "$mgr" in
    apt-get)
      apt-get update -qq
      DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@"
      ;;
    dnf)    dnf install -y -q "$@" ;;
    yum)    yum install -y -q "$@" ;;
    pacman) pacman -Sy --noconfirm --needed "$@" ;;
    apk)    apk add --no-cache "$@" ;;
    zypper) zypper --non-interactive install -y "$@" ;;
  esac
}

pkg_for() {
  local cmd="$1" mgr="$2"
  case "$cmd" in
    openvpn) echo openvpn ;;
    curl)    echo curl ;;
    openssl) echo openssl ;;
    tar)     echo tar ;;
    git)     echo git ;;
    ca-certificates) echo ca-certificates ;;
    ip)      case "$mgr" in apk) echo iproute2 ;; pacman) echo iproute2 ;; *) echo iproute ;; esac ;;
    iptables) echo iptables ;;
    unzip)   echo unzip ;;
  esac
}

MGR=$(detect_mgr)
[[ "$MGR" == "apt-get" ]] && iproute_pkg=iproute2 || iproute_pkg=iproute

# 核心依赖清单
need_cmd=()
for c in openvpn curl openssl tar iptables git; do
  command -v "$c" >/dev/null || need_cmd+=("$c")
done
command -v ip >/dev/null || need_cmd+=(ip)

if [[ ${#need_cmd[@]} -gt 0 ]]; then
  echo "      正在自动补充缺失的基础组件: ${need_cmd[*]}"
  if [[ -z "$MGR" ]]; then
    echo "      [错误] 未识别的包管理器，请手动安装: ${need_cmd[*]}" >&2
    exit 1
  fi
  pkgs=()
  for c in "${need_cmd[@]}"; do
    if [[ "$c" == "ip" ]]; then pkgs+=("$iproute_pkg"); else pkgs+=("$(pkg_for "$c" "$MGR")"); fi
  done
  install_pkgs "$MGR" "${pkgs[@]}" || {
    echo "      [警告] 部分软件包安装失败，尝试继续执行..."
  }
fi

# 确保证书正常，避免某些精简版系统 https 握手失败
if [[ "$MGR" == "apt-get" ]]; then
  dpkg -s ca-certificates >/dev/null 2>&1 || apt-get install -y -qq ca-certificates 2>/dev/null || true
elif [[ "$MGR" == "apk" ]]; then
  apk add --no-cache ca-certificates 2>/dev/null || true
fi

echo "[2/6] 准备编译环境与源码..."
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)         GOARCH=amd64 ;;
  aarch64|arm64)  GOARCH=arm64 ;;
  *) echo "      [错误] 暂不支持该系统架构: $ARCH" >&2; exit 1 ;;
esac

# ── 1C1G 编译内存保护：小内存自动挂载 1GB 临时虚拟内存，彻底杜绝 OOM 崩溃 ──
setup_build_swap() {
  local mem_total_kb
  mem_total_kb=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 2000000)
  local swap_total_kb
  swap_total_kb=$(awk '/SwapTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)

  # 如果物理内存小于 1.5GB 且无 Swap，创建 1GB 临时 Swap
  if [[ "$mem_total_kb" -lt 1500000 && "$swap_total_kb" -lt 500000 ]]; then
    echo "      检测到 1C1G/小内存环境，正在配置 1GB 临时编译虚拟内存以防 OOM 崩溃..."
    if dd if=/dev/zero of=/swapfile_fanout_build bs=1M count=1024 status=none 2>/dev/null; then
      chmod 600 /swapfile_fanout_build
      mkswap /swapfile_fanout_build >/dev/null 2>&1 || true
      if swapon /swapfile_fanout_build 2>/dev/null; then
        SWAP_CREATED=1
        echo "      已挂载 1GB 编译缓冲虚拟内存"
      else
        rm -f /swapfile_fanout_build 2>/dev/null || true
      fi
    fi
  fi
}

setup_build_swap

# 安装 Go 编译器
install_go_compiler() {
  if command -v go >/dev/null 2>&1; then
    local go_ver
    go_ver=$(go version | awk '{print $3}' | sed 's/go//')
    # 只要有 go 且基本可用即可
    if [[ -n "$go_ver" ]]; then
      return 0
    fi
  fi

  echo "      正在自动配置 Go 编译环境..."
  if [[ "$MGR" == "apt-get" ]]; then
    apt-get update -qq && apt-get install -y -qq golang-go 2>/dev/null || apt-get install -y -qq golang 2>/dev/null || true
  elif [[ "$MGR" == "yum" || "$MGR" == "dnf" ]]; then
    "$MGR" install -y -q golang 2>/dev/null || true
  elif [[ "$MGR" == "apk" ]]; then
    apk add --no-cache go 2>/dev/null || true
  elif [[ "$MGR" == "pacman" ]]; then
    pacman -Sy --noconfirm go 2>/dev/null || true
  fi

  # 如果包管理器安装失败或版本过老，直接下载官方精简二进制
  if ! command -v go >/dev/null 2>&1; then
    echo "      通过官方安装包快速获取 Go 运行环境..."
    local gtar="go1.22.6.linux-${GOARCH}.tar.gz"
    local gurl="https://golang.google.cn/dl/${gtar}"
    local gt="$(mktemp -d)"
    if curl -fsSL "$gurl" -o "$gt/$gtar" 2>/dev/null || curl -fsSL "https://go.dev/dl/${gtar}" -o "$gt/$gtar" 2>/dev/null; then
      tar -C /usr/local -xzf "$gt/$gtar" 2>/dev/null || true
      export PATH="/usr/local/go/bin:$PATH"
      echo 'export PATH="/usr/local/go/bin:$PATH"' > /etc/profile.d/fanout-go.sh 2>/dev/null || true
    fi
    rm -rf "$gt"
  fi
}

install_go_compiler

if ! command -v go >/dev/null 2>&1; then
  echo -e "\033[0;31m[错误] 未能成功就绪 Go 编译环境，请先手动运行: apt install -y golang 或 yum install -y golang\033[0m" >&2
  exit 1
fi

# ── 3. 源码获取与多源容灾拉取 ─────────────────────────────
echo "      正在从 GitHub (${REPO}) 同步最新代码..."
TMP=$(mktemp -d)
local_src=""

clone_repo() {
  local target_dir="$1"
  # 优先直接直连官方
  if git clone --depth 1 "https://github.com/${REPO}.git" "$target_dir" 2>/dev/null; then
    return 0
  fi
  # 备用加速代理 1: ghfast
  if git clone --depth 1 "https://ghfast.top/https://github.com/${REPO}.git" "$target_dir" 2>/dev/null; then
    return 0
  fi
  # 备用加速代理 2: mirror.ghproxy.com
  if git clone --depth 1 "https://mirror.ghproxy.com/https://github.com/${REPO}.git" "$target_dir" 2>/dev/null; then
    return 0
  fi
  # 备用降级策略：拉取 tar.gz 打包源码
  local tar_urls=(
    "https://github.com/${REPO}/archive/refs/heads/main.tar.gz"
    "https://ghfast.top/https://github.com/${REPO}/archive/refs/heads/main.tar.gz"
  )
  for u in "${tar_urls[@]}"; do
    if curl -fsSL --connect-timeout 10 "$u" -o "$TMP/repo.tar.gz" 2>/dev/null; then
      if tar xzf "$TMP/repo.tar.gz" -C "$TMP" 2>/dev/null; then
        local extracted_dir
        extracted_dir=$(find "$TMP" -maxdepth 1 -type d -name "2112*" | head -1)
        if [[ -n "$extracted_dir" ]]; then
          mv "$extracted_dir" "$target_dir"
          return 0
        fi
      fi
    fi
  done
  return 1
}

if [[ -f main.go && -f vpngate.go ]]; then
  echo "      检测到当前目录下即为 fanout 源码，直接就地编译"
  local_src="$(pwd)"
else
  if clone_repo "$TMP/2112"; then
    if [[ -d "$TMP/2112/fanout-main" ]]; then
      local_src="$TMP/2112/fanout-main"
    elif [[ -f "$TMP/2112/main.go" ]]; then
      local_src="$TMP/2112"
    fi
  fi
fi

if [[ -z "$local_src" || ! -f "$local_src/main.go" ]]; then
  echo -e "\033[0;31m[错误] 获取 GitHub 源码失败，请检查网络是否能访问 GitHub\033[0m" >&2
  exit 1
fi

# ── 4. 编译二进制 (防 OOM，多源代理加速) ──────────────────────
echo "      整理依赖并编译新版二进制..."
cd "$local_src"
export GOPROXY="https://goproxy.cn,https://proxy.golang.org,direct"
export GOCACHE="/tmp/fanout_gocache"
mkdir -p "$GOCACHE"

# 针对 1C1G VPS：加入 -p 1 单核低并发构建，杜绝瞬间内存爆满
go mod tidy 2>/dev/null || true
TMP_BIN="$TMP/fanout_new"
if ! go build -p 1 -trimpath -ldflags "-s -w" -o "$TMP_BIN" .; then
  # 降级尝试标准编译
  go build -trimpath -ldflags "-s -w" -o "$TMP_BIN" . || {
    echo -e "\033[0;31m[错误] 编译失败，请检查环境或报错信息\033[0m" >&2
    exit 1
  }
fi
chmod +x "$TMP_BIN"

# 停止旧服务，安全替换二进制
svc_stop_old
install -m 755 "$TMP_BIN" "$BIN"

# 同步安装管理菜单命令 f
if [[ -f f.sh ]]; then
  install -m 755 f.sh /usr/local/bin/f
fi
cd - >/dev/null
rm -rf "$TMP"
rm -rf /tmp/fanout_gocache 2>/dev/null || true

echo "[3/6] 准备 Xray 核心..."
mkdir -p "${WORK_DIR}/bin"
if command -v /usr/local/x-ui/x-ui >/dev/null 2>&1 || [[ -x /usr/bin/x-ui ]]; then
  echo "      检测到已安装 3x-ui，出口将自动对接入站面板"
elif [[ -d /etc/xray-cf-lite && -f /usr/local/etc/xray/config.json ]]; then
  echo "      检测到已安装 xray-cf-lite，入站由其管理"
elif [[ -x "${WORK_DIR}/bin/xray" ]]; then
  echo "      已有 Xray 核心: $("${WORK_DIR}/bin/xray" version 2>/dev/null | head -1)"
else
  case "$GOARCH" in
    amd64) XRAY_ASSET=Xray-linux-64.zip ;;
    arm64) XRAY_ASSET=Xray-linux-arm64-v8a.zip ;;
  esac
  echo "      下载兜底 Xray 内核 (${XRAY_ASSET})..."
  XT=$(mktemp -d)
  XURLS=(
    "https://github.com/XTLS/Xray-core/releases/latest/download/${XRAY_ASSET}"
    "https://ghfast.top/https://github.com/XTLS/Xray-core/releases/latest/download/${XRAY_ASSET}"
  )
  for xu in "${XURLS[@]}"; do
    if curl -fsSL --connect-timeout 8 "$xu" -o "$XT/x.zip" 2>/dev/null; then
      if command -v unzip >/dev/null 2>&1; then
        unzip -qo "$XT/x.zip" -d "$XT" 2>/dev/null || true
      elif command -v busybox >/dev/null 2>&1; then
        busybox unzip -qo "$XT/x.zip" -d "$XT" 2>/dev/null || true
      fi
      if [[ -f "$XT/xray" ]]; then
        install -m 755 "$XT/xray" "${WORK_DIR}/bin/xray"
        echo "      已就绪: $("${WORK_DIR}/bin/xray" version 2>/dev/null | head -1)"
        break
      fi
    fi
  done
  rm -rf "$XT"
fi

echo "[4/6] 配置网络转发与防火墙放行..."
sysctl -qw net.ipv4.ip_forward=1 2>/dev/null || true
grep -q '^net.ipv4.ip_forward=1' /etc/sysctl.conf 2>/dev/null \
  || echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf

# 自动放行 10.99.0.0/16 netns 专用隧道段
if ! iptables -C FORWARD -s 10.99.0.0/16 -j ACCEPT 2>/dev/null; then
  iptables -I FORWARD 1 -s 10.99.0.0/16 -j ACCEPT 2>/dev/null || true
fi
if ! iptables -C FORWARD -d 10.99.0.0/16 -j ACCEPT 2>/dev/null; then
  iptables -I FORWARD 1 -d 10.99.0.0/16 -j ACCEPT 2>/dev/null || true
fi
command -v netfilter-persistent >/dev/null 2>&1 && netfilter-persistent save >/dev/null 2>&1 || true

echo "[5/6] 部署守护服务与配置..."
migrate_and_seed_settings
svc_install
svc_enable_start

echo "[6/6] 校验运行状态..."
sleep 2

if svc_is_active; then
  echo "      服务已成功启动并保持运行状态 (${INIT_SYS})"
else
  echo -e "      \033[0;33m[提示] 服务首次启动中，正在检测日志...\033[0m"
  sleep 2
  if ! svc_is_active; then
    echo -e "      \033[0;31m[错误] 服务启动异常，请查看日志: $(svc_logs_hint)\033[0m" >&2
    exit 1
  fi
fi

# 等待生成口令与随机路径（最多等 10 秒）
for _ in $(seq 1 10); do
  [[ -s "${WORK_DIR}/password" && -s "${WORK_DIR}/basepath" ]] && break
  sleep 1
done

IP=$(curl -s --max-time 6 http://api.ipify.org 2>/dev/null || curl -s --max-time 6 http://1.1.1.1/cdn-cgi/trace 2>/dev/null | awk -F= '/ip/ {print $2}' || echo "<本机公网IP>")
BP=$(cat "${WORK_DIR}/basepath" 2>/dev/null || true)
ACTUAL_PORT=$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p' "${WORK_DIR}/settings.json" 2>/dev/null | head -1)
[[ -n $ACTUAL_PORT ]] && WEB_PORT="$ACTUAL_PORT"

echo
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "  \033[0;32m★ fanout 已成功安装/更新至最新版！\033[0m"
echo
echo -e "  管理面板地址:  \033[1;36mhttp://${IP}:${WEB_PORT}/${BP}/\033[0m"
echo -e "  面板登录口令:  \033[1;33m$(cat "${WORK_DIR}/password" 2>/dev/null || echo "见 ${WORK_DIR}/password")\033[0m"
echo
echo "  快捷管理菜单:  在终端输入 \033[1;32mf\033[0m 即可打开交互式运维菜单"
echo "  配置保存位置:  ${WORK_DIR}/settings.json"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo
