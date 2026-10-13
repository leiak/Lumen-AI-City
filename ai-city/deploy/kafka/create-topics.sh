#!/usr/bin/env bash
set -euo pipefail

BROKERS="${KAFKA_BROKERS:-kafka:9092}"
REPLICATION_FACTOR="${REPLICATION_FACTOR:-1}"
TOPICS_FILE="${TOPICS_FILE:-/opt/kafka/topics.env}"

while IFS="|" read -r topic partitions configs; do
  if [[ -z "${topic}" || "${topic}" == \#* || "${topic}" == "topic" ]]; then
    continue
  fi

  kafka-topics \
    --bootstrap-server "${BROKERS}" \
    --create --if-not-exists \
    --topic "${topic}" \
    --partitions "${partitions}" \
    --replication-factor "${REPLICATION_FACTOR}"

  kafka-configs \
    --bootstrap-server "${BROKERS}" \
    --entity-type topics \
    --entity-name "${topic}" \
    --alter \
    --add-config "${configs//;/,}"

  kafka-topics \
    --bootstrap-server "${BROKERS}" \
    --describe \
    --topic "${topic}"
done < "${TOPICS_FILE}"
