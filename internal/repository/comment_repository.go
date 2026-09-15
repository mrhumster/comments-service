//go:generate mockgen -source=comment_repository.go -destination=mock/comment_repository_mock.go -package=mock
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/comments-service/internal/domain/models"
)

const (
	// MaxListLimit caps both top-level and reply page sizes.
	MaxListLimit = 100
)

// CommentRepository persists comments in Postgres. Read queries return
// CommentView (with reply count); write paths operate on the base model.
type CommentRepository interface {
	Create(ctx context.Context, comment *models.Comment) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Comment, error)
	// ListByStream returns top-level comments (parent_id IS NULL) newest
	// first, keyset-before (created_at, id).
	ListByStream(ctx context.Context, streamID uuid.UUID, limit int, beforeCreatedAt *time.Time, beforeID *uuid.UUID) ([]*models.CommentView, error)
	// ListReplies returns direct children of parentID oldest first,
	// keyset-after (created_at, id).
	ListReplies(ctx context.Context, parentID uuid.UUID, limit int, afterCreatedAt *time.Time, afterID *uuid.UUID) ([]*models.CommentView, error)
	UpdateBody(ctx context.Context, id uuid.UUID, body string, now time.Time) (*models.Comment, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
