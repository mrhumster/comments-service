package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/comments-service/internal/domain/models"
	"gorm.io/gorm"
)

type GormCommentRepository struct {
	db *gorm.DB
}

func NewGormCommentRepository(db *gorm.DB) *GormCommentRepository {
	return &GormCommentRepository{db: db}
}

func (r *GormCommentRepository) Create(ctx context.Context, comment *models.Comment) error {
	return r.db.WithContext(ctx).Create(comment).Error
}

func (r *GormCommentRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Comment, error) {
	var c models.Comment
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

const replyCountSelect = "comments.*, (SELECT COUNT(*) FROM comments AS r WHERE r.parent_id = comments.id) AS reply_count"

func (r *GormCommentRepository) ListByStream(ctx context.Context, streamID uuid.UUID, limit int, beforeCreatedAt *time.Time, beforeID *uuid.UUID) ([]*models.CommentView, error) {
	limit = clampLimit(limit)
	q := r.db.WithContext(ctx).
		Select(replyCountSelect).
		Where("comments.stream_id = ? AND comments.parent_id IS NULL", streamID)

	if beforeCreatedAt != nil && beforeID != nil {
		q = q.Where("(comments.created_at < ?) OR (comments.created_at = ? AND comments.id < ?)",
			*beforeCreatedAt, *beforeCreatedAt, *beforeID)
	}

	var out []*models.CommentView
	err := q.Order("comments.created_at DESC, comments.id DESC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *GormCommentRepository) ListReplies(ctx context.Context, parentID uuid.UUID, limit int, afterCreatedAt *time.Time, afterID *uuid.UUID) ([]*models.CommentView, error) {
	limit = clampLimit(limit)
	q := r.db.WithContext(ctx).
		Select(replyCountSelect).
		Where("comments.parent_id = ?", parentID)

	if afterCreatedAt != nil && afterID != nil {
		q = q.Where("(comments.created_at > ?) OR (comments.created_at = ? AND comments.id > ?)",
			*afterCreatedAt, *afterCreatedAt, *afterID)
	}

	var out []*models.CommentView
	err := q.Order("comments.created_at ASC, comments.id ASC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *GormCommentRepository) UpdateBody(ctx context.Context, id uuid.UUID, body string, now time.Time) (*models.Comment, error) {
	err := r.db.WithContext(ctx).
		Model(&models.Comment{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"body":       body,
			"edited_at":  now,
			"updated_at": now,
		}).Error
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *GormCommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Comment{}).Error
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
