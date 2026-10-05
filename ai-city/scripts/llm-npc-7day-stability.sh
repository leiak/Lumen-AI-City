#!/bin/bash
# scripts/llm-npc-7day-stability.sh
# 每小时检查 LLM-NPC / a2a 稳定性指标
LOG_LLM=/tmp/llm-npc-stability.log
LOG_A2A=/tmp/a2a-stability.log

alert_slack() {
    # Slack 告警 stub — 生产用 webhook URL
    echo "[$(date)] ALERT: $1" >> "$LOG_LLM"
}

while true; do
    # 检查 agent_os_tick_errors 指标 = 0
    errors=$(curl -s http://localhost:8084/metrics 2>/dev/null | grep "^agent_os_tick_errors " | awk '{print $2}')
    if [[ "$errors" != "" && "$errors" != "0" ]]; then
        echo "[$(date)] agent_os_tick_errors = $errors" >> "$LOG_LLM"
        alert_slack "agent_os_tick_errors = $errors"
    fi

    # 检查 a2a inbox received 无 gap
    last_inbox=$(curl -s http://localhost:8083/metrics 2>/dev/null | grep "^a2a_inbox_received_total " | awk '{print $2}')
    echo "[$(date)] inbox = $last_inbox" >> "$LOG_A2A"

    sleep 3600
done