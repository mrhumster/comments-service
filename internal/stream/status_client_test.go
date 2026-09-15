package stream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHTTPStatusClient_Status(t *testing.T) {
	t.Run("published public", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/stream/"+testStreamID().String()+"/status", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"published","visibility":"public"}`))
		}))
		defer srv.Close()

		client := NewStatusClient(srv.URL)
		info, err := client.Status(context.Background(), testStreamID())
		require.NoError(t, err)
		require.True(t, info.Commentable())
	})

	t.Run("published private rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"published","visibility":"private"}`))
		}))
		defer srv.Close()

		client := NewStatusClient(srv.URL)
		info, err := client.Status(context.Background(), testStreamID())
		require.NoError(t, err)
		require.False(t, info.Commentable())
	})

	t.Run("not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"stream not found"}`))
		}))
		defer srv.Close()

		client := NewStatusClient(srv.URL)
		_, err := client.Status(context.Background(), testStreamID())
		require.ErrorIs(t, err, ErrStreamNotFound)
	})

	t.Run("unexpected status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		client := NewStatusClient(srv.URL)
		_, err := client.Status(context.Background(), testStreamID())
		require.Error(t, err)
	})
}

func testStreamID() uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-000000000042")
}