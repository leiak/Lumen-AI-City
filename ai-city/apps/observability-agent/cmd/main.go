// Package main Observability Agent
// 职责：日志采样 / Trace 收集 / Prometheus 指标暴露
// 详细设计见 docs/09-架构优化v2.md §38
package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	log.Println("observability-agent listening on :9091")
	_ = http.ListenAndServe(":9091", nil)
}
