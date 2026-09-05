// Service 实现 A2AGateway 4 个 RPC（docs/06-A2A协议.md §20）。
//
// Sprint 5.5 行为：
//   - RegisterCard: 调 Registry.Register；
//                   缺字段 → gRPC InvalidArgument("F_001:...")；
//                   auth["ed25519"] 解析失败 → gRPC InvalidArgument("F_006:...")；
//                   重复 → 幂等覆盖（accepted:true，无错误码）。
//   - Discover:    Registry.Discover；空 capability → InvalidArgument("F_003:...")。
//   - SendMessage: 校验 from/to 都已注册（F_005/F_004 走 MessageResponse.Error）；
//                   verifier.Verify 发件方签名（F_007/F_008 走 MessageResponse.Error）；
//                   dispatcher.Deliver 路由（F_009 走 MessageResponse.Error）。
//   - Stream:      每条进来的消息同样走 verifier + dispatcher；
//                  任何验签 / 时间窗 / 路由失败 → gRPC Unauthenticated + 关流。
package a2asrv

import (
	"context"
	"io"
	"log"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service 持有 Registry + Verifier + Dispatcher，对外提供 A2AGatewayServer。
type Service struct {
	a2av1.UnimplementedA2AGatewayServer
	reg        *Registry
	verifier   *Verifier
	dispatcher *Dispatcher
}

// NewService 构造 service。
//   - verifier nil → NewVerifier(0)（5min 默认窗口）
//   - dispatcher nil → NewDispatcher()（无 adapter，路由必返 F_009；调用方应 Register）
func NewService(reg *Registry, verifier *Verifier, dispatcher *Dispatcher) *Service {
	if verifier == nil {
		verifier = NewVerifier(0)
	}
	if dispatcher == nil {
		dispatcher = NewDispatcher()
	}
	return &Service{reg: reg, verifier: verifier, dispatcher: dispatcher}
}

// RegisterCard 注册 / 覆盖 AgentCard。
//   - 缺字段 → gRPC status InvalidArgument("F_001:agent_id and name required")
//   - auth["ed25519"] 解析失败 → gRPC status InvalidArgument("F_006:invalid ed25519 public key")
//   - 重复 → 幂等覆盖（accepted:true，gRPC OK，不暴露 F_002）
func (s *Service) RegisterCard(ctx context.Context, card *a2av1.AgentCard) (*a2av1.RegisterResponse, error) {
	if card == nil || card.GetAgentId() == "" || card.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "F_001:agent_id and name required")
	}
	// 提前用 registry 的同款校验拿到 F_006 —— 保持 service / registry 行为一致。
	// 直接调 Register：errCode=F_002 时已覆盖，service 不向调用方暴露 F_002。
	if ok, errCode := s.reg.Register(card); !ok {
		switch errCode {
		case "F_001":
			return nil, status.Error(codes.InvalidArgument, "F_001:agent_id and name required")
		case "F_006":
			return nil, status.Error(codes.InvalidArgument, "F_006:invalid ed25519 public key")
		default:
			return nil, status.Error(codes.InvalidArgument, errCode+":register rejected")
		}
	}
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

// SendMessage 投递消息。
// 错误码走 MessageResponse.Error：
//   - 收件方未注册 → "F_004:recipient not found"
//   - 发件方未注册 → "F_005:sender not registered"
//   - 签名缺失/坏/验签失败 → "F_007:..."
//   - ts_ms 出窗 → "F_008:ts_ms out of window"
//   - 路由无 adapter → "F_009:unknown provider"
//   - 成功 → delivered:true, error=""
func (s *Service) SendMessage(ctx context.Context, msg *a2av1.Message) (*a2av1.MessageResponse, error) {
	if msg == nil {
		return &a2av1.MessageResponse{Delivered: false, Error: "F_005:empty message"}, nil
	}
	recipient, ok := s.reg.Get(msg.GetToAgentId())
	if !ok {
		return &a2av1.MessageResponse{Delivered: false, Error: "F_004:recipient not found"}, nil
	}
	sender, ok := s.reg.Get(msg.GetFromAgentId())
	if !ok {
		return &a2av1.MessageResponse{Delivered: false, Error: "F_005:sender not registered"}, nil
	}
	if err := s.verifier.Verify(sender, msg, time.Now()); err != nil {
		return &a2av1.MessageResponse{Delivered: false, Error: err.Error()}, nil
	}
	if _, err := s.dispatcher.Deliver(ctx, recipient, msg); err != nil {
		return &a2av1.MessageResponse{Delivered: false, Error: err.Error()}, nil
	}
	log.Printf("[a2asrv] SendMessage %s → %s (msg=%s conv=%s, %d bytes payload, provider=%s)",
		msg.GetFromAgentId(), msg.GetToAgentId(), msg.GetMessageId(),
		msg.GetConversationId(), len(msg.GetPayload()), recipient.GetProvider())
	return &a2av1.MessageResponse{Delivered: true}, nil
}

// Stream 双向流：每条进来的消息走 verifier + dispatcher，
// 失败 → gRPC Unauthenticated + 关闭流；成功 → adapter.Deliver 返回值 swap 回传。
func (s *Service) Stream(stream a2av1.A2AGateway_StreamServer) error {
	ctx := stream.Context()
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		recipient, ok := s.reg.Get(msg.GetToAgentId())
		if !ok {
			return status.Error(codes.Unauthenticated, "F_004:recipient not found")
		}
		sender, ok := s.reg.Get(msg.GetFromAgentId())
		if !ok {
			return status.Error(codes.Unauthenticated, "F_005:sender not registered")
		}
		if err := s.verifier.Verify(sender, msg, time.Now()); err != nil {
			// F_007 / F_008 都映射到 Unauthenticated（流无法塞 MessageResponse.Error）
			return status.Error(codes.Unauthenticated, err.Error())
		}
		reply, err := s.dispatcher.Deliver(ctx, recipient, msg)
		if err != nil {
			return status.Error(codes.Unauthenticated, err.Error()) // F_009
		}
		// EchoAdapter 返非 nil reply；stub 返 nil → 跳过回传（fire-and-forget）
		if reply == nil {
			continue
		}
		if err := stream.Send(reply); err != nil {
			return err
		}
	}
}
