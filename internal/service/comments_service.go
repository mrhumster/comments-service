//go:generate mockgen -source=comments_service.go -destination=mock/comments_service_mock.go -package=mock
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/comments-service/internal/domain/models"
)

// ListLimitMax caps page sizes for both list endpoints.
const ListLimitMax = 100

var (
	ErrEmailNotVerified = errors.New("email not verified")
	ErrCommentNotFound  = errors.New("comment not found")
	ErrCommentForbidden = errors.New("forbidden")
	ErrInvalidParent    = errors.New("invalid parent comment")
	ErrEmptyBody        = errors.New("empty body")
	ErrStreamNotFound   = errors.New("stream not found")
	ErrStreamNotPublished = errors.New("stream is not published")
	ErrStreamUnavailable  = errors.New("stream service unavailable")
)

// Actor is the authenticated caller: user identity plus JWT claims used by
// the write guards (verified email, admin bypass for every write).
type Actor struct {
	UserID        uuid.UUID
	Role          string
	Email         string
	EmailVerified bool
}

// Cursor is a keyset position on (created_at, id). The handler encodes and
// decodes it for the HTTP layer; service/repo consume the typed struct.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type CommentsService interface {
	Create(ctx context.Context, actor Actor, streamID uuid.UUID, parentID *uuid.UUID, rawBody string) (*models.Comment, error)
	// ListTop returns top-level comments newest first.
	ListTop(ctx context.Context, streamID uuid.UUID, limit int, cursor *Cursor) ([]*models.CommentView, *Cursor, error)
	// ListReplies returns direct children (oldest first).
	ListReplies(ctx context.Context, parentID uuid.UUID, limit int, cursor *Cursor) ([]*models.CommentView, *Cursor, error)
	Update(ctx context.Context, actor Actor, commentID uuid.UUID, rawBody string) (*models.Comment, error)
	Delete(ctx context.Context, actor Actor, commentID uuid.UUID) error
}
