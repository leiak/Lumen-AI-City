class AgentOSError(Exception):
    code: str = "R_000"
    http_status: int = 500
    message: str = "unknown"


class R001LLMTimeout(AgentOSError):
    code = "R_001"; http_status = 504; message = "LLM 调用超时"


class R002LLMRateLimit(AgentOSError):
    code = "R_002"; http_status = 429; message = "LLM 限流"


class R003TokenCapExceeded(AgentOSError):
    code = "R_003"; http_status = 400; message = "token 超上限"


class R004ChatTurnExhausted(AgentOSError):
    code = "R_004"; http_status = 400; message = "对话已达 6 回合上限"


class R005NPCNotFound(AgentOSError):
    code = "R_005"; http_status = 404; message = "NPC 未找到"


class R006MilvusUnavailable(AgentOSError):
    code = "R_006"; http_status = 503; message = "Milvus 不可用"


class R007SagaTimeout(AgentOSError):
    code = "R_007"; http_status = 504; message = "Saga 步骤超时"


class R008CrossCityRefused(AgentOSError):
    code = "R_008"; http_status = 403; message = "跨城联邦被拒绝"


class R009InvalidInput(AgentOSError):
    code = "R_009"; http_status = 400; message = "参数非法"


class R010UpstreamDown(AgentOSError):
    code = "R_010"; http_status = 502; message = "上游服务不可用"


class R011LLMStreamFail(AgentOSError):
    code = "R_011"; http_status = 502; message = "LLM stream() 中途报错"


class R012LLMStreamTimeout(AgentOSError):
    code = "R_012"; http_status = 502; message = "LLM 单句生成超时（> 8s）"


class R013EmotionParseFail(AgentOSError):
    code = "R_013"; http_status = 400; message = "emotion 标签不在 8 类内"


class R014RedisPublishFail(AgentOSError):
    code = "R_014"; http_status = 502; message = "Redis 发布节拍包失败"


class R015SessionNotFound(AgentOSError):
    code = "R_015"; http_status = 400; message = "session_id 过期或不存在"
