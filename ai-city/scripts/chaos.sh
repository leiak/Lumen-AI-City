#!/usr/bin/env bash
# AI City - 故障注入
# 用法: ./scripts/chaos.sh <scenario>

set -euo pipefail

SCENARIO="${1:-kill-pod}"

echo "==> 故障注入: $SCENARIO"

case "$SCENARIO" in
  kill-pod)
    echo "==> 杀掉 agent-os pod"
    kubectl delete pod -n aicity -l app=agent-os --grace-period=0 --force
    ;;
  kafka-lag)
    echo "==> 制造 Kafka lag（暂停 consumer）"
    kubectl scale deployment/saga-worker -n aicity --replicas=0
    sleep 60
    echo "  恢复"
    kubectl scale deployment/saga-worker -n aicity --replicas=3
    ;;
  llm-down)
    echo "==> LLM 不可用模拟（注入 503）"
    # TODO: 用 Toxiproxy 注入
    ;;
  network-partition)
    echo "==> 网络分区（隔离 a2a-gateway）"
    # TODO: NetworkPolicy
    ;;
  *)
    echo "未知场景: $SCENARIO"
    echo "可选: kill-pod | kafka-lag | llm-down | network-partition"
    exit 1
    ;;
esac

echo "==> 故障注入完成。检查告警与恢复情况..."
