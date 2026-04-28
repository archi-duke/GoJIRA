package db

// usecase_artifacts — usecase-analyze2 가 결정적 조립을 위해 영속화하는 보조 데이터.
// Feature 이슈에 들어있지 않은 4종 (domain / userStories / useCases / diagrams) 만 저장한다.
// projectId 단위 1:1 (unique). schemaVersion 으로 향후 마이그레이션 대비.

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func UsecaseArtifacts() *mongo.Collection { return database.Collection("usecase_artifacts") }

// FindUsecaseArtifact — 없으면 (nil, nil).
func FindUsecaseArtifact(ctx context.Context, projectID interface{}) (bson.M, error) {
	var doc bson.M
	err := UsecaseArtifacts().FindOne(ctx, bson.M{"projectId": projectID}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return doc, nil
}

// UpsertUsecaseArtifactPartial — projectId 기준 upsert. fields 의 키만 $set 한다.
// updatedAt / schemaVersion 은 자동 채움. _id 는 자동 생성.
func UpsertUsecaseArtifactPartial(ctx context.Context, projectID interface{}, fields bson.M, user string) error {
	setDoc := bson.M{}
	for k, v := range fields {
		setDoc[k] = v
	}
	setDoc["projectId"] = projectID
	setDoc["updatedAt"] = time.Now()
	setDoc["schemaVersion"] = 1
	if user != "" {
		setDoc["updatedBy"] = user
	}

	opts := options.UpdateOne().SetUpsert(true)
	_, err := UsecaseArtifacts().UpdateOne(ctx,
		bson.M{"projectId": projectID},
		bson.M{"$set": setDoc},
		opts,
	)
	return err
}

// DeleteUsecaseArtifact — 프로젝트 리셋/아카이브에서 호출.
func DeleteUsecaseArtifact(ctx context.Context, projectID interface{}) error {
	_, err := UsecaseArtifacts().DeleteOne(ctx, bson.M{"projectId": projectID})
	return err
}
