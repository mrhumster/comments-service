package handler

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mrhumster/comments-service/internal/domain/models"
	"github.com/mrhumster/comments-service/internal/service"
	svcmock "github.com/mrhumster/comments-service/internal/service/mock"
	"go.uber.org/mock/gomock"
)

func newRouter(svc *svcmock.MockCommentsService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewCommentsHandler(svc)
	r.GET("/streams/:streamId/comments", h.ListTop)
	r.GET("/comments/:id/replies", h.ListReplies)
	r.POST("/streams/:streamId/comments", h.Create)
	r.PATCH("/comments/:id", h.Update)
	r.DELETE("/comments/:id", h.Delete)
	return r
}

func do(method, path string, svc *svcmock.MockCommentsService) *httptest.ResponseRecorder {
	r := newRouter(svc)
	req := httptest.NewRequest(method, path, strings.NewReader(`{"body":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestListTopPublic(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	streamID := uuid.New()
	svc.EXPECT().ListTop(gomock.Any(), streamID, 50, gomock.Nil()).Return([]*models.CommentView{
		{Comment: models.Comment{ID: uuid.New(), Body: "hi", CreatedAt: time.Now().UTC()}},
	}, nil, nil)

	w := do("GET", "/streams/"+streamID.String()+"/comments", svc)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"body":"hi"`) {
		t.Errorf("body missing")
	}
}

func TestListTopInvalidStreamID(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	w := do("GET", "/streams/not-a-uuid/comments", svc)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListTopInvalidCursor(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	w := do("GET", "/streams/"+uuid.New().String()+"/comments?cursor=not-base64!!!", svc)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListTopReadGateErrors(t *testing.T) {
	streamID := uuid.New().String()
	path := "/streams/" + streamID + "/comments"

	t.Run("stream not published", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := svcmock.NewMockCommentsService(ctrl)
		defer ctrl.Finish()
		svc.EXPECT().ListTop(gomock.Any(), gomock.Any(), 50, gomock.Nil()).Return(nil, nil, service.ErrStreamNotPublished)

		w := do("GET", path, svc)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "stream is not published") {
			t.Errorf("missing error reason")
		}
	})

	t.Run("stream not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := svcmock.NewMockCommentsService(ctrl)
		defer ctrl.Finish()
		svc.EXPECT().ListTop(gomock.Any(), gomock.Any(), 50, gomock.Nil()).Return(nil, nil, service.ErrStreamNotFound)

		w := do("GET", path, svc)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "stream not found") {
			t.Errorf("missing error reason")
		}
	})

	t.Run("stream service unavailable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := svcmock.NewMockCommentsService(ctrl)
		defer ctrl.Finish()
		svc.EXPECT().ListTop(gomock.Any(), gomock.Any(), 50, gomock.Nil()).Return(nil, nil, service.ErrStreamUnavailable)

		w := do("GET", path, svc)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("comment not found on replies", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := svcmock.NewMockCommentsService(ctrl)
		defer ctrl.Finish()
		svc.EXPECT().ListReplies(gomock.Any(), gomock.Any(), 50, gomock.Nil()).Return(nil, nil, service.ErrCommentNotFound)

		w := do("GET", "/comments/"+uuid.New().String()+"/replies", svc)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestCreateUnverified(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	svc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), "hi").Return(nil, service.ErrEmailNotVerified)

	w := do("POST", "/streams/"+uuid.New().String()+"/comments", svc)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "email not verified") {
		t.Errorf("missing error reason")
	}
}

func TestCreateVerified(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	created := &models.Comment{ID: uuid.New(), Body: "hi", CreatedAt: time.Now().UTC()}
	svc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), "hi").Return(created, nil)

	w := do("POST", "/streams/"+uuid.New().String()+"/comments", svc)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateStreamGateErrors(t *testing.T) {
	t.Run("stream not published", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := svcmock.NewMockCommentsService(ctrl)
		defer ctrl.Finish()
		svc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), "hi").Return(nil, service.ErrStreamNotPublished)

		w := do("POST", "/streams/"+uuid.New().String()+"/comments", svc)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "stream is not published") {
			t.Errorf("missing error reason")
		}
	})

	t.Run("stream not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := svcmock.NewMockCommentsService(ctrl)
		defer ctrl.Finish()
		svc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), "hi").Return(nil, service.ErrStreamNotFound)

		w := do("POST", "/streams/"+uuid.New().String()+"/comments", svc)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("stream service unavailable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := svcmock.NewMockCommentsService(ctrl)
		defer ctrl.Finish()
		svc.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Nil(), "hi").Return(nil, service.ErrStreamUnavailable)

		w := do("POST", "/streams/"+uuid.New().String()+"/comments", svc)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})
}

func TestUpdateForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	svc.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), "hi").Return(nil, service.ErrCommentForbidden)

	w := do("PATCH", "/comments/"+uuid.New().String(), svc)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestUpdateOK(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	now := time.Now().UTC()
	svc.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), "hi").Return(&models.Comment{ID: uuid.New(), Body: "hi", EditedAt: &now}, nil)

	w := do("PATCH", "/comments/"+uuid.New().String(), svc)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteOK(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	svc.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	w := do("DELETE", "/comments/"+uuid.New().String(), svc)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("missing ok")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmock.NewMockCommentsService(ctrl)
	defer ctrl.Finish()

	streamID := uuid.New()
	ts := time.Date(2026, 9, 15, 12, 0, 0, 123, time.UTC)
	lastID := uuid.New()

	decoded, err := base64.RawURLEncoding.DecodeString(base64.RawURLEncoding.EncodeToString([]byte(ts.Format(time.RFC3339Nano) + "|" + lastID.String())))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "2026-09-15T12:00:00.000000123Z|"+lastID.String() {
		t.Fatalf("cursor round trip failed: %s", decoded)
	}

	svc.EXPECT().ListTop(gomock.Any(), streamID, 50, gomock.Nil()).Return([]*models.CommentView{
		{Comment: models.Comment{ID: lastID, CreatedAt: ts}},
	}, &service.Cursor{CreatedAt: ts, ID: lastID}, nil)

	cursor := base64.RawURLEncoding.EncodeToString([]byte(ts.Format(time.RFC3339Nano) + "|" + lastID.String()))
	svc.EXPECT().ListTop(gomock.Any(), streamID, 50, gomock.Not(gomock.Nil())).Return([]*models.CommentView{}, nil, nil)

	w := do("GET", "/streams/"+streamID.String()+"/comments", svc)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"next_cursor":"`+cursor+`"`) {
		t.Fatalf("missing next_cursor: %s", w.Body.String())
	}

	w2 := do("GET", "/streams/"+streamID.String()+"/comments?cursor="+cursor, svc)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 with cursor, got %d", w2.Code)
	}
}
