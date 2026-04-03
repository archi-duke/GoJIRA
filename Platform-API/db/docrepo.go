package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func DocRepos() *mongo.Collection { return database.Collection("docRepos") }
func DocLinks() *mongo.Collection { return database.Collection("docLinks") }

func InitDocRepoIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	DocRepos().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "projectId", Value: 1}},
	})
	DocLinks().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "taskId", Value: 1}},
	})
	DocLinks().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "docRepoId", Value: 1}},
	})
	log.Println("docRepo indexes initialized")
}

// --- DocRepos ---

func FindAllDocRepos(ctx context.Context, filter bson.M) ([]bson.M, error) {
	if filter == nil {
		filter = bson.M{}
	}
	filter["isActive"] = true
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	cursor, err := DocRepos().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func FindDocRepoByID(ctx context.Context, id string) (bson.M, error) {
	return findByID(ctx, DocRepos(), id)
}

func InsertDocRepo(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, DocRepos(), doc)
}

func UpdateDocRepo(ctx context.Context, id string, updates bson.M) error {
	return updateDoc(ctx, DocRepos(), id, updates)
}

func SoftDeleteDocRepo(ctx context.Context, id, by string) error {
	return softDelete(ctx, DocRepos(), id, by)
}

// --- DocLinks ---

func FindDocLinks(ctx context.Context, filter bson.M) ([]bson.M, error) {
	if filter == nil {
		filter = bson.M{}
	}
	filter["isActive"] = true
	cursor, err := DocLinks().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func InsertDocLink(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, DocLinks(), doc)
}

func SoftDeleteDocLink(ctx context.Context, id, by string) error {
	return softDelete(ctx, DocLinks(), id, by)
}
