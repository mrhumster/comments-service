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
	"github.com/mrhumster/comments-service/internal/stream"
	"gorm.io/gorm"
)

const (
	EventCommentCreated = "comment.created"
	EventCommentReplied = "comment.replied"

	activitySnippetRunes = 80
)

type CommentsServiceImpl struct {
	repo         repository.CommentRepository
	recorder     queue.ActivityEventRecorder
	streamStatus stream.StatusClient
}

func NewCommentsServiceImpl(repo repository.CommentRepository) *CommentsServiceImpl {
	return &CommentsServiceImpl{repo: repo}
}

func (s *CommentsServiceImpl) WithActivityRecorder(r queue.ActivityEventRecorder) {
	s.recorder = r
}

func (s *CommentsServiceImpl) WithStreamStatusClient(c stream.StatusClient) {
	s.streamStatus = c
}

func (s *CommentsServiceImpl) Create(ctx context.Context, actor Actor, streamID uuid.UUID, parentID *uuid.UUID, rawBody string) (*models.Comment, error) {
	if actor.Role != "admin" && !actor.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	if err := s.checkStreamCommentable(ctx, streamID); err != nil {
		return nil, err
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
		// A reply must stay on the same stream as its parent — otherwise an
		// attacker could leak/attach content across unrelated streams.
		if parent.StreamID != streamID {
			return nil, ErrInvalidParent
		}
	}

	now := time.Now().UTC()
	comment := &models.Comment{
		ID:        uuid.New(),
		StreamID:  streamID,
		UserID:    actor.UserID,
		UserEmail: actor.Email,
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

// checkStreamCommentable fails closed: no client configured, lookup errors or
// a non-published/private stream all reject the comment.
func (s *CommentsServiceImpl) checkStreamCommentable(ctx context.Context, streamID uuid.UUID) error {
	if s.streamStatus == nil {
		return ErrStreamUnavailable
	}
	info, err := s.streamStatus.Status(ctx, streamID)
	if err != nil {
		if errors.Is(err, stream.ErrStreamNotFound) {
			return ErrStreamNotFound
		}
		slog.Error("check stream status", "error", err)
		return ErrStreamUnavailable
	}
	if !info.Commentable() {
		return ErrStreamNotPublished
	}
	return nil
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
