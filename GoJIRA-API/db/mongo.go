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

	// Ensure unique index on usecase_artifacts.projectId (1:1 with project)
	uaCol := database.Collection("usecase_artifacts")
	uaIdx := mongo.IndexModel{
		Keys:    bson.D{{Key: "projectId", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	if _, err := uaCol.Indexes().CreateOne(ctx, uaIdx); err != nil {
		log.Printf("Warning: usecase_artifacts index creation: %v", err)
	}

	log.Println("MongoDB connected:", config.MongoURI)
}

func Issues() *mongo.Collection      { return database.Collection("issues") }
func IssuesTrash() *mongo.Collection { return database.Collection("issues_trash") }
func Projects() *mongo.Collection    { return database.Collection("projects") }
func Comments() *mongo.Collection    { return database.Collection("comments") }
func Accounts() *mongo.Collection    { return database.Collection("accounts") }

// MoveIssueToTrash — issues 컬렉션 doc 을 issues_trash 로 이동 (휴지통).
// _trashedAt / _trashedBy / _yamlContent 추가. 원본 이슈는 issues 에서 삭제.
// 자동 정리는 PurgeOldTrash 가 _trashedAt 기준 3일 경과 시 처리.
// 이슈가 없으면 no-op (DELETE 멱등성 보장).
func MoveIssueToTrash(ctx context.Context, key, user, yamlContent string) error {
	doc, err := FindIssueByKey(ctx, key)
	if err == mongo.ErrNoDocuments {
		return nil
	}
	if err != nil {
		return err
	}
	if doc == nil {
		return nil
	}
	delete(doc, "_id")
	doc["_trashedAt"] = time.Now()
	doc["_trashedBy"] = user
	if yamlContent != "" {
		doc["_yamlContent"] = yamlContent
	}
	if _, err := IssuesTrash().InsertOne(ctx, doc); err != nil {
		return err
	}
	return DeleteIssueByKey(ctx, key)
}

// PurgeOldTrash — _trashedAt 이 cutoff 이전인 휴지통 항목 영구 삭제.
func PurgeOldTrash(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := IssuesTrash().DeleteMany(ctx, bson.M{"_trashedAt": bson.M{"$lt": cutoff}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

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
	fields["updated"] = time.Now().Format("2006-01-02T15:04:05.000+0900")
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

// --- Issue Links ---

// AddIssueLink appends a link entry to an issue's issueLinks array and marks it dirty.
func AddIssueLink(ctx context.Context, key string, link bson.M) error {
	_, err := Issues().UpdateOne(ctx, bson.M{"key": key}, bson.M{
		"$push": bson.M{"issueLinks": link},
		"$set":  bson.M{"_dirty": true},
	})
	return err
}

// RemoveIssueLink removes a link by linkID from all issues' issueLinks arrays,
// records the linkID in _deletedLinks for JIRA sync, and marks them dirty.
// Returns the keys of affected issues.
func RemoveIssueLink(ctx context.Context, linkID string) ([]string, error) {
	// Find affected issues first
	cursor, err := Issues().Find(ctx, bson.M{"issueLinks.id": linkID})
	if err != nil {
		return nil, err
	}
	var affected []bson.M
	if err := cursor.All(ctx, &affected); err != nil {
		return nil, err
	}
	var keys []string
	for _, doc := range affected {
		if k, ok := doc["key"].(string); ok {
			keys = append(keys, k)
		}
	}

	_, err = Issues().UpdateMany(ctx,
		bson.M{"issueLinks.id": linkID},
		bson.M{
			"$pull": bson.M{"issueLinks": bson.M{"id": linkID}},
			"$push": bson.M{"_deletedLinks": linkID},
			"$set":  bson.M{"_dirty": true},
		},
	)
	return keys, err
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

func FindProjectByCode(ctx context.Context, jiraCode string) bson.M {
	var doc bson.M
	err := Projects().FindOne(ctx, bson.M{"jiraCodes.main.jiraCode": jiraCode}).Decode(&doc)
	if err != nil {
		return nil
	}
	return doc
}

func UpsertProject(ctx context.Context, jiraCode string, doc bson.M) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := Projects().UpdateOne(ctx, bson.M{"jiraCodes.main.jiraCode": jiraCode}, bson.M{"$set": doc}, opts)
	return err
}

func UpdateProjectField(ctx context.Context, jiraCode string, field string, value interface{}) error {
	_, err := Projects().UpdateOne(ctx, bson.M{"jiraCodes.main.jiraCode": jiraCode}, bson.M{"$set": bson.M{field: value}})
	return err
}

// UpdateProjectFields merges multiple fields into the project document with $set.
// Keys may use dot notation (e.g. "archSettings.normalizedAttributes") for nested paths.
// Returns (matched bool, error). matched=false when no project with the given code exists.
func UpdateProjectFields(ctx context.Context, jiraCode string, fields bson.M) (bool, error) {
	if len(fields) == 0 {
		return true, nil
	}
	res, err := Projects().UpdateOne(ctx,
		bson.M{"jiraCodes.main.jiraCode": jiraCode},
		bson.M{"$set": fields},
	)
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// UnsetProjectField removes a single top-level field from the project document.
// Used by reset flows that need a true absence (not a null value).
func UnsetProjectField(ctx context.Context, jiraCode, field string) error {
	_, err := Projects().UpdateOne(ctx,
		bson.M{"jiraCodes.main.jiraCode": jiraCode},
		bson.M{"$unset": bson.M{field: ""}},
	)
	return err
}

// DeleteProjectByCode removes the project document entirely from the projects
// collection. Used by archive flows (deleteProject=true in reset). Returns
// (matched bool, error). matched=false when no such project exists.
func DeleteProjectByCode(ctx context.Context, jiraCode string) (bool, error) {
	res, err := Projects().DeleteOne(ctx,
		bson.M{"jiraCodes.main.jiraCode": jiraCode},
	)
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// --- Accounts ---

func UpsertAccount(ctx context.Context, jiraID string, doc bson.M) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := Accounts().UpdateOne(ctx, bson.M{"jiraID": jiraID}, bson.M{"$set": doc}, opts)
	return err
}
