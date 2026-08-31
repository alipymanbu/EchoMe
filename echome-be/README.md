# EchoMe Backend

这是EchoMe项目的后端代码，一个利用AI进行角色扮演的应用程序。后端基于Go语言实现，使用DDD(领域驱动设计)架构，并使用Wire进行依赖注入。

## 项目结构

项目遵循DDD架构，主要包含以下目录：

- `cmd/api`: 应用程序入口点
- `config`: 配置相关代码
- `internal/app`: 应用程序核心逻辑和依赖注入
- `internal/domain`: 领域模型和接口定义
- `internal/usecase`: 用例层，实现业务逻辑
- `internal/infrastructure`: 基础设施层，实现数据存储等
- `internal/interfaces`: 接口层，处理HTTP请求
- `client`: 第三方服务客户端

## 技术栈

- Go 1.21+
- Echo: HTTP路由框架
- Gorilla WebSocket: WebSocket支持
- Wire: 依赖注入
- koanf: 配置管理
- 阿里云百炼API: AI对话生成

## 安装依赖

```bash
# 安装项目依赖
go mod tidy
```

## 生成依赖注入代码

项目使用Wire进行依赖注入，需要生成依赖注入代码：

```bash
# 在项目根目录执行
make wire
```

## 运行项目

```bash
make run
```

## 配置

项目使用koanf进行配置管理，配置文件格式为YAML。默认配置文件路径为`config/etc/config.yaml`。

### 主要配置项

#### 服务器配置
- `server.port`: 服务器端口

#### AI服务配置

ASR、TTS、LLM 在 `ai` 下独立选择提供商和能力参数，提供商的密钥与端点统一放在 `providers` 下：

```yaml
ai:
  timeout: 30
  max_retries: 3
  asr:
    provider: "aliyun"
    model: "paraformer-realtime-v2"
    sample_rate: 16000
    format: "pcm"
    language_hints: ["zh", "en"]
  tts:
    provider: "mimo"
    model: "mimo-v2.5-tts"
    voice: "mimo_default"
    sample_rate: 24000
    format: "pcm16"
    min_segment_runes: 12
    max_segment_runes: 100
    max_segment_wait_ms: 800
  llm:
    provider: "aliyun"
    model: "qwen-turbo"
    temperature: 0.7
    max_tokens: 2000

providers:
  aliyun:
    api_key: "your-alibailian-api-key"
    endpoint: "https://dashscope.aliyuncs.com"
    region: "cn-beijing"
  mimo:
    api_key: "your-mimo-api-key"
    endpoint: "https://api.xiaomimimo.com/v1"
```

当前支持：阿里云和 MiMo 的 ASR/LLM，以及阿里云和 MiMo 的 TTS；三项能力可以独立切换。切换某项能力只修改对应的 `ai.<capability>.provider`。

MiMo ASR 当前会将前端 WebSocket 上传的 PCM 音频缓存到连接结束，封装为 16-bit 单声道 WAV 后调用 MiMo 识别，因此结果在一次语音片段结束后返回，不是逐字实时结果。

当客户端请求中的 `enable_search` 为 `true` 时，后端会向当前 LLM 发送标准 OpenAI-compatible function tool `tavily_search`。模型决定调用工具后，后端执行 Tavily 搜索，并用标准的 `assistant.tool_calls` + `tool` 消息继续请求 LLM，最终回复仍按原有文本流返回。MiMo 和阿里云兼容模式均支持这套流程；需要在 `tavily.api_key` 中配置密钥。

#### WebRTC配置
- `webrtc.stun_server`: STUN服务器地址

#### S3兼容对象存储配置

后端提供通用的 S3 兼容对象存储服务，支持 AWS S3、MinIO、阿里云 OSS、Cloudflare R2 等。配置写在 `config/etc/config.yaml` 的 `s3` 节点：

```yaml
s3:
  endpoint: "https://s3.example.com"
  region: "us-east-1"
  bucket: "echome"
  access_key_id: "your-access-key-id"
  secret_access_key: "your-secret-access-key"
  session_token: ""
  force_path_style: false
  presign_expiry_mins: 15
```

`endpoint` 对 AWS S3 可以留空；MinIO 或其他兼容服务通常需要填写，并按服务要求设置 `force_path_style`。密钥也可以省略，SDK 会使用环境变量、工作负载身份或实例角色等默认凭据链。当前服务已提供上传、下载、删除和预签名 URL 能力，业务代码通过 `internal/domain/storage.ObjectStorage` 接口使用。

文件上传接口：

```http
POST /api/files
Content-Type: multipart/form-data
```

表单字段为 `file`。服务端会生成对象 Key，校验真实文件类型和文件大小（默认 10 MiB），并返回包含 `key`、临时 `url`、`content_type` 和 `size` 的标准 API 响应。前端通过 `/v1/api/files` 调用该接口。

## API端点

### 角色相关
- `GET /api/characters`: 获取所有角色
- `GET /api/characters/{id}`: 获取单个角色
- `POST /api/character`: 创建角色（语音克隆并创建角色）

### 会话相关
- `POST /api/sessions`: 创建会话
- `GET /api/sessions?userId={userId}`: 获取用户的所有会话
- `GET /api/sessions/{id}`: 获取单个会话
- `GET /api/sessions/{id}/messages`: 获取会话中的所有消息
- `POST /api/sessions/{id}/messages`: 发送消息

### WebSocket端点
- `GET /ws/asr`: 语音识别WebSocket连接
- `GET /ws/tts`: 文本转语音WebSocket连接
- `GET /ws/webrtc/{sessionId}/{userId}`: WebRTC信令WebSocket连接
- `GET /ws/voice-conversation/{sessionId}/{characterId}`

### 系统端点
- `GET /health`: 健康检查端点，返回系统状态和可用服务信息
- `GET /swagger/*`: API文档（Swagger UI）

## 功能说明

### 角色创建功能

主要特点：

1. 通过语音克隆创建角色，自动设置角色ID为语音ID
2. 角色信息与数据库保持一致，包含ID、Name、Prompt、Avatar、CreatedAt和UpdatedAt字段
3. 创建角色的API需要传入：
   - `audio`（可选，当需要自定义音色时必须）
   - `name`（必须，角色名称）
   - `prompt`（必须，角色提示词）
   - `avatar`（可选，角色头像）
   - `flag`（必须，布尔值，标识是否需要自定义音色）

### 聊天功能

AI对话生成功能进行了优化，现在具有以下特点：

1. 当角色上下文为空时，会使用默认提示词："你是一个友好、专业的AI助手，会用自然的方式回答用户的问题。"
2. 无论是同步还是流式响应模式，都会应用相同的角色上下文处理逻辑

### AI服务集成

项目支持两种AI服务提供商：

1. **阿里云百炼API**：通过`client/aliyun_bl.go`实现
实现了`domain.AIService`接口，通过工厂模式（`client.NewAIServiceFromConfig`）根据配置动态选择使用哪种服务。

### 会话管理

- 用户可以创建多个与不同角色的会话
- 每个会话包含多条消息
- 发送消息后，系统会自动生成AI回复

### WebRTC支持

项目提供WebRTC信令服务，支持实时音视频通信功能。

## 启动验证

应用程序启动时会自动进行以下验证：

1. **配置验证**：检查所有必需的配置项是否正确设置
2. **服务验证**：验证所有依赖服务是否正确初始化
3. **路由验证**：确认所有必需的API端点和WebSocket端点已注册

启动成功后，可以通过以下方式验证系统状态：

```bash
# 检查健康状态
curl http://localhost:8081/health

# 访问API文档
open http://localhost:8081/swagger/
```

## 开发注意事项

1. **依赖注入**：修改依赖关系后，需要重新生成依赖注入代码

2. **内存存储**：当前项目使用内存存储数据，重启服务后数据会丢失

3. **AI服务配置**：使用前需要配置对应AI服务的API密钥和相关参数

4. **服务验证**：应用程序启动时会自动验证所有服务的配置和可用性

5. **安全的角色仓库查询**：角色仓库查询功能已实现安全访问，包括：
   - 正确处理数据库模型与领域模型的转换
   - 完善的空指针检查和错误处理
   - 使用上下文参数进行安全的数据库操作
   - 正确解析JSON字段并处理异常情况

## Swagger文档使用

项目已集成Swagger文档，用于方便地查看和测试API。

```bash
# 安装swag CLI工具（如果尚未安装）
go install github.com/swaggo/swag/cmd/swag@latest

# 在项目根目录执行命令生成最新文档
make swag
```

这将更新docs目录下的`docs.go`、`swagger.json`和`swagger.yaml`文件，确保Swagger文档与实际API保持一致。
