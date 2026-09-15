package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/comments-service/internal/domain/models"
	"github.com/mrhumster/comments-service/internal/queue"
	"github.com/mrhumster/comments-service/internal/repository"
	"github.com/mrhumster/comments-service/internal/sanitize"
	"gorm.io/gorm"
)

const (
	EventCommentCreated = "comment.created"
	EventCommentReplied = "comment.replied"

	activitySnippetRunes = 80
)

type CommentsServiceImpl struct {
	repo     repository.CommentRepository
	recorder queue.ActivityEventRecorder
}

func NewCommentsServiceImpl(repo repository.CommentRepository) *CommentsServiceImpl {
	return &CommentsServiceImpl{repo: repo}
}

func (s *CommentsServiceImpl) WithActivityRecorder(r queue.ActivityEventRecorder) {
	s.recorder = r
}

func (s *CommentsServiceImpl) Create(ctx context.Context, actor Actor, streamID uuid.UUID, parentID *uuid.UUID, rawBody string) (*models.Comment, error) {
	if actor.Role != "admin" && !actor.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	body := sanitize.Body(rawBody)
	if body == "" {
		return nil, ErrEmptyBody
	}

	if parentID != nil {
		parent, err := s.repo.GetByID(ctx, *parentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrCommentNotFound
			}
			return nil, err
		}
		// Two-level threading: replies attach to top-level comments only.
		if parent.ParentID != nil {
			return nil, ErrInvalidParent
		}
	}

	now := time.Now().UTC()
	comment := &models.Comment{
		ID:        uuid.New(),
		StreamID:  streamID,
		UserID:    actor.UserID,
		ParentID:  parentID,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, comment); err != nil {
		return nil, err
	}

	eventType := EventCommentCreated
	if parentID != nil {
		eventType = EventCommentReplied
	}
	s.recordEvent(ctx, comment, eventType)

	return comment, nil
}

func (s *CommentsServiceImpl) ListTop(ctx context.Context, streamID uuid.UUID, limit int, cursor *Cursor) ([]*models.CommentView, *Cursor, error) {
	var beforeCreatedAt *time.Time
	var beforeID *uuid.UUID
	if cursor != nil {
		beforeCreatedAt = &cursor.CreatedAt
		beforeID = &cursor.ID
	}

	items, err := s.repo.ListByStream(ctx, streamID, limit, beforeCreatedAt, beforeID)
	if err != nil {
		return nil, nil, err
	}
	return items, nextCursor(items, limit), nil
}

func (s *CommentsServiceImpl) ListReplies(ctx context.Context, parentID uuid.UUID, limit int, cursor *Cursor) ([]*models.CommentView, *Cursor, error) {
	var afterCreatedAt *time.Time
	var afterID *uuid.UUID
	if cursor != nil {
		afterCreatedAt = &cursor.CreatedAt
		afterID = &cursor.ID
	}

	items, err := s.repo.ListReplies(ctx, parentID, limit, afterCreatedAt, afterID)
	if err != nil {
		return nil, nil, err
	}
	return items, nextCursor(items, limit), nil
}

func (s *CommentsServiceImpl) Update(ctx context.Context, actor Actor, commentID uuid.UUID, rawBody string) (*models.Comment, error) {
	body := sanitize.Body(rawBody)
	if body == "" {
		return nil, ErrEmptyBody
	}

	comment, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotFound
		}
		return nil, err
	}
	if comment.UserID != actor.UserID && actor.Role != "admin" {
		return nil, ErrCommentForbidden
	}

	return s.repo.UpdateBody(ctx, commentID, body, time.Now().UTC())
}

func (s *CommentsServiceImpl) Delete(ctx context.Context, actor Actor, commentID uuid.UUID) error {
	comment, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	if comment.UserID != actor.UserID && actor.Role != "admin" {
		return ErrCommentForbidden
	}
	return s.repo.Delete(ctx, commentID)
}

func nextCursor(items []*models.CommentView, limit int) *Cursor {
	if len(items) == 0 || len(items) < limit {
		return nil
	}
	last := items[len(items)-1]
	return &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
}

func (s *CommentsServiceImpl) recordEvent(ctx context.Context, comment *models.Comment, eventType string) {
	if s.recorder == nil {
		return
	}
	payload := map[string]any{
		"comment_id": comment.ID,
		"parent_id":  comment.ParentID,
	}
	if snippet := sanitize.Snippet(comment.Body, activitySnippetRunes); snippet != "" {
		payload["snippet"] = snippet
	}
	if err := s.recorder.RecordActivityEvent(ctx, comment.UserID, eventType, &comment.StreamID, payload); err != nil {
		slog.Warn("record comment activity event", "error", err)
	}
}
