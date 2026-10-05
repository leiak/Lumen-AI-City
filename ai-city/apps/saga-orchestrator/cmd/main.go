package main

import (
	"log"
	"net/http"

	"github.com/aicity/saga-orchestrator/internal/state"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	sagaTriggerTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "saga_trigger_total",
			Help: "Total number of saga triggers",
		},
		[]string{"saga_id"},
	)
	sagaCompensationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "saga_compensation_total",
			Help: "Total number of saga compensations triggered",
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(sagaTriggerTotal)
	prometheus.MustRegister(sagaCompensationTotal)
}

var sagaStore = state.NewStore()

func main() {
	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "saga-orchestrator"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "kafka": false})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// POST /v1/saga — 创建 Saga
	r.POST("/v1/saga", func(c *gin.Context) {
		var req struct {
			SagaID   string           `json:"saga_id"`
			PlayerID string           `json:"player_id"`
			Steps    []state.SagaStep `json:"steps"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		saga := sagaStore.Create(req.SagaID, req.PlayerID, req.Steps)
		c.JSON(http.StatusOK, saga)
	})

	// GET /v1/saga/:id — 查询 Saga
	r.GET("/v1/saga/:id", func(c *gin.Context) {
		id := c.Param("id")
		saga, err := sagaStore.Get(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, saga)
	})

	// POST /v1/saga/trigger — 触发预定义剧本（welcome_3npc 等）
	r.POST("/v1/saga/trigger", func(c *gin.Context) {
		var req struct {
			SagaID    string `json:"saga_id"`
			PlayerID  string `json:"player_id"`
			ForceFail string `json:"force_fail,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// 加载 saga 定义（简化：从 yaml 静态加载）
		steps := loadSagaSteps(req.SagaID)
		if len(steps) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "saga definition not found: " + req.SagaID})
			return
		}
		// Force fail 注入
		for i := range steps {
			if req.ForceFail != "" && steps[i].WorkerID == req.ForceFail {
				steps[i].Payload["force_fail"] = true
			}
		}
		// 发布到 Kafka（简化：log 模拟）
		for _, step := range steps {
			log.Printf("publish to saga.npc.action: %+v", step)
			// TODO: kafka producer
		}
		sagaTriggerTotal.WithLabelValues(req.SagaID).Inc()
		c.JSON(http.StatusOK, gin.H{"status": "triggered", "saga_id": req.SagaID, "steps_count": len(steps)})
	})

	log.Println("saga-orchestrator listening on :9100")
	r.Run(":9100")
}

// loadSagaSteps 加载 saga 定义（welcome_3npc.yaml 简化版）
func loadSagaSteps(sagaID string) []state.SagaStep {
	if sagaID == "welcome_3npc" || sagaID == "welcome_3npc_fail" {
		return []state.SagaStep{
			{WorkerID: "worker_a", Action: "say", Payload: map[string]any{"text": "欢迎来到 A 城！"}},
			{WorkerID: "worker_b", Action: "say", Payload: map[string]any{"text": "我是护士 grace，欢迎你！"}},
			{WorkerID: "worker_c", Action: "say", Payload: map[string]any{"text": "书店在城东，随时来坐坐。"}},
		}
	}
	return []state.SagaStep{}
}