#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${DIR}/docker-compose.yml"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

function print_banner() {
    echo -e "${CYAN}"
    echo "  __   __ _"
    echo "  \\ \\ / /| |_  __ _"
    echo "   \\ V / | __|/ _\` |"
    echo "    | |  | |_| (_| |  _   [On-Premise Interactive Demo]"
    echo "    |_|   \\__|\\__, | (_)  Deploy, Supervision & Self-Healing"
    echo "              |___/"
    echo -e "${NC}"
}

function start_demo() {
    echo -e "${BLUE}==> [1/3] Docker イメージをビルド中（Pure Go / Cgo=0）...${NC}"
    docker compose -f "${COMPOSE_FILE}" build
    
    echo -e "${BLUE}==> [2/3] オンプレミス模擬コンテナを起動中...${NC}"
    docker compose -f "${COMPOSE_FILE}" up -d
    
    echo -e "${BLUE}==> [3/3] ヘルスチェック待機中...${NC}"
    for i in {1..15}; do
        if curl -s http://localhost:8080/healthz > /dev/null 2>&1; then
            echo -e "${GREEN}✓ デモ環境が正常に起動しました！${NC}"
            break
        fi
        sleep 1
    done
    
    echo ""
    echo -e "${GREEN}================================================================${NC}"
    echo -e " 🌐 ブラウザで確認: ${YELLOW}http://localhost:8080${NC}"
    echo -e " 📊 CUIステータス:  ${CYAN}./demo/run.sh status${NC}"
    echo -e " 💥 障害注入実験:    ${RED}./demo/run.sh chaos${NC}"
    echo -e "${GREEN}================================================================${NC}"
}

function show_status() {
    echo -e "${CYAN}==> [ytg supervisor status] コンテナ内プロセスの看取り状態:${NC}"
    docker compose -f "${COMPOSE_FILE}" exec demo-server ytg supervisor status
}

function run_chaos() {
    echo -e "${YELLOW}==> [Chaos Injection] 意図的に demo-api プロセスを強制終了 (SIGKILL) します...${NC}"
    BEFORE_PID=$(docker compose -f "${COMPOSE_FILE}" exec demo-server pgrep -f demo-api || true)
    echo -e "現在の demo-api PID: ${YELLOW}${BEFORE_PID}${NC}"
    
    echo -e "${RED}⚡ 実行: kill -9 ${BEFORE_PID}${NC}"
    docker compose -f "${COMPOSE_FILE}" exec demo-server pkill -9 -f demo-api || true
    
    echo -e "${BLUE}==> Supervisor がミリ秒単位で検知・自己修復するのを待機中 (1.5秒)...${NC}"
    sleep 1.5
    
    AFTER_PID=$(docker compose -f "${COMPOSE_FILE}" exec demo-server pgrep -f demo-api || true)
    echo -e "${GREEN}✓ 自動自己修復完了！${NC} 新しい PID: ${GREEN}${AFTER_PID}${NC}"
    echo ""
    show_status
}

function run_rollback() {
    echo -e "${YELLOW}==> [Escape Hatch] 手動ロールバック（切り戻し）を実行します...${NC}"
    docker compose -f "${COMPOSE_FILE}" exec demo-server ytg supervisor rollback --all
    echo -e "${GREEN}✓ ロールバック実行完了${NC}"
    echo ""
    show_status
}

function show_logs() {
    echo -e "${BLUE}==> コンテナログを表示します (Ctrl+C で終了):${NC}"
    docker compose -f "${COMPOSE_FILE}" logs -f demo-server
}

function stop_demo() {
    echo -e "${YELLOW}==> デモ環境を停止・削除中...${NC}"
    docker compose -f "${COMPOSE_FILE}" down
    echo -e "${GREEN}✓ デモ環境を停止しました。${NC}"
}

function interactive_menu() {
    print_banner
    while true; do
        echo -e "${CYAN}--- 対話型デモメニュー ---${NC}"
        echo " 1) 🚀 デモ起動 (Start Docker Environment)"
        echo " 2) 📊 ステータス確認 (ytg supervisor status)"
        echo " 3) 💥 障害注入・自己修復実験 (Kill Process & Self-Healing)"
        echo " 4) ⏪ 手動ロールバック実験 (ytg supervisor rollback)"
        echo " 5) 📜 リアルタイムログ表示 (Docker Logs)"
        echo " 6) 🌐 ブラウザでWeb画面を開く (open http://localhost:8080)"
        echo " 7) 🛑 デモ停止 (Stop & Clean up)"
        echo " 0) 終了 (Exit)"
        echo -n "選択してください [0-7]: "
        read -r choice
        echo ""
        
        case $choice in
            1) start_demo ;;
            2) show_status ;;
            3) run_chaos ;;
            4) run_rollback ;;
            5) show_logs ;;
            6) 
               if which open > /dev/null; then open "http://localhost:8080"; 
               elif which xdg-open > /dev/null; then xdg-open "http://localhost:8080"; 
               fi ;;
            7) stop_demo ;;
            0) echo "終了します。"; exit 0 ;;
            *) echo -e "${RED}無効な選択です。${NC}" ;;
        esac
        echo ""
    done
}

case "$1" in
    start)    start_demo ;;
    status)   show_status ;;
    chaos)    run_chaos ;;
    rollback) run_rollback ;;
    logs)     show_logs ;;
    stop)     stop_demo ;;
    *)        interactive_menu ;;
esac
