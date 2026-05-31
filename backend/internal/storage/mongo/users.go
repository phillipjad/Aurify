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

// UserRepository is the MongoDB-backed ports.UserRepository.
type UserRepository struct {
	coll *mongo.Collection
}

var _ ports.UserRepository = (*UserRepository)(nil)

// Save inserts a new user or replaces an existing one (upsert by _id).
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	now := time.Now().UTC()
	user.UpdatedAt = now
	if user.ID == "" {
		user.ID = bson.NewObjectID().Hex()
		user.CreatedAt = now
		_, err := r.coll.InsertOne(ctx, user)
		return err
	}
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": user.ID}, user, options.Replace().SetUpsert(true))
	return err
}

// FindByID looks up a user by id, mapping a miss to domain.ErrNotFound.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	return r.findOne(ctx, bson.M{"_id": id})
}

// FindByEmail looks up a user by email, mapping a miss to domain.ErrNotFound.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findOne(ctx, bson.M{"email": email})
}

func (r *UserRepository) findOne(ctx context.Context, filter bson.M) (*domain.User, error) {
	var user domain.User
	if err := r.coll.FindOne(ctx, filter).Decode(&user); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}
