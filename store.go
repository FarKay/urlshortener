package main

import (
	"context"
	"errors"
	"time"
)

// Errors the handlers understand, whatever database is behind the store.
var (
	ErrNotFound  = errors.New("link not found")
	ErrCodeTaken = errors.New("code already in use")
)

// Link is one shortened URL. The bson tags control how it is
// stored in MongoDB, the json tags how it is sent to clients.
type Link struct {
	Code      string    `json:"code" bson:"code"`
	URL       string    `json:"url" bson:"url"`
	Clicks    int       `json:"clicks" bson:"clicks"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// Store is everything the handlers need from a database.
// MemoryStore and MongoStore both implement it.
type Store interface {
	// Save stores a new link. It returns ErrCodeTaken if the code exists.
	Save(ctx context.Context, link Link) error
	// Get returns a link without counting a click. ErrNotFound if missing.
	Get(ctx context.Context, code string) (Link, error)
	// Visit adds one click and returns the updated link. ErrNotFound if missing.
	Visit(ctx context.Context, code string) (Link, error)
	// Delete removes a link. ErrNotFound if missing.
	Delete(ctx context.Context, code string) error
}