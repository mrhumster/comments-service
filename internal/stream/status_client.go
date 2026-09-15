//go:generate mockgen -source=status_client.go -destination=mock/status_client_mock.go -package=mock
package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusPublished   = "published"
	VisibilityPrivate = "private"
)

var ErrStreamNotFound = errors.New("stream not found")

// StatusInfo is the response of the internal stream-service status endpoint.
type StatusInfo struct {
	Status     string `json:"status"`
	Visibility string `json:"visibility"`
}

// Commentable reports whether comments are allowed on this stream: published
// status and not private visibility (public and unlisted published streams are
// commentable).
func (i *StatusInfo) Commentable() bool {
	return i.Status == StatusPublished && i.Visibility != VisibilityPrivate
}

// StatusClient resolves a stream's commentability via stream-service.
type StatusClient interface {
	Status(ctx context.Context, id uuid.UUID) (*StatusInfo, error)
}

type HTTPStatusClient struct {
	baseURL string
	client  *http.Client
}

// NewStatusClient returns a client calling GET {baseURL}/stream/{id}/status.
func NewStatusClient(baseURL string) *HTTPStatusClient {
	return &HTTPStatusClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// Status returns the stream status, or ErrStreamNotFound on a 404.
func (c *HTTPStatusClient) Status(ctx context.Context, id uuid.UUID) (*StatusInfo, error) {
	url := fmt.Sprintf("%s/stream/%s/status", c.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("stream status request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream status request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrStreamNotFound
	case http.StatusOK:
		var info StatusInfo
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return nil, fmt.Errorf("decode stream status: %w", err)
		}
		return &info, nil
	default:
		return nil, fmt.Errorf("stream status: unexpected HTTP %d", resp.StatusCode)
	}
}