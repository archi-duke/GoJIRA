package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"gojira-api/config"
)

var (
	client   *mongo.Client
	database *mongo.Database
)

func Init() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	client, err = mongo.Connect(options.Client().ApplyURI(config.MongoURI))
	if err != nil {
		log.Fatalf("MongoDB connect error: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("MongoDB ping error: %v", err)
	}

	database = client.Database(config.MongoDB)

	// Ensure unique index on issues.key
	issuesCol := database.Collection("issues")
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "key", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	if _, err := issuesCol.Indexes().CreateOne(ctx, indexModel); err != nil {
		log.Printf("Warning: index creation: %v", err)
	}

	log.Println("MongoDB connected:", config.MongoURI)
}

func Issues() *mongo.Collection   { return database.Collection("issues") }
func Projects() *mongo.Collection { return database.Collection("projects") }
func Comments() *mongo.Collection { return database.Collection("comments") }
func Accounts() *mongo.Collection { return database.Collection("accounts") }

// --- Issue CRUD ---

func FindIssueByKey(ctx context.Context, key string) (bson.M, error) {
	var result bson.M
	err := Issues().FindOne(ctx, bson.M{"key": key}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func FindAllIssues(ctx context.Context, filter bson.M) ([]bson.M, error) {
	if filter == nil {
		filter = bson.M{}
	}
	cursor, err := Issues().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func UpsertIssue(ctx context.Context, key string, doc bson.M) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := Issues().UpdateOne(ctx, bson.M{"key": key}, bson.M{"$set": doc}, opts)
	return err
}

func UpdateIssueFields(ctx context.Context, key string, fields bson.M) error {
	fields["_dirty"] = true
	fields["fields.updated"] = time.Now().Format("2006-01-02T15:04:05.000+0900")
	_, err := Issues().UpdateOne(ctx, bson.M{"key": key}, bson.M{"$set": fields})
	return err
}

func DeleteIssueByKey(ctx context.Context, key string) error {
	_, err := Issues().DeleteOne(ctx, bson.M{"key": key})
	return err
}

func FindDirtyIssues(ctx context.Context) ([]bson.M, error) {
	return FindAllIssues(ctx, bson.M{"_dirty": true})
}

func ClearDirtyFlag(ctx context.Context, key string) error {
	_, err := Issues().UpdateOne(ctx, bson.M{"key": key}, bson.M{
		"$set": bson.M{"_dirty": false, "_syncedAt": time.Now()},
	})
	return err
}

// --- Comments ---

func InsertComment(ctx context.Context, doc bson.M) error {
	_, err := Comments().InsertOne(ctx, doc)
	return err
}

func FindCommentsByKey(ctx context.Context, issueKey string) ([]bson.M, error) {
	cursor, err := Comments().Find(ctx, bson.M{"issueKey": issueKey})
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func UpdateComment(ctx context.Context, commentID string, body string) error {
	_, err := Comments().UpdateOne(ctx,
		bson.M{"commentId": commentID},
		bson.M{"$set": bson.M{"body": body}},
	)
	return err
}

func DeleteComment(ctx context.Context, commentID string) error {
	_, err := Comments().DeleteOne(ctx, bson.M{"commentId": commentID})
	return err
}

// --- Projects ---

func FindAllProjects(ctx context.Context, filter bson.M) ([]bson.M, error) {
	if filter == nil {
		filter = bson.M{}
	}
	cursor, err := Projects().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func UpsertProject(ctx context.Context, jiraCode string, doc bson.M) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := Projects().UpdateOne(ctx, bson.M{"JiraCode": jiraCode}, bson.M{"$set": doc}, opts)
	return err
}

// --- Accounts ---

func UpsertAccount(ctx context.Context, jiraID string, doc bson.M) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := Accounts().UpdateOne(ctx, bson.M{"jiraID": jiraID}, bson.M{"$set": doc}, opts)
	return err
}
