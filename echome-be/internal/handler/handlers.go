package handler

import (
	"github.com/justin/echome-be/internal/domain/ai"
	"github.com/justin/echome-be/internal/domain/character"
	"github.com/justin/echome-be/internal/domain/conversation"
	"github.com/justin/echome-be/internal/domain/storage"
	"github.com/labstack/echo/v4"
)

type Handlers struct {
	router *Router
}

// NewHandlers
func NewHandlers(characterService *character.CharacterService, aiService ai.Repo, conversationService *conversation.ConversationService, objectStorage storage.ObjectStorage, maxUploadSize int64) *Handlers {
	router := NewRouter(characterService, aiService, conversationService, objectStorage, maxUploadSize)
	return &Handlers{
		router: router,
	}
}

// RegisterRoutes
func (h *Handlers) RegisterRoutes(e *echo.Echo) {
	h.router.RegisterAllRoutes(e)
}

// GetRouter
func (h *Handlers) GetRouter() *Router {
	return h.router
}
