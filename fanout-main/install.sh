#!/usr/bin/env bash
# fanout 一键安装 / 重装 / 升级脚本
# GitHub 使用方式：
#   bash <(curl -fsSL https://raw.githubusercontent.com/kriskris00/2112/main/fanout-main/install.sh)
#
# 重要：此脚本始终从 GitHub 拉取 main 分支的完整 fanout-main 源码，
# 不依赖“当前目录是什么”，适合把整个项目上传 GitHub 后直接执行。
#
# 可选：
#   FANOUT_CLEAN=1 bash <(curl -fsSL .../install.sh)
#     清理运行时缓存/历史节点/旧状态后重新初始化，但保留 settings/password/basepath。
#   FANOUT_PORT=8899 ...
#     强制指定管理端口；默认沿用已有 settings.json 端口。

set -Eeuo pipefail
export LANG=C.UTF-8
export LC_ALL=C.UTF-8

REPO="${FANOUT_REPO:-kriskris00/2112}"
BRANCH="${FANOUT_BRANCH:-main}"
WORK_DIR="${FANOUT_DIR:-/var/lib/fanout}"
BIN="/usr/local/bin/fanout"
UNIT="/etc/systemd/system/fanout.service"
TMP_ROOT=""
SWAP_CREATED=0

log(){ printf '\033[1;36m[fanout]\033[0m %s\n' "$*"; }
warn(){ printf '\033[1;33m[警告]\033[0m %s\n' "$*" >&2; }
die(){ printf '\033[1;31m[错误]\033[0m %s\n' "$*" >&2; exit 1; }
trap 'rc=$?; [[ -n "${TMP_ROOT:-}" ]] && rm -rf "$TMP_ROOT" 2>/dev/null || true; if [[ "$SWAP_CREATED" == 1 ]]; then swapoff /swapfile_fanout_build 2>/dev/null || true; rm -f /swapfile_fanout_build 2>/dev/null || true; fi; exit $rc' EXIT

[[ $EUID -eq 0 ]] || die "请使用 root 运行。"

# ---------- init ----------
if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
  INIT=systemd
elif command -v rc-service >/dev/null 2>&1; then
  INIT=openrc
else
  die "仅支持 systemd / OpenRC。"
fi

# ---------- package manager ----------
PKG_MGR=""
for x in apt-get dnf yum apk pacman zypper; do
  if command -v "$x" >/dev/null 2>&1; then PKG_MGR="$x"; break; fi
done

pkg_name(){
  case "$1:$PKG_MGR" in
    ip:apk|ip:pacman) echo iproute2;;
    ip:*) echo iproute2;;
    ca-certificates:*) echo ca-certificates;;
    *) echo "$1";;
  esac
}

pkg_install(){
  [[ -n "$PKG_MGR" ]] || return 1
  case "$PKG_MGR" in
    apt-get) apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@";;
    dnf) dnf install -y -q "$@";;
    yum) yum install -y -q "$@";;
    apk) apk add --no-cache "$@";;
    pacman) pacman -Sy --noconfirm --needed "$@";;
    zypper) zypper --non-interactive install -y "$@";;
  esac
}

log "检查基础环境"
MISSING=()
for c in curl tar openssl ip iptables git; do command -v "$c" >/dev/null 2>&1 || MISSING+=("$(pkg_name "$c")"); done
# unzip 用于 Xray；若没有，安装。
command -v unzip >/dev/null 2>&1 || MISSING+=(unzip)
if ((${#MISSING[@]})); then
  [[ -n "$PKG_MGR" ]] || die "缺少组件：${MISSING[*]}，且未找到包管理器。"
  # 去重
  mapfile -t MISSING < <(printf '%s\n' "${MISSING[@]}" | awk '!seen[$0]++')
  log "安装缺失组件：${MISSING[*]}"
  pkg_install "${MISSING[@]}" || die "基础依赖安装失败。"
fi

# OpenVPN 是实际出口所需核心。
if ! command -v openvpn >/dev/null 2>&1; then
  pkg_install openvpn || die "OpenVPN 安装失败。"
fi

# ---------- architecture ----------
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64; XRAY_ASSET=Xray-linux-64.zip;;
  aarch64|arm64) GOARCH=arm64; XRAY_ASSET=Xray-linux-arm64-v8a.zip;;
  *) die "暂不支持架构：$ARCH";;
esac

# ---------- settings backup ----------
mkdir -p "$WORK_DIR"
chmod 700 "$WORK_DIR"
SETTINGS="$WORK_DIR/settings.json"
BACKUP_DIR="$WORK_DIR/install-backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

# 端口：显式 FANOUT_PORT > 现有 settings > 8899
WEB_PORT="${FANOUT_PORT:-}"
if [[ -z "$WEB_PORT" && -f "$SETTINGS" ]]; then
  WEB_PORT="$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9]\{1,5\}\).*/\1/p' "$SETTINGS" | head -1 || true)"
fi
WEB_PORT="${WEB_PORT:-8899}"
[[ "$WEB_PORT" =~ ^[0-9]+$ && "$WEB_PORT" -ge 1 && "$WEB_PORT" -le 65535 ]] || die "管理端口无效：$WEB_PORT"

# 备份用户配置，但不把历史运行状态当成新节点池。
for f in settings.json password basepath; do
  [[ -f "$WORK_DIR/$f" ]] && cp -a "$WORK_DIR/$f" "$BACKUP_DIR/$f" || true
done

# ---------- optional clean ----------
if [[ "${FANOUT_CLEAN:-0}" == 1 ]]; then
  log "启用 FANOUT_CLEAN=1：清理历史节点/隧道状态，保留管理端口、口令和路径。"
  rm -f \
    "$WORK_DIR/state.json" \
    "$WORK_DIR/state.json.tmp" \
    "$WORK_DIR/cached_nodes.csv" \
    "$WORK_DIR/nodes.json" \
    "$WORK_DIR/ip_intel.json" \
    "$WORK_DIR/custom_nodes.json" \
    "$WORK_DIR/custom_nodes.yaml" 2>/dev/null || true
  find "$WORK_DIR" -maxdepth 1 -type f -name 'state.json.tmp.*' -delete 2>/dev/null || true
fi

# ---------- stop service ----------
log "停止旧 fanout 服务"
if [[ "$INIT" == systemd ]]; then
  systemctl stop fanout 2>/dev/null || true
else
  rc-service fanout stop 2>/dev/null || true
fi

# 杀掉异常遗留的同名 fanout，避免 8899 被旧进程占用。
if pgrep -x fanout >/dev/null 2>&1; then
  pkill -TERM -x fanout 2>/dev/null || true
  for _ in {1..10}; do pgrep -x fanout >/dev/null 2>&1 || break; sleep 1; done
  pgrep -x fanout >/dev/null 2>&1 && pkill -KILL -x fanout 2>/dev/null || true
fi

# ---------- temporary swap for tiny VPS ----------
setup_build_swap(){
  local mem_kb swap_kb
  mem_kb=$(awk '/MemTotal/ {print $2; exit}' /proc/meminfo 2>/dev/null || echo 2000000)
  swap_kb=$(awk '/SwapTotal/ {print $2; exit}' /proc/meminfo 2>/dev/null || echo 0)
  if [[ "$mem_kb" -lt 1500000 && "$swap_kb" -lt 500000 && ! -e /swapfile_fanout_build ]]; then
    log "小内存 VPS：创建临时 1GB 编译 Swap。"
    if dd if=/dev/zero of=/swapfile_fanout_build bs=1M count=1024 status=none 2>/dev/null; then
      chmod 600 /swapfile_fanout_build
      mkswap /swapfile_fanout_build >/dev/null 2>&1 || true
      if swapon /swapfile_fanout_build 2>/dev/null; then SWAP_CREATED=1; fi
    fi
  fi
}
setup_build_swap

# ---------- Go ----------
install_go(){
  if command -v go >/dev/null 2>&1; then return 0; fi
  log "安装 Go 编译环境"
  case "$PKG_MGR" in
    apt-get) pkg_install golang-go || pkg_install golang || true;;
    dnf|yum) pkg_install golang || true;;
    apk) pkg_install go || true;;
    pacman) pkg_install go || true;;
    zypper) pkg_install go || true;;
  esac
  if ! command -v go >/dev/null 2>&1; then
    local gv="1.22.6" gt="$TMP_ROOT/go.tar.gz"
    mkdir -p "$TMP_ROOT"
    for u in \
      "https://go.dev/dl/go${gv}.linux-${GOARCH}.tar.gz" \
      "https://golang.google.cn/dl/go${gv}.linux-${GOARCH}.tar.gz"; do
      if curl -fL --connect-timeout 10 --max-time 120 "$u" -o "$gt" 2>/dev/null; then
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "$gt"
        export PATH="/usr/local/go/bin:$PATH"
        break
      fi
    done
  fi
  command -v go >/dev/null 2>&1 || die "Go 安装失败。"
}
TMP_ROOT="$(mktemp -d /tmp/fanout-install.XXXXXX)"
install_go

# ---------- fetch GitHub source ----------
# 不使用当前目录源码，避免“从 GitHub 执行却实际编译服务器旧源码”的问题。
log "从 GitHub 拉取 ${REPO}@${BRANCH} 最新完整源码"
SRC="$TMP_ROOT/src"
mkdir -p "$SRC"
FETCH_OK=0

# 方案 A：git clone，能得到完整仓库。
if git clone --depth 1 --branch "$BRANCH" --single-branch \
    "https://github.com/${REPO}.git" "$SRC/repo" 2>/dev/null; then
  FETCH_OK=1
else
  # 方案 B：GitHub 官方 codeload tarball，不依赖 git。
  rm -rf "$SRC/repo" "$SRC/unpack"
  mkdir -p "$SRC/unpack"
  if curl -fL --connect-timeout 10 --max-time 180 \
      "https://codeload.github.com/${REPO}/tar.gz/refs/heads/${BRANCH}" \
      -o "$SRC/repo.tar.gz" 2>/dev/null; then
    tar -xzf "$SRC/repo.tar.gz" -C "$SRC/unpack"
    extracted="$(find "$SRC/unpack" -mindepth 1 -maxdepth 1 -type d | head -1)"
    [[ -n "$extracted" ]] && mv "$extracted" "$SRC/repo" && FETCH_OK=1
  fi
fi

# 最后才使用 GitHub raw 文件兜底，不允许拿当前目录旧源码编译。
if [[ "$FETCH_OK" != 1 ]]; then
  die "GitHub 源码下载失败。请检查服务器访问 github.com/codeload.github.com 的网络。"
fi

# 仓库可能是 repo/fanout-main，也可能直接把 fanout-main 放根目录。
if [[ -f "$SRC/repo/fanout-main/main.go" ]]; then
  APP_SRC="$SRC/repo/fanout-main"
elif [[ -f "$SRC/repo/main.go" ]]; then
  APP_SRC="$SRC/repo"
else
  die "GitHub 仓库中未找到 fanout-main/main.go。请确认源码确实已上传到 main 分支。"
fi

[[ -f "$APP_SRC/go.mod" ]] || die "源码缺少 go.mod：$APP_SRC"
[[ -f "$APP_SRC/main.go" ]] || die "源码缺少 main.go：$APP_SRC"

# 记录 GitHub 当前提交，用于面板 F11 一键更新检测。
BUILD_REVISION=""
if [[ -d "$SRC/repo/.git" ]]; then
  BUILD_REVISION="$(git -C "$SRC/repo" rev-parse HEAD 2>/dev/null || true)"
fi

# ---------- build ----------
log "编译 GitHub 最新源码"
cd "$APP_SRC"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GOMAXPROCS=1
export GOTOOLCHAIN=local
export GOCACHE="$TMP_ROOT/gocache"
mkdir -p "$GOCACHE"

# 不执行 go mod tidy，避免安装脚本修改用户上传源码；只下载并编译。
go mod download
NEW_BIN="$TMP_ROOT/fanout"
go build -p 1 -trimpath -ldflags "-s -w -X main.version=v3.1.0-fanout -X main.buildRevision=${BUILD_REVISION}" -o "$NEW_BIN" .
chmod 755 "$NEW_BIN"
"$NEW_BIN" --help >/dev/null 2>&1 || true

# ---------- Xray ----------
log "准备 Xray 核心"
mkdir -p "$WORK_DIR/bin"
if [[ -x "$WORK_DIR/bin/xray" ]]; then
  log "已有 Xray：$($WORK_DIR/bin/xray version 2>/dev/null | head -1 || true)"
else
  XT="$TMP_ROOT/xray"
  mkdir -p "$XT"
  for u in \
    "https://github.com/XTLS/Xray-core/releases/latest/download/${XRAY_ASSET}" \
    "https://github.com/XTLS/Xray-core/releases/latest/download/${XRAY_ASSET}"; do
    if curl -fL --connect-timeout 10 --max-time 180 "$u" -o "$XT/x.zip" 2>/dev/null; then
      unzip -qo "$XT/x.zip" -d "$XT/out" 2>/dev/null || true
      if [[ -x "$XT/out/xray" || -f "$XT/out/xray" ]]; then
        install -m 755 "$XT/out/xray" "$WORK_DIR/bin/xray"
        break
      fi
    fi
  done
  [[ -x "$WORK_DIR/bin/xray" ]] || warn "Xray 下载失败；如果源码/系统已有 Xray，服务仍可继续启动。"
fi

# ---------- install binary ----------
log "安装新版 fanout 二进制"
install -m 755 "$NEW_BIN" "$BIN"
if [[ -f "$APP_SRC/f.sh" ]]; then install -m 755 "$APP_SRC/f.sh" /usr/local/bin/f; fi

# ---------- settings ----------
printf '{\n  "port": %s,\n  "listen_addr": ""\n}\n' "$WEB_PORT" > "$SETTINGS"
chmod 600 "$SETTINGS"

# ---------- service ----------
log "写入守护服务"
if [[ "$INIT" == systemd ]]; then
  cat > "$UNIT" <<SERVICE
[Unit]
Description=fanout - VPN Gate 出口扇出网关
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=${BIN} -dir ${WORK_DIR}
Restart=always
RestartSec=3
TimeoutStopSec=30
KillMode=mixed
LimitNOFILE=65535
MemoryHigh=700M
MemoryMax=900M
TasksMax=512

[Install]
WantedBy=multi-user.target
SERVICE
  systemctl daemon-reload
  systemctl enable fanout >/dev/null 2>&1 || true
else
  cat > /etc/init.d/fanout <<SERVICE
#!/sbin/openrc-run
name="fanout"
description="fanout - VPN Gate 出口扇出网关"
command="${BIN}"
command_args="-dir ${WORK_DIR}"
command_background=true
pidfile="/run/fanout.pid"
output_log="/var/log/fanout.log"
error_log="/var/log/fanout.log"
respawn_delay=3
respawn_max=0
supervisor=supervise-daemon
depend() { need net; after firewall; }
SERVICE
  chmod +x /etc/init.d/fanout
  rc-update add fanout default >/dev/null 2>&1 || true
fi

# ---------- networking ----------
sysctl -qw net.ipv4.ip_forward=1 2>/dev/null || true
if ! grep -q '^net.ipv4.ip_forward=1' /etc/sysctl.conf 2>/dev/null; then
  echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf
fi
if command -v iptables >/dev/null 2>&1; then
  iptables -C FORWARD -s 10.99.0.0/16 -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -s 10.99.0.0/16 -j ACCEPT 2>/dev/null || true
  iptables -C FORWARD -d 10.99.0.0/16 -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -d 10.99.0.0/16 -j ACCEPT 2>/dev/null || true
  command -v netfilter-persistent >/dev/null 2>&1 && netfilter-persistent save >/dev/null 2>&1 || true
fi

# ---------- start ----------
log "启动新版服务"
if [[ "$INIT" == systemd ]]; then
  systemctl daemon-reload
  systemctl restart fanout
else
  rc-service fanout restart
fi

sleep 3
if [[ "$INIT" == systemd ]]; then
  systemctl is-active --quiet fanout || {
    journalctl -u fanout -n 80 --no-pager -l >&2 || true
    die "fanout 启动失败。"
  }
else
  rc-service fanout status >/dev/null 2>&1 || die "fanout 启动失败。"
fi

# ---------- result ----------
IP="$(curl -4 -fsS --max-time 6 https://api.ipify.org 2>/dev/null || true)"
[[ -n "$IP" ]] || IP="<本机公网IP>"
BP="$(cat "$WORK_DIR/basepath" 2>/dev/null || true)"
if [[ -z "$BP" ]]; then BP="(启动后自动生成)"; fi

log "安装完成"
echo
printf '管理面板： http://%s:%s/%s/\n' "$IP" "$WEB_PORT" "$BP"
printf '配置目录： %s\n' "$WORK_DIR"
printf '二进制：   %s\n' "$BIN"
printf '源码版本： %s/%s\n' "$REPO" "$BRANCH"
PASS="$(cat "$WORK_DIR/password" 2>/dev/null || true)"
if [[ -n "$PASS" ]]; then
  printf '访问密码： %s\n' "$PASS"
else
  warn "未读取到访问密码，请执行：cat $WORK_DIR/password"
fi
echo
if [[ "$INIT" == systemd ]]; then
  echo '查看日志： journalctl -u fanout -n 100 --no-pager -l'
  echo '查看状态： systemctl status fanout --no-pager'
else
  echo '查看日志： cat /var/log/fanout.log'
fi
