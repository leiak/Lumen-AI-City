#!/bin/bash
# scripts/cross-city-7day-stability.sh
# 每小时一次跑 check-cross-city.sh，记录到 /tmp/cross-city-stability.log
LOG=/tmp/cross-city-stability.log
while true; do
  echo "=== $(date -Iseconds) ===" >> "$LOG"
  ./scripts/check-cross-city.sh >> "$LOG" 2>&1 || echo "FAIL at $(date -Iseconds)" >> "$LOG"
  sleep 3600
done