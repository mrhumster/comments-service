package models

import (
	"time"

	"github.com/google/uuid"
)

// Comment is a single user comment under a stream. Replies reference a
// top-level comment via ParentID (two-level threads, enforced by the service).
type Comment struct {
	ID        uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	StreamID  uuid.UUID  `json:"stream_id" gorm:"type:uuid;not null;index"`
	UserID    uuid.UUID  `json:"user_id" gorm:"type:uuid;not null;index"`
	UserEmail string     `json:"user_email,omitempty" gorm:"type:text"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty" gorm:"type:uuid"`
	Body      string     `json:"body" gorm:"type:text;not null"`
	EditedAt  *time.Time `json:"edited_at,omitempty"`
	CreatedAt time.Time  `json:"created_at" gorm:"not null"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"not null"`
}

func (Comment) TableName() string { return "comments" }

// CommentView is a comment plus the number of direct replies, used by the
// read API to render reply badges without an extra round trip.
type CommentView struct {
	Comment
	ReplyCount int `json:"reply_count" gorm:"column:reply_count"`
}
