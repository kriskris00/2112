#!/usr/bin/env bash
# Replace only fanout, preserving the existing service and runtime configuration.
set -Eeuo pipefail
[[ $EUID -eq 0 ]] || { echo '请使用 root 运行'; exit 1; }
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
TARGET_VERSION='v3.2.1-gov20261018'
BIN=/usr/local/bin/fanout
WORK_DIR=/var/lib/fanout
[[ -f "$BIN" && -d "$WORK_DIR" ]] || { echo '未发现现有安装，请勿使用此升级脚本'; exit 1; }
command -v systemctl >/dev/null || { echo '本升级脚本仅用于现有 systemd 安装'; exit 1; }
case "$(uname -m)" in
  x86_64|amd64) SOURCE=fanout;;
  aarch64|arm64) SOURCE=dist/fanout-linux-arm64;;
  *) echo '不支持该架构'; exit 1;;
esac
[[ -f "$SOURCE" && -f binary-sha256.txt ]] || { echo '请上传并解压完整升级包'; exit 1; }
sha256sum -c binary-sha256.txt
chmod 755 "$SOURCE"
[[ "$("./$SOURCE" -version)" == "fanout $TARGET_VERSION" ]] || { echo '版本校验失败'; exit 1; }
BACKUP="$(mktemp -d /var/backups/fanout-upgrade.XXXXXXXX)"
chmod 700 "$BACKUP"
cp -p -- "$BIN" "$BACKUP/fanout.previous"
systemctl cat fanout > "$BACKUP/fanout.service.txt"
changed=0
rollback() {
  rc=$?
  if [[ $rc -ne 0 && $changed == 1 ]]; then
    echo "升级启动未通过，正在回滚程序；备份在 $BACKUP"
    systemctl stop fanout || true
    install -m 755 "$BACKUP/fanout.previous" "$BIN"
    systemctl start fanout || true
  fi
  exit "$rc"
}
trap rollback EXIT
echo "即将重启 fanout，连接会短暂中断；备份目录：$BACKUP"
systemctl stop fanout
changed=1
# State files are now stable. Back up credentials and configuration privately.
tar -C "$WORK_DIR" --exclude='*.log' -czf "$BACKUP/runtime-before-upgrade.tar.gz" .
install -m 755 "$SOURCE" "$BIN"
systemctl start fanout
sleep 3
systemctl is-active --quiet fanout
"$BIN" -version
echo "程序已更新。请刷新网页，在“容量与保护”中查看出口上限和资源阈值。"
echo "这只确认服务已启动，尚不表示 VPN 出口已验证通过。备份：$BACKUP"
journalctl -u fanout -n 12 --no-pager
