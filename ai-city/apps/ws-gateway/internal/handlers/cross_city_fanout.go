// Package handlers —— ws-gateway 跨城事件 fanout（2.0 stage 1, Task 55）。
//
// 订阅 Kafka 主题 `a2a.cross_city.event`，把远端 NPC 的 dialogue/move 等事件
// 转发到本地 Hub.BroadcastToAll()。预期部署：city_a / city_b 各跑一份 ws-gateway
// 实例，两实例订阅同一 Kafka topic，因此本包接受 *hub.Hub 参数（一份实例只
// 持有一个 Hub —— city_a 实例传 hubA；city_b 实例传 hubB）。
//
// Kafka client 注：本文件用最小 stub（无外部依赖）—— 真正的 kafka-go 客户端
// 在后续 Task 引入（go.mod 加 segmentio/kafka-go）。当前 stub：
//   - NewKafkaConsumer 不报错，立刻返回空消息 chan；
//   - Messages() 永不返回消息；
//   - 调用方拿到 nil/空 consumer 时直接退出 goroutine，不影响主流程。
//
// 等真 Kafka 客户端接入时，把 KafkaMessage struct 改为 kafka.Message 即可，
// 本包其它代码无须改。
package handlers

import (
	"encoding/json"
	"log"
	"os"
	"sync"

	"github.com/aicity/ws-gateway/internal/hub"
)

// KafkaMessage Kafka 投递的最小 payload（待替换为 kafka.Message）。
type KafkaMessage struct {
	Topic string
	Value []byte
}

// KafkaConsumer 是 Kafka consumer 的最小接口（暂以 stub 实现）。
//
// 真正的实现应该用 segmentio/kafka-go：NewReader(brokers) + SetGroupID(groupID)
// + ReadMessage()。当前 stub 不连 broker、永远不返回消息。
type KafkaConsumer struct {
	brokers string
	topic   string
	groupID string

	mu      sync.Mutex
	messages chan KafkaMessage
	closed  bool
}

// NewKafkaConsumer 构造 consumer（stub 立即返回成功；不连 broker）。
//
// brokers 形如 "kafka:9092"；topic = "a2a.cross_city.event"；groupID 用
// 实例 ID（"ws-cross-city"）保证两城 consumer 各自独立 offset。
func NewKafkaConsumer(brokers, topic, groupID string) (*KafkaConsumer, error) {
	return &KafkaConsumer{
		brokers:  brokers,
		topic:    topic,
		groupID:  groupID,
		messages: make(chan KafkaMessage, 100),
	}, nil
}

// Messages 返消息 chan（stub 永远空）。
func (k *KafkaConsumer) Messages() <-chan KafkaMessage { return k.messages }

// Close 释放资源（stub no-op）。
func (k *KafkaConsumer) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return nil
	}
	k.closed = true
	close(k.messages)
	return nil
}

// StartCrossCityFanout 启动 Kafka consumer，把 a2a.cross_city.event 转发到本地 Hub。
//
// 环境变量：
//   KAFKA_BROKERS  默认 "kafka:9092"
//
// 入参 hubLocal 通常是本城 Hub（city_a 跑 hubA、city_b 跑 hubB）；同一进程
// 若持有两 Hub 也可调两次本函数（不同 groupID）。
//
// 阻塞：goroutine 形式运行，靠 ctx 取消来关停（stub 暂无外部信号，后续接入
// 真 Kafka 客户端后再 ctx-aware）。
func StartCrossCityFanout(hubLocal *hub.Hub) {
	if hubLocal == nil {
		log.Printf("cross-city fanout: hubLocal nil, skip")
		return
	}
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "kafka:9092"
	}
	topic := "a2a.cross_city.event"
	groupID := "ws-cross-city"

	consumer, err := NewKafkaConsumer(brokers, topic, groupID)
	if err != nil {
		log.Printf("cross-city fanout init failed: %v", err)
		return
	}
	log.Printf("cross-city fanout started: brokers=%s topic=%s group=%s",
		brokers, topic, groupID)

	go func() {
		defer consumer.Close()
		for msg := range consumer.Messages() {
			var event map[string]interface{}
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("decode cross-city event: %v", err)
				continue
			}
			// 重新 marshal 成 hub 期望的 []byte（fanout 是 raw bytes）
			out, err := json.Marshal(event)
			if err != nil {
				log.Printf("re-marshal cross-city event: %v", err)
				continue
			}
			hubLocal.BroadcastToAll(out)
		}
	}()
}

// StartCrossCityFanoutDual 同时启动 fanout 到 hubA + hubB（单进程跑两城用）。
//
// 当前 docker-compose 一城一个 ws-gateway 容器，所以实际部署走 StartCrossCityFanout
// 单 Hub 版本。本函数保留双 Hub 形态，给本地 dual-instance 集成测试用。
func StartCrossCityFanoutDual(hubA, hubB *hub.Hub) {
	StartCrossCityFanout(hubA)
	StartCrossCityFanout(hubB)
}