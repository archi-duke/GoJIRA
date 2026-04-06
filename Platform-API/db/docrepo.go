package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- project.documents 기반 repo 관리 (docRepos 컬렉션 미사용) ---

// FindAllProjects projects 컬렉션 전체 조회
func FindAllProjects(ctx context.Context) ([]bson.M, error) {
	cursor, err := database.Collection("projects").Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// FindProjectByCode jiraCode로 프로젝트 조회
func FindProjectByCode(ctx context.Context, jiraCode string) (bson.M, error) {
	var doc bson.M
	err := database.Collection("projects").FindOne(ctx, bson.M{"jiraCodes.main.jiraCode": jiraCode}).Decode(&doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// UpdateProjectDocuments project.documents 필드 업데이트
func UpdateProjectDocuments(ctx context.Context, jiraCode string, docs bson.M) error {
	_, err := database.Collection("projects").UpdateOne(ctx,
		bson.M{"jiraCodes.main.jiraCode": jiraCode},
		bson.M{"$set": bson.M{"documents": docs}},
	)
	return err
}

// UpdateProjectField 프로젝트의 단일 필드 업데이트
func UpdateProjectField(ctx context.Context, jiraCode string, field string, value interface{}) error {
	_, err := database.Collection("projects").UpdateOne(ctx,
		bson.M{"jiraCodes.main.jiraCode": jiraCode},
		bson.M{"$set": bson.M{field: value}},
	)
	return err
}

// ClearProjectDocRepo project.documents를 빈 객체로 초기화
func ClearProjectDocRepo(ctx context.Context, jiraCode string) {
	database.Collection("projects").UpdateOne(ctx,
		bson.M{"jiraCodes.main.jiraCode": jiraCode},
		bson.M{"$set": bson.M{"documents": bson.M{}}},
	)
}

// GetProjectDocInfo project.documents에서 repo 정보 추출
func GetProjectDocInfo(ctx context.Context, jiraCode string) (owner, name, defaultBranch string, err error) {
	prj, err := FindProjectByCode(ctx, jiraCode)
	if err != nil {
		return "", "", "", err
	}
	docs := BsonGetDoc(prj, "documents")
	owner = BsonGetStr(docs, "repoOwner")
	name = BsonGetStr(docs, "repoName")
	defaultBranch = BsonGetStr(docs, "defaultBranch")
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	return
}

// --- DocLinks (유지) ---

func DocLinks() *bson.M { return nil } // placeholder

func InitDocRepoIndexes() {
	log.Println("docRepo indexes: using project.documents (no separate collection)")
}

// --- bson 헬퍼 ---

func BsonGetDoc(doc bson.M, key string) bson.M {
	if doc == nil {
		return nil
	}
	switch v := doc[key].(type) {
	case bson.M:
		return v
	case bson.D:
		m := bson.M{}
		for _, e := range v {
			m[e.Key] = e.Value
		}
		return m
	}
	return nil
}

func BsonGetStr(doc bson.M, key string) string {
	if doc == nil {
		return ""
	}
	s, _ := doc[key].(string)
	return s
}

// UpdateProjectDocRepo project.documents에 repo 정보 설정
func UpdateProjectDocRepo(ctx context.Context, jiraCode, repoName, repoOwner, repoUrl string) {
	database.Collection("projects").UpdateOne(ctx,
		bson.M{"jiraCodes.main.jiraCode": jiraCode},
		bson.M{"$set": bson.M{
			"documents.repoName":      repoName,
			"documents.repoOwner":     repoOwner,
			"documents.repoUrl":       repoUrl,
			"documents.defaultBranch": "main",
		}},
	)
}

// dummy to keep import
var _ = time.Now
