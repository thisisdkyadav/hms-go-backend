package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	users               *mongo.Collection
	passwordResetTokens *mongo.Collection
	wardens             *mongo.Collection
	associateWardens    *mongo.Collection
	securities          *mongo.Collection
	hostelSupervisors   *mongo.Collection
	hostelGates         *mongo.Collection
	hostels             *mongo.Collection
}

func NewRepository(database *mongo.Database) *Repository {
	return &Repository{
		users:               database.Collection("users"),
		passwordResetTokens: database.Collection("passwordresettokens"),
		wardens:             database.Collection("wardens"),
		associateWardens:    database.Collection("associatewardens"),
		securities:          database.Collection("securities"),
		hostelSupervisors:   database.Collection("hostelsupervisors"),
		hostelGates:         database.Collection("hostelgates"),
		hostels:             database.Collection("hostels"),
	}
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string, includePassword bool) (*User, error) {
	filter := bson.M{
		"email": primitive.Regex{
			Pattern: "^" + regexp.QuoteMeta(email) + "$",
			Options: "i",
		},
	}

	return r.findUser(ctx, filter, includePassword)
}

func (r *Repository) FindUserByID(ctx context.Context, userID primitive.ObjectID, includePassword bool) (*User, error) {
	return r.findUser(ctx, bson.M{"_id": userID}, includePassword)
}

func (r *Repository) SetUserAESKey(ctx context.Context, userID primitive.ObjectID, aesKey string) error {
	_, err := r.users.UpdateByID(ctx, userID, bson.M{
		"$set": bson.M{
			"aesKey":    aesKey,
			"updatedAt": time.Now().UTC(),
		},
	})
	return err
}

func (r *Repository) UpdateUserPassword(ctx context.Context, userID primitive.ObjectID, passwordHash string) error {
	_, err := r.users.UpdateByID(ctx, userID, bson.M{
		"$set": bson.M{
			"password":  passwordHash,
			"updatedAt": time.Now().UTC(),
		},
	})
	return err
}

func (r *Repository) UpdatePinnedTabs(ctx context.Context, userID primitive.ObjectID, pinnedTabs []string) ([]string, error) {
	var updated struct {
		PinnedTabs []string `bson:"pinnedTabs"`
	}

	result := r.users.FindOneAndUpdate(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"pinnedTabs": pinnedTabs,
				"updatedAt":  time.Now().UTC(),
			},
		},
		options.FindOneAndUpdate().
			SetReturnDocument(options.After).
			SetProjection(bson.M{"pinnedTabs": 1}),
	)

	if err := result.Decode(&updated); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return updated.PinnedTabs, nil
}

func (r *Repository) InvalidatePasswordResetTokens(ctx context.Context, userID primitive.ObjectID) error {
	_, err := r.passwordResetTokens.UpdateMany(ctx, bson.M{
		"userId": userID,
		"used":   false,
	}, bson.M{
		"$set": bson.M{"used": true},
	})
	return err
}

func (r *Repository) CreatePasswordResetToken(ctx context.Context, token PasswordResetToken) error {
	_, err := r.passwordResetTokens.InsertOne(ctx, token)
	return err
}

func (r *Repository) FindValidPasswordResetToken(ctx context.Context, tokenHash string) (*PasswordResetToken, error) {
	var token PasswordResetToken
	err := r.passwordResetTokens.FindOne(ctx, bson.M{
		"token":     tokenHash,
		"used":      false,
		"expiresAt": bson.M{"$gt": time.Now().UTC()},
	}).Decode(&token)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *Repository) MarkPasswordResetTokenUsed(ctx context.Context, tokenID primitive.ObjectID) error {
	_, err := r.passwordResetTokens.UpdateByID(ctx, tokenID, bson.M{
		"$set": bson.M{"used": true},
	})
	return err
}

func (r *Repository) FindHostelSummary(ctx context.Context, role string, userID primitive.ObjectID) (*HostelSummary, error) {
	type assignmentDocument struct {
		ActiveHostelID primitive.ObjectID `bson:"activeHostelId"`
		HostelID       primitive.ObjectID `bson:"hostelId"`
	}

	var (
		collection *mongo.Collection
		filter     = bson.M{"userId": userID}
		hostelID   primitive.ObjectID
	)

	switch role {
	case "Warden":
		collection = r.wardens
	case "Associate Warden":
		collection = r.associateWardens
	case "Hostel Supervisor":
		collection = r.hostelSupervisors
	case "Security":
		collection = r.securities
	case "Hostel Gate":
		collection = r.hostelGates
	default:
		return nil, nil
	}

	var assignment assignmentDocument
	if err := collection.FindOne(ctx, filter).Decode(&assignment); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("find hostel assignment: %w", err)
	}

	if assignment.ActiveHostelID != primitive.NilObjectID {
		hostelID = assignment.ActiveHostelID
	}
	if hostelID == primitive.NilObjectID && assignment.HostelID != primitive.NilObjectID {
		hostelID = assignment.HostelID
	}
	if hostelID == primitive.NilObjectID {
		return nil, nil
	}

	var hostel HostelSummary
	if err := r.hostels.FindOne(ctx, bson.M{"_id": hostelID}).Decode(&hostel); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("find hostel: %w", err)
	}

	return &hostel, nil
}

func (r *Repository) findUser(ctx context.Context, filter bson.M, includePassword bool) (*User, error) {
	opts := options.FindOne()
	if !includePassword {
		opts.SetProjection(bson.M{"password": 0})
	}

	var user User
	err := r.users.FindOne(ctx, filter, opts).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}
