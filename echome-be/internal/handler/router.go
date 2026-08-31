package handler

import (
	"github.com/justin/echome-be/internal/domain/ai"
	"github.com/justin/echome-be/internal/domain/character"
	"github.com/justin/echome-be/internal/domain/conversation"
	"github.com/justin/echome-be/internal/domain/storage"
	"github.com/labstack/echo/v4"
)

type Router struct {
	characterHandlers *CharacterHandlers
	webSocketHandlers *WebSocketHandlers
	fileHandlers      *FileHandlers
}

// NewRouter 创建路由
func NewRouter(
	characterService *character.CharacterService,
	aiClient ai.Repo,
	conversationService *conversation.ConversationService,
	objectStorage storage.ObjectStorage,
	maxUploadSize int64,
) *Router {
	return &Router{
		characterHandlers: NewCharacterHandlers(characterService),
		webSocketHandlers: NewWebSocketHandlers(aiClient, conversationService),
		fileHandlers:      NewFileHandlers(objectStorage, maxUploadSize),
	}
}

// RegisterAllRoutes 注册所有路由
func (r *Router) RegisterAllRoutes(e *echo.Echo) {
	// 注册角色路由
	r.characterHandlers.RegisterRoutes(e)

	// 注册 WebSocket 路由
	r.webSocketHandlers.RegisterRoutes(e)
	r.fileHandlers.RegisterRoutes(e)
}

// GetCharacterService 获取角色服务
func (r *Router) GetCharacterService() *character.CharacterService {
	return r.characterHandlers.characterService
}
