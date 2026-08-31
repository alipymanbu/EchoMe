package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justin/echome-be/internal/domain"
	"github.com/justin/echome-be/internal/domain/storage"
	"github.com/labstack/echo/v4"
)

const (
	defaultMaxUploadSize = 10 * 1024 * 1024
	maxSniffSize         = 512
)

var allowedUploadTypes = map[string]string{
	"image/jpeg":      "jpg",
	"image/png":       "png",
	"image/gif":       "gif",
	"image/webp":      "webp",
	"application/pdf": "pdf",
	"audio/mpeg":      "mp3",
	"audio/wav":       "wav",
	"audio/x-wav":     "wav",
	"audio/ogg":       "ogg",
	"audio/webm":      "webm",
	"audio/mp4":       "m4a",
}

type FileHandlers struct {
	objectStorage storage.ObjectStorage
	maxUploadSize int64
}

type FileUploadResponse struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

func NewFileHandlers(objectStorage storage.ObjectStorage, maxUploadSize int64) *FileHandlers {
	if maxUploadSize <= 0 {
		maxUploadSize = defaultMaxUploadSize
	}
	return &FileHandlers{objectStorage: objectStorage, maxUploadSize: maxUploadSize}
}

func (h *FileHandlers) RegisterRoutes(e *echo.Echo) {
	e.POST("/api/files", h.Upload)
}

// Upload receives a file, stores it in S3, and returns a short-lived read URL.
func (h *FileHandlers) Upload(c echo.Context) error {
	if h.objectStorage == nil {
		return domain.Error(c, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Object storage is not configured")
	}

	// Leave enough room for multipart headers while preventing an oversized
	// request from being fully buffered by the multipart parser.
	maxRequestSize := h.maxUploadSize + 1024*1024
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxRequestSize)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			return domain.Error(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "File is too large")
		}
		return domain.BadRequest(c, "A file is required")
	}
	if fileHeader.Size <= 0 {
		return domain.BadRequest(c, "File cannot be empty")
	}
	if fileHeader.Size > h.maxUploadSize {
		return domain.Error(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "File is too large")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return domain.InternalError(c, "Failed to open uploaded file")
	}
	defer file.Close()

	sniff := make([]byte, maxSniffSize)
	readSize, readErr := io.ReadFull(file, sniff)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return domain.BadRequest(c, "Failed to read uploaded file")
	}
	contentType := http.DetectContentType(sniff[:readSize])
	extension, ok := allowedUploadTypes[contentType]
	if !ok {
		return domain.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE_TYPE", "File type is not supported")
	}

	now := time.Now().UTC()
	key := fmt.Sprintf("files/%s/%s/%s.%s", now.Format("2006"), now.Format("01"), uuid.NewString(), extension)
	body := io.MultiReader(bytes.NewReader(sniff[:readSize]), file)
	if err := h.objectStorage.PutObject(c.Request().Context(), key, body, fileHeader.Size, contentType); err != nil {
		return domain.Error(c, http.StatusBadGateway, "STORAGE_ERROR", "Failed to store file")
	}

	url, err := h.objectStorage.PresignGetObject(c.Request().Context(), key, 0)
	if err != nil {
		_ = h.objectStorage.DeleteObject(c.Request().Context(), key)
		return domain.Error(c, http.StatusBadGateway, "STORAGE_ERROR", "Failed to create file URL")
	}

	return domain.Success(c, FileUploadResponse{
		Key:         key,
		URL:         url,
		ContentType: contentType,
		Size:        fileHeader.Size,
	})
}
