// Package mongo provides the MongoDB storage adapters (mongo-driver/v2). The
// Store owns the client/database and exposes typed repositories that implement
// the application's ports.
package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Store holds the MongoDB client and the repositories built on top of it.
type Store struct {
	client *mongo.Client
	db     *mongo.Database
	users  *UserRepository
	covers *CoverRepository
}

// Connect dials MongoDB, verifies the connection, and constructs repositories.
func Connect(ctx context.Context, uri, database string) (*Store, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	db := client.Database(database)
	return &Store{
		client: client,
		db:     db,
		users:  &UserRepository{coll: db.Collection("users")},
		covers: &CoverRepository{coll: db.Collection("covers")},
	}, nil
}

// Users returns the user repository.
func (s *Store) Users() *UserRepository { return s.users }

// Covers returns the cover repository.
func (s *Store) Covers() *CoverRepository { return s.covers }

// Ping checks connectivity; used by the readiness probe.
func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

// Disconnect closes the underlying client.
func (s *Store) Disconnect(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}
