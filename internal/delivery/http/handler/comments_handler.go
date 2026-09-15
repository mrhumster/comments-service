package handler

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/comments-service/internal/delivery/http/middleware"
	"github.com/mrhumster/comments-service/internal/domain/models"
	"github.com/mrhumster/comments-service/internal/metrics"
	"github.com/mrhumster/comments-service/internal/service"
)

const defaultLimit = 50

type CommentsHandler struct {
	svc service.CommentsService
}

func NewCommentsHandler(svc service.CommentsService) *CommentsHandler {
	return &CommentsHandler{svc: svc}
}

type ListResponse struct {
	Comments   []*models.CommentView `json:"comments"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

type createCommentRequest struct {
	Body     string  `json:"body"`
	ParentID *string `json:"parent_id,omitempty"`
}

type updateCommentRequest struct {
	Body string `json:"body"`
}

// ListTop returns top-level comments for a stream, newest first.
// Public endpoint (stream membership is not checked — the read API mirrors
// the public catalog access; private/unlisted streams rely on their URL).
//   - limit (default 50, max 100)
//   - cursor — base64(RFC3339Nano|id) of the last item's position
func (h *CommentsHandler) ListTop(c *gin.Context) {
	streamID, ok := parseUUID(c, c.Param("streamId"))
	if !ok {
		return
	}

	limit, ok := parseLimit(c)
	if !ok {
		return
	}

	cursor, err := parseCursor(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
		return
	}

	items, next, err := h.svc.ListTop(c.Request.Context(), streamID, limit, cursor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	resp := ListResponse{Comments: items}
	if next != nil {
		resp.NextCursor = encodeCursor(next)
	}
	c.JSON(http.StatusOK, resp)
}

// ListReplies returns direct children of a top-level comment, oldest first.
// Public endpoint.
func (h *CommentsHandler) ListReplies(c *gin.Context) {
	parentID, ok := parseUUID(c, c.Param("id"))
	if !ok {
		return
	}

	limit, ok := parseLimit(c)
	if !ok {
		return
	}

	cursor, err := parseCursor(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
		return
	}

	items, next, err := h.svc.ListReplies(c.Request.Context(), parentID, limit, cursor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	resp := ListResponse{Comments: items}
	if next != nil {
		resp.NextCursor = encodeCursor(next)
	}
	c.JSON(http.StatusOK, resp)
}

// Create adds a comment (or a reply via parent_id) to a stream.
// Requires a verified email (admin bypass), mirrors the CreateStream gate.
func (h *CommentsHandler) Create(c *gin.Context) {
	streamID, ok := parseUUID(c, c.Param("streamId"))
	if !ok {
		return
	}

	var req createCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	var parentID *uuid.UUID
	if req.ParentID != nil {
		pid, err := uuid.Parse(strings.TrimSpace(*req.ParentID))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parent_id"})
			return
		}
		parentID = &pid
	}

	actor := middleware.Actor(c)
	comment, err := h.svc.Create(c.Request.Context(), actor, streamID, parentID, req.Body)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	kind := "comment"
	if parentID != nil {
		kind = "reply"
	}
	metrics.Created(kind)
	c.JSON(http.StatusCreated, gin.H{"comment": comment})
}

// Update edits a comment body (author or admin). Marks the comment as edited.
func (h *CommentsHandler) Update(c *gin.Context) {
	commentID, ok := parseUUID(c, c.Param("id"))
	if !ok {
		return
	}

	var req updateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	comment, err := h.svc.Update(c.Request.Context(), middleware.Actor(c), commentID, req.Body)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	metrics.Updated("success")
	c.JSON(http.StatusOK, gin.H{"comment": comment})
}

// Delete removes a comment (author or admin). Replies are removed via the
// FK ON DELETE CASCADE.
func (h *CommentsHandler) Delete(c *gin.Context) {
	commentID, ok := parseUUID(c, c.Param("id"))
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), middleware.Actor(c), commentID); err != nil {
		writeServiceError(c, err)
		return
	}

	metrics.Deleted("success")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEmailNotVerified):
		c.JSON(http.StatusForbidden, gin.H{"error": "email not verified"})
	case errors.Is(err, service.ErrCommentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
	case errors.Is(err, service.ErrCommentForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case errors.Is(err, service.ErrInvalidParent):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parent comment"})
	case errors.Is(err, service.ErrEmptyBody):
		c.JSON(http.StatusBadRequest, gin.H{"error": "body required"})
	case errors.Is(err, service.ErrStreamNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
	case errors.Is(err, service.ErrStreamNotPublished):
		c.JSON(http.StatusForbidden, gin.H{"error": "stream is not published"})
	case errors.Is(err, service.ErrStreamUnavailable):
		slog.Error("stream service unavailable", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "internal server error"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func parseUUID(c *gin.Context, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return uuid.Nil, false
	}
	return id, true
}

func parseLimit(c *gin.Context) (int, bool) {
	limit := defaultLimit
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return 0, false
		}
		limit = n
	}
	if limit > service.ListLimitMax {
		limit = service.ListLimitMax
	}
	return limit, true
}

func parseCursor(c *gin.Context) (*service.Cursor, error) {
	raw := c.Query("cursor")
	if raw == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed cursor")
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}

	return &service.Cursor{CreatedAt: createdAt, ID: id}, nil
}

func encodeCursor(c *service.Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()))
}
