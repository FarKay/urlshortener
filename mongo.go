package main

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStore keeps links in a MongoDB collection.
type MongoStore struct {
	client *mongo.Client
	coll   *mongo.Collection
}

func NewMongoStore(uri, dbName, collName string) (*MongoStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	// Connect does not prove the server is reachable, Ping does.
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	coll := client.Database(dbName).Collection(collName)

	// A unique index makes MongoDB itself refuse two links with the
	// same code. This replaces the mutex-protected "check then save".
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}

	return &MongoStore{client: client, coll: coll}, nil
}

func (s *MongoStore) Save(ctx context.Context, link Link) error {
	_, err := s.coll.InsertOne(ctx, link)
	if mongo.IsDuplicateKeyError(err) {
		return ErrCodeTaken
	}
	return err
}

func (s *MongoStore) Get(ctx context.Context, code string) (Link, error) {
	var link Link
	err := s.coll.FindOne(ctx, bson.M{"code": code}).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Link{}, ErrNotFound
	}
	return link, err
}

func (s *MongoStore) Visit(ctx context.Context, code string) (Link, error) {
	// $inc is applied by the database in one atomic step, so two
	// simultaneous clicks can never lose an update.
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var link Link
	err := s.coll.FindOneAndUpdate(
		ctx,
		bson.M{"code": code},
		bson.M{"$inc": bson.M{"clicks": 1}},
		opts,
	).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Link{}, ErrNotFound
	}
	return link, err
}

func (s *MongoStore) Delete(ctx context.Context, code string) error {
	res, err := s.coll.DeleteOne(ctx, bson.M{"code": code})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}