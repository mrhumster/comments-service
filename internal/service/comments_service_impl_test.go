package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/mrhumster/comments-service/internal/domain/models"
	"github.com/mrhumster/comments-service/internal/queue/mock"
	repomock "github.com/mrhumster/comments-service/internal/repository/mock"
	"github.com/mrhumster/comments-service/internal/stream"
	streammock "github.com/mrhumster/comments-service/internal/stream/mock"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func newTestService(t *testing.T) (*CommentsServiceImpl, *repomock.MockCommentRepository, *mock.MockActivityEventRecorder) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := repomock.NewMockCommentRepository(ctrl)
	rec := mock.NewMockActivityEventRecorder(ctrl)
	svc := NewCommentsServiceImpl(repo)
	svc.WithActivityRecorder(rec)
	sclient := streammock.NewMockStatusClient(ctrl)
	sclient.EXPECT().Status(gomock.Any(), gomock.Any()).
		Return(&stream.StatusInfo{Status: stream.StatusPublished, Visibility: "public"}, nil).AnyTimes()
	svc.WithStreamStatusClient(sclient)
	return svc, repo, rec
}

func newGateTestService(t *testing.T) (*CommentsServiceImpl, *repomock.MockCommentRepository, *mock.MockActivityEventRecorder, *streammock.MockStatusClient) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := repomock.NewMockCommentRepository(ctrl)
	rec := mock.NewMockActivityEventRecorder(ctrl)
	svc := NewCommentsServiceImpl(repo)
	svc.WithActivityRecorder(rec)
	sclient := streammock.NewMockStatusClient(ctrl)
	svc.WithStreamStatusClient(sclient)
	return svc, repo, rec, sclient
}

func verifiedActor() Actor {
	return Actor{UserID: uuid.New(), Role: "member", EmailVerified: true}
}

func TestCreateGates(t *testing.T) {
	t.Run("unverified member rejected", func(t *testing.T) {
		svc, _, rec := newTestService(t)
		_, err := svc.Create(context.Background(), Actor{UserID: uuid.New(), Role: "member", EmailVerified: false}, uuid.New(), nil, "hello")
		require.ErrorIs(t, err, ErrEmailNotVerified)
		_ = rec
	})

	t.Run("admin bypasses gate and empty body", func(t *testing.T) {
		svc, repo, rec := newTestService(t)
		repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
		rec.EXPECT().RecordActivityEvent(gomock.Any(), gomock.Any(), EventCommentCreated, gomock.Any(), gomock.Any()).Return(nil)

		c, err := svc.Create(context.Background(), Actor{UserID: uuid.New(), Role: "admin", EmailVerified: false}, uuid.New(), nil, "**hi** <script>alert(1)</script>")
		require.NoError(t, err)
		require.Equal(t, "**hi**", c.Body)
		require.NotEqual(t, uuid.Nil, c.ID)
	})

	t.Run("empty body after sanitize", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		_, err := svc.Create(context.Background(), verifiedActor(), uuid.New(), nil, "<script>alert(1)</script>")
		require.ErrorIs(t, err, ErrEmptyBody)
	})
}

func TestCreateParentValidation(t *testing.T) {
	parentID := uuid.New()

	t.Run("parent not found", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		repo.EXPECT().GetByID(gomock.Any(), parentID).Return(nil, gorm.ErrRecordNotFound)
		_, err := svc.Create(context.Background(), verifiedActor(), uuid.New(), &parentID, "reply")
		require.ErrorIs(t, err, ErrCommentNotFound)
	})

	t.Run("reply to a reply rejected", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		nestedParent := uuid.New()
		repo.EXPECT().GetByID(gomock.Any(), parentID).Return(&models.Comment{ID: parentID, ParentID: &nestedParent}, nil)
		_, err := svc.Create(context.Background(), verifiedActor(), uuid.New(), &parentID, "reply")
		require.ErrorIs(t, err, ErrInvalidParent)
	})

	t.Run("reply to a parent from another stream rejected", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		streamID := uuid.New()
		otherStreamID := uuid.New()
		require.NotEqual(t, streamID, otherStreamID)
		repo.EXPECT().GetByID(gomock.Any(), parentID).Return(&models.Comment{ID: parentID, StreamID: otherStreamID}, nil)
		_, err := svc.Create(context.Background(), verifiedActor(), streamID, &parentID, "reply")
		require.ErrorIs(t, err, ErrInvalidParent)
	})

	t.Run("replies only within the target stream", func(t *testing.T) {
		svc, repo, rec := newTestService(t)
		streamID := uuid.New()
		repo.EXPECT().GetByID(gomock.Any(), parentID).Return(&models.Comment{ID: parentID, StreamID: streamID}, nil)
		repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
		rec.EXPECT().RecordActivityEvent(gomock.Any(), gomock.Any(), EventCommentReplied, gomock.Any(), gomock.Any()).Return(nil)

		c, err := svc.Create(context.Background(), verifiedActor(), streamID, &parentID, "nice")
		require.NoError(t, err)
		require.Equal(t, parentID, *c.ParentID)
	})

	t.Run("reply ok emits comment.replied", func(t *testing.T) {
		svc, repo, rec := newTestService(t)
		streamID := uuid.New()
		repo.EXPECT().GetByID(gomock.Any(), parentID).Return(&models.Comment{ID: parentID, StreamID: streamID}, nil)
		repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
		rec.EXPECT().RecordActivityEvent(gomock.Any(), gomock.Any(), EventCommentReplied, gomock.Any(), gomock.Any()).Return(nil)

		c, err := svc.Create(context.Background(), verifiedActor(), streamID, &parentID, "nice")
		require.NoError(t, err)
		require.Equal(t, parentID, *c.ParentID)
	})
}

func TestUpdate(t *testing.T) {
	owner := uuid.New()
	commentID := uuid.New()

	t.Run("not found", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		repo.EXPECT().GetByID(gomock.Any(), commentID).Return(nil, gorm.ErrRecordNotFound)
		_, err := svc.Update(context.Background(), verifiedActor(), commentID, "new")
		require.ErrorIs(t, err, ErrCommentNotFound)
	})

	t.Run("foreign user forbidden", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		repo.EXPECT().GetByID(gomock.Any(), commentID).Return(&models.Comment{ID: commentID, UserID: uuid.New()}, nil)
		_, err := svc.Update(context.Background(), verifiedActor(), commentID, "new")
		require.ErrorIs(t, err, ErrCommentForbidden)
	})

	t.Run("author updates own", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		now := time.Now().UTC()
		repo.EXPECT().GetByID(gomock.Any(), commentID).Return(&models.Comment{ID: commentID, UserID: owner}, nil)
		updated := &models.Comment{ID: commentID, UserID: owner, Body: "new", EditedAt: &now}
		repo.EXPECT().UpdateBody(gomock.Any(), commentID, "new", gomock.Any()).Return(updated, nil)

		c, err := svc.Update(context.Background(), Actor{UserID: owner, Role: "member"}, commentID, "new")
		require.NoError(t, err)
		require.Equal(t, "new", c.Body)
		require.NotNil(t, c.EditedAt)
	})

	t.Run("admin can update any", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		others := &models.Comment{ID: commentID, UserID: uuid.New()}
		repo.EXPECT().GetByID(gomock.Any(), commentID).Return(others, nil)
		repo.EXPECT().UpdateBody(gomock.Any(), commentID, "new", gomock.Any()).Return(others, nil)

		_, err := svc.Update(context.Background(), Actor{UserID: uuid.New(), Role: "admin"}, commentID, "new")
		require.NoError(t, err)
	})
}

func TestDelete(t *testing.T) {
	commentID := uuid.New()

	t.Run("not found", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		repo.EXPECT().GetByID(gomock.Any(), commentID).Return(nil, gorm.ErrRecordNotFound)
		require.ErrorIs(t, svc.Delete(context.Background(), verifiedActor(), commentID), ErrCommentNotFound)
	})

	t.Run("foreign user forbidden", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		repo.EXPECT().GetByID(gomock.Any(), commentID).Return(&models.Comment{ID: commentID, UserID: uuid.New()}, nil)
		require.ErrorIs(t, svc.Delete(context.Background(), verifiedActor(), commentID), ErrCommentForbidden)
	})

	t.Run("author deletes", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		owner := uuid.New()
		repo.EXPECT().GetByID(gomock.Any(), commentID).Return(&models.Comment{ID: commentID, UserID: owner}, nil)
		repo.EXPECT().Delete(gomock.Any(), commentID).Return(nil)
		require.NoError(t, svc.Delete(context.Background(), Actor{UserID: owner, Role: "member"}, commentID))
	})
}

func TestListTopCursor(t *testing.T) {
	t.Run("next cursor present on full page", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		streamID := uuid.New()
		now := time.Now().UTC()
		items := []*models.CommentView{
			{Comment: models.Comment{ID: uuid.New(), CreatedAt: now}},
			{Comment: models.Comment{ID: uuid.New(), CreatedAt: now.Add(-time.Minute)}},
		}
		repo.EXPECT().ListByStream(gomock.Any(), streamID, 2, gomock.Nil(), gomock.Nil()).Return(items, nil)

		got, next, err := svc.ListTop(context.Background(), streamID, 2, nil)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.NotNil(t, next)
		require.Equal(t, items[1].ID, next.ID)
	})

	t.Run("no next cursor on short page", func(t *testing.T) {
		svc, repo, _ := newTestService(t)
		repo.EXPECT().ListByStream(gomock.Any(), gomock.Any(), 50, gomock.Nil(), gomock.Nil()).Return([]*models.CommentView{}, nil)
		_, next, err := svc.ListTop(context.Background(), uuid.New(), 50, nil)
		require.NoError(t, err)
		require.Nil(t, next)
	})
}

func TestListReadGate(t *testing.T) {
	t.Run("published public stream exposed", func(t *testing.T) {
		svc, repo, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: stream.StatusPublished, Visibility: "public"}, nil)
		repo.EXPECT().ListByStream(gomock.Any(), streamID, 50, gomock.Nil(), gomock.Nil()).
			Return([]*models.CommentView{}, nil)

		_, _, err := svc.ListTop(context.Background(), streamID, 50, nil)
		require.NoError(t, err)
	})

	t.Run("published private rejected", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: stream.StatusPublished, Visibility: stream.VisibilityPrivate}, nil)
		_, _, err := svc.ListTop(context.Background(), streamID, 50, nil)
		require.ErrorIs(t, err, ErrStreamNotPublished)
	})

	t.Run("non-published rejected", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: "draft", Visibility: "public"}, nil)
		_, _, err := svc.ListTop(context.Background(), streamID, 50, nil)
		require.ErrorIs(t, err, ErrStreamNotPublished)
	})

	t.Run("stream not found", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).Return(nil, stream.ErrStreamNotFound)
		_, _, err := svc.ListTop(context.Background(), streamID, 50, nil)
		require.ErrorIs(t, err, ErrStreamNotFound)
	})

	t.Run("stream service down fails closed", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).Return(nil, errors.New("dial tcp: refused"))
		_, _, err := svc.ListTop(context.Background(), streamID, 50, nil)
		require.ErrorIs(t, err, ErrStreamUnavailable)
	})

	t.Run("no client fails closed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := repomock.NewMockCommentRepository(ctrl)
		svc := NewCommentsServiceImpl(repo)
		_, _, err := svc.ListTop(context.Background(), uuid.New(), 50, nil)
		require.ErrorIs(t, err, ErrStreamUnavailable)
	})
}

func TestListRepliesReadGate(t *testing.T) {
	streamID := uuid.New()

	t.Run("parent not found", func(t *testing.T) {
		svc, repo, _, _ := newGateTestService(t)
		parentID := uuid.New()
		repo.EXPECT().GetByID(gomock.Any(), parentID).Return(nil, gorm.ErrRecordNotFound)
		_, _, err := svc.ListReplies(context.Background(), parentID, 50, nil)
		require.ErrorIs(t, err, ErrCommentNotFound)
	})

	t.Run("parent on private stream rejected", func(t *testing.T) {
		svc, repo, _, sclient := newGateTestService(t)
		parentID := uuid.New()
		repo.EXPECT().GetByID(gomock.Any(), parentID).
			Return(&models.Comment{ID: parentID, StreamID: streamID}, nil)
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: stream.StatusPublished, Visibility: stream.VisibilityPrivate}, nil)
		_, _, err := svc.ListReplies(context.Background(), parentID, 50, nil)
		require.ErrorIs(t, err, ErrStreamNotPublished)
	})

	t.Run("parent on commentable stream exposed", func(t *testing.T) {
		svc, repo, _, sclient := newGateTestService(t)
		parentID := uuid.New()
		repo.EXPECT().GetByID(gomock.Any(), parentID).
			Return(&models.Comment{ID: parentID, StreamID: streamID}, nil)
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: stream.StatusPublished, Visibility: "public"}, nil)
		repo.EXPECT().ListReplies(gomock.Any(), parentID, 50, gomock.Nil(), gomock.Nil()).
			Return([]*models.CommentView{}, nil)

		_, _, err := svc.ListReplies(context.Background(), parentID, 50, nil)
		require.NoError(t, err)
	})
}

func TestRecordEventBestEffort(t *testing.T) {
	svc, repo, rec := newTestService(t)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
	rec.EXPECT().RecordActivityEvent(gomock.Any(), gomock.Any(), EventCommentCreated, gomock.Any(), gomock.Any()).Return(errors.New("redis down"))

	c, err := svc.Create(context.Background(), verifiedActor(), uuid.New(), nil, "still persists")
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestCreateStreamGate(t *testing.T) {
	t.Run("published private rejected", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: stream.StatusPublished, Visibility: stream.VisibilityPrivate}, nil)
		_, err := svc.Create(context.Background(), verifiedActor(), streamID, nil, "hello")
		require.ErrorIs(t, err, ErrStreamNotPublished)
	})

	t.Run("non-published rejected (admin included)", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: "draft", Visibility: "public"}, nil)
		_, err := svc.Create(context.Background(), Actor{UserID: uuid.New(), Role: "admin", EmailVerified: false}, streamID, nil, "hello")
		require.ErrorIs(t, err, ErrStreamNotPublished)
	})

	t.Run("stream not found", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).Return(nil, stream.ErrStreamNotFound)
		_, err := svc.Create(context.Background(), verifiedActor(), streamID, nil, "hello")
		require.ErrorIs(t, err, ErrStreamNotFound)
	})

	t.Run("stream service down fails closed", func(t *testing.T) {
		svc, _, _, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).Return(nil, errors.New("dial tcp: refused"))
		_, err := svc.Create(context.Background(), verifiedActor(), streamID, nil, "hello")
		require.ErrorIs(t, err, ErrStreamUnavailable)
	})

	t.Run("no client fails closed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := repomock.NewMockCommentRepository(ctrl)
		svc := NewCommentsServiceImpl(repo)
		_, err := svc.Create(context.Background(), verifiedActor(), uuid.New(), nil, "hello")
		require.ErrorIs(t, err, ErrStreamUnavailable)
	})

	t.Run("published unlisted allowed", func(t *testing.T) {
		svc, repo, rec, sclient := newGateTestService(t)
		streamID := uuid.New()
		sclient.EXPECT().Status(gomock.Any(), streamID).
			Return(&stream.StatusInfo{Status: stream.StatusPublished, Visibility: "unlisted"}, nil)
		repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
		rec.EXPECT().RecordActivityEvent(gomock.Any(), gomock.Any(), EventCommentCreated, gomock.Any(), gomock.Any()).Return(nil)

		c, err := svc.Create(context.Background(), verifiedActor(), streamID, nil, "hello")
		require.NoError(t, err)
		require.Equal(t, "hello", c.Body)
	})
}
