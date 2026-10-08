#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${DIR}/.." && pwd)"
BIN_DIR="${DIR}/bin"
PID_FILE="${DIR}/.supervisor.pid"
MANIFEST_FILE="${DIR}/manifest-local.yaml"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

function start_local() {
    echo -e "${BLUE}==> [1/3] ローカルネイティブバイナリをビルド中...${NC}"
    mkdir -p "${BIN_DIR}"
    go build -o "${BIN_DIR}/local-ytg" "${ROOT_DIR}/cmd/ytg"
    go build -o "${BIN_DIR}/local-demo-api" "${DIR}/cmd/demo-api"
    go build -o "${BIN_DIR}/local-demo-worker" "${DIR}/cmd/demo-worker"

    if [ -f "${PID_FILE}" ] && kill -0 $(cat "${PID_FILE}") 2>/dev/null; then
        echo -e "${YELLOW}すでに Supervisor が稼働中です (PID: $(cat "${PID_FILE}"))${NC}"
        return
    fi

    echo -e "${BLUE}==> [2/3] ytg supervisor をバックグラウンド起動中 (Port 8081)...${NC}"
    "${BIN_DIR}/local-ytg" supervisor run -c "${MANIFEST_FILE}" > "${DIR}/supervisor.log" 2>&1 &
    echo $! > "${PID_FILE}"
    sleep 1

    echo -e "${GREEN}✓ ローカルデモ環境が起動しました！ (PID: $(cat "${PID_FILE}"))${NC}"
    echo ""
    echo -e "${GREEN}================================================================${NC}"
    echo -e " 🌐 ブラウザで確認: ${YELLOW}http://localhost:8081${NC}"
    echo -e " 📊 CUIステータス:  ${CYAN}./demo/run-local.sh status${NC}"
    echo -e " 💥 障害注入実験:    ${RED}./demo/run-local.sh chaos${NC}"
    echo -e " 🛑 デモ停止:        ${RED}./demo/run-local.sh stop${NC}"
    echo -e "${GREEN}================================================================${NC}"
}

function show_status() {
    "${BIN_DIR}/local-ytg" supervisor status
}

function run_chaos() {
    API_PID=$(pgrep -f "local-demo-api" || true)
    if [ -z "$API_PID" ]; then
        echo -e "${RED}demo-api プロセスが見つかりません。${NC}"
        return
    fi
    echo -e "${RED}⚡ 実行: kill -9 ${API_PID}${NC}"
    kill -9 $API_PID || true
    echo -e "${BLUE}==> Supervisor の自己修復を待機中 (1.5秒)...${NC}"
    sleep 1.5
    NEW_PID=$(pgrep -f "local-demo-api" || true)
    echo -e "${GREEN}✓ 自動復旧完了！ 新しい PID: ${NEW_PID}${NC}"
    echo ""
    show_status
}

function run_rollback() {
    "${BIN_DIR}/local-ytg" supervisor rollback --all
    echo ""
    show_status
}

function stop_local() {
    if [ -f "${PID_FILE}" ]; then
        PID=$(cat "${PID_FILE}")
        if kill -0 $PID 2>/dev/null; then
            echo -e "${YELLOW}Supervisor プロセス (PID: $PID) を終了中...${NC}"
            kill $PID || true
        fi
        rm -f "${PID_FILE}"
    fi
    pkill -f "local-demo-api" || true
    pkill -f "local-demo-worker" || true
    echo -e "${GREEN}✓ ローカルデモ環境を安全に停止しました。${NC}"
}

case "$1" in
    start)    start_local ;;
    status)   show_status ;;
    chaos)    run_chaos ;;
    rollback) run_rollback ;;
    stop)     stop_local ;;
    *)
        echo "Usage: ./demo/run-local.sh <start|status|chaos|rollback|stop>"
        ;;
esac
