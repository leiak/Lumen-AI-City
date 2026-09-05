// Service 实现 A2AGateway 4 个 RPC（docs/06-A2A协议.md §20）。
//
// MVP 行为：
//   - RegisterCard: 调 Registry.Register；缺字段 → gRPC InvalidArgument("F_001:...")
//                   重复 → 幂等覆盖（accepted:true，无错误码）
//   - Discover:    Registry.Discover；空 capability → InvalidArgument("F_003:...")
//   - SendMessage: 校验 from/to 都已注册（F_005/F_004 走 MessageResponse.Error）
//   - Stream:      每条进来的消息 swap from/to + type="event" + 清空 signature 后回传
//                  （MVP echo；真实路由由 Sprint 5.5+ adapter 替换）
package a2asrv

import (
	"context"
	"io"
	"log"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service 持有 Registry，对外提供 A2AGatewayServer。
type Service struct {
	a2av1.UnimplementedA2AGatewayServer
	reg *Registry
}

// NewService 构造 service。
func NewService(reg *Registry) *Service {
	return &Service{reg: reg}
}

// RegisterCard 注册 / 覆盖 AgentCard。
// 缺字段 → gRPC status InvalidArgument("F_001:agent_id and name required")
// 重复注册 → 幂等覆盖，accepted:true，gRPC OK（不暴露 F_002）
func (s *Service) RegisterCard(ctx context.Context, card *a2av1.AgentCard) (*a2av1.RegisterResponse, error) {
	if card == nil || card.GetAgentId() == "" || card.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "F_001:agent_id and name required")
	}
	s.reg.Register(card) // errCode=F_002 时已覆盖；MVP 不向调用方暴露
	return &a2av1.RegisterResponse{
		Accepted: true,
		CardId:   card.GetAgentId(),
	}, nil
}

// Discover 联邦发现。capability 空 → InvalidArgument("F_003:capability required")。
func (s *Service) Discover(ctx context.Context, req *a2av1.DiscoverRequest) (*a2av1.DiscoverResponse, error) {
	if req.GetCapability() == "" {
		return nil, status.Error(codes.InvalidArgument, "F_003:capability required")
	}
	cards, _ := s.reg.Discover(req.GetCapability(), req.GetCityFilter())
	return &a2av1.DiscoverResponse{Cards: cards}, nil
}

// SendMessage 投递消息（MVP 不消费 Inbox，仅校验双方已注册）。
// 错误码走 MessageResponse.Error：
//   - 收件方未注册 → Delivered=false, Error="F_004:recipient not found"
//   - 发件方未注册 → Delivered=false, Error="F_005:sender not registered"
//   - 成功 → Delivered=true, Error=""
func (s *Service) SendMessage(ctx context.Context, msg *a2av1.Message) (*a2av1.MessageResponse, error) {
	if msg == nil {
		return &a2av1.MessageResponse{Delivered: false, Error: "F_005:empty message"}, nil
	}
	if _, ok := s.reg.Get(msg.GetToAgentId()); !ok {
		return &a2av1.MessageResponse{Delivered: false, Error: "F_004:recipient not found"}, nil
	}
	if _, ok := s.reg.Get(msg.GetFromAgentId()); !ok {
		return &a2av1.MessageResponse{Delivered: false, Error: "F_005:sender not registered"}, nil
	}
	// MVP 不做 inbox 持久化 / 不路由 —— 仅校验 + 返成功
	log.Printf("[a2asrv] SendMessage %s → %s (msg=%s conv=%s, %d bytes payload)",
		msg.GetFromAgentId(), msg.GetToAgentId(), msg.GetMessageId(),
		msg.GetConversationId(), len(msg.GetPayload()))
	return &a2av1.MessageResponse{Delivered: true}, nil
}

// Stream 双向流 echo：每条进来的消息 swap from/to、type→event、signature 清空后回传。
// 入站流关闭后，服务端也关闭出站流。
func (s *Service) Stream(stream a2av1.A2AGateway_StreamServer) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		echo := &a2av1.Message{
			MessageId:      msg.GetMessageId(),
			FromAgentId:    msg.GetToAgentId(),
			ToAgentId:      msg.GetFromAgentId(),
			ConversationId: msg.GetConversationId(),
			Type:           "event",
			Payload:        msg.GetPayload(),
			TsMs:           msg.GetTsMs(),
			TraceId:        msg.GetTraceId(),
			Signature:      "", // MVP 不验证签名；清空避免误用
		}
		if err := stream.Send(echo); err != nil {
			return err
		}
	}
}
