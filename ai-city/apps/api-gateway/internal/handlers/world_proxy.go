// Package handlers - world_proxy 反向代理到 world-engine REST API
package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// WorldProxy 把匹配请求转发到 world-engine
// 路径: /v1/tiles/* 与 /v1/world/*
type WorldProxy struct {
	baseURL *url.URL
	client  *http.Client
}

func NewWorldProxy(rawBaseURL string) (*WorldProxy, error) {
	u, err := url.Parse(rawBaseURL)
	if err != nil {
		return nil, err
	}
	return &WorldProxy{
		baseURL: u,
		client:  &http.Client{},
	}, nil
}

// Proxy 通配处理器
// gin 路径: /v1/tiles/*tileSubPath  与  /v1/world/*worldSubPath
func (p *WorldProxy) Proxy(c *gin.Context) {
	// 还原 world-engine 的目标 URL
	// 当前 gin 路由前缀:
	//   /v1/tiles/:id   →  /v1/tiles/:id
	//   /v1/world/move  →  /v1/world/move
	originalPath := c.Request.URL.Path

	target := *p.baseURL
	target.Path = originalPath
	target.RawQuery = c.Request.URL.RawQuery

	// 构造转发请求
	body, _ := io.ReadAll(c.Request.Body)
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "proxy_build_request_failed", "detail": err.Error()})
		return
	}
	// 透传必要 header
	for _, h := range []string{"Content-Type", "Authorization", "Accept"} {
		if v := c.GetHeader(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "world_engine_unreachable", "detail": err.Error()})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	// 透传响应头（Content-Type）
	for k, vs := range resp.Header {
		if strings.EqualFold(k, "Content-Type") || strings.EqualFold(k, "X-Trace-Id") {
			for _, v := range vs {
				c.Header(k, v)
			}
		}
	}
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
}