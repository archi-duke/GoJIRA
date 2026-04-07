package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func AgentSessions() *mongo.Collection {
	return database.Collection("agent_sessions")
}

func CreateAgentSession(ctx context.Context, sessionID, userID string, sessionContext bson.M) error {
	_, err := AgentSessions().InsertOne(ctx, bson.M{
		"_id":       sessionID,
		"userId":    userID,
		"messages":  []interface{}{},
		"context":   sessionContext,
		"createdAt": time.Now(),
		"updatedAt": time.Now(),
	})
	return err
}

func GetAgentSession(ctx context.Context, sessionID string) (bson.M, error) {
	var doc bson.M
	err := AgentSessions().FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func AppendAgentMessage(ctx context.Context, sessionID string, message bson.M) error {
	_, err := AgentSessions().UpdateOne(ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$push": bson.M{"messages": message},
			"$set":  bson.M{"updatedAt": time.Now()},
		},
	)
	return err
}

func UpdateAgentSessionContext(ctx context.Context, sessionID string, sessionContext bson.M) error {
	_, err := AgentSessions().UpdateOne(ctx,
		bson.M{"_id": sessionID},
		bson.M{"$set": bson.M{"context": sessionContext, "updatedAt": time.Now()}},
	)
	return err
}

func DeleteAgentSession(ctx context.Context, sessionID string) error {
	_, err := AgentSessions().DeleteOne(ctx, bson.M{"_id": sessionID})
	return err
}

// ListAgentSessions 사용자의 세션 목록 조회 (최신순, 제목은 첫 메시지)
func ListAgentSessions(ctx context.Context, userID string) ([]bson.M, error) {
	opts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(50)
	// messages는 제외하고 메타데이터만 반환
	opts.SetProjection(bson.M{"messages": bson.M{"$slice": 1}, "userId": 1, "context": 1, "createdAt": 1, "updatedAt": 1})
	cursor, err := AgentSessions().Find(ctx, bson.M{"userId": userID}, opts)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	cursor.All(ctx, &results)
	return results, nil
}

// SearchAgentSessions 세션 내 메시지 검색
func SearchAgentSessions(ctx context.Context, userID, query string) ([]bson.M, error) {
	filter := bson.M{
		"userId":           userID,
		"messages.content": bson.M{"$regex": query, "$options": "i"},
	}
	opts := options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(20)
	opts.SetProjection(bson.M{"messages": bson.M{"$slice": 1}, "userId": 1, "context": 1, "createdAt": 1, "updatedAt": 1})
	cursor, err := AgentSessions().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	cursor.All(ctx, &results)
	return results, nil
}

// EnsureAgentSessionIndex TTL 인덱스 생성 (7일 후 자동 삭제)
func EnsureAgentSessionIndex(ctx context.Context) {
	expireAfter := int32(7 * 24 * 3600) // 7일
	AgentSessions().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "updatedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(expireAfter),
	})
}
