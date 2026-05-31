package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// CoverRepository is the MongoDB-backed ports.CoverRepository.
type CoverRepository struct {
	coll *mongo.Collection
}

var _ ports.CoverRepository = (*CoverRepository)(nil)

// Save inserts a new cover or replaces an existing one (upsert by _id).
func (r *CoverRepository) Save(ctx context.Context, cover *domain.Cover) error {
	now := time.Now().UTC()
	cover.UpdatedAt = now
	if cover.ID == "" {
		cover.ID = bson.NewObjectID().Hex()
		if cover.CreatedAt.IsZero() {
			cover.CreatedAt = now
		}
		_, err := r.coll.InsertOne(ctx, cover)
		return err
	}
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": cover.ID}, cover, options.Replace().SetUpsert(true))
	return err
}

// FindByID looks up a cover by id, mapping a miss to domain.ErrNotFound.
func (r *CoverRepository) FindByID(ctx context.Context, id string) (*domain.Cover, error) {
	var cover domain.Cover
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&cover); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &cover, nil
}

// ListByUser returns a user's covers, newest first, paginated.
func (r *CoverRepository) ListByUser(ctx context.Context, userID string, limit, offset int) ([]domain.Cover, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cursor, err := r.coll.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	covers := make([]domain.Cover, 0)
	if err := cursor.All(ctx, &covers); err != nil {
		return nil, err
	}
	return covers, nil
}
