package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// OpencodeInstances 사용자별 OpenCode 인스턴스 라우팅 매핑.
//
// 한 사용자당 최대 1건 (unique userId). Agent-Service 가 X-User-Id 헤더로
// 사용자별 OpenCode 인스턴스 host:port 를 조회해 라우팅한다.
//
// status:
//   - "active"  — systemd 가 띄운 인스턴스가 살아있음 (헬스체크 통과)
//   - "down"    — 헬스체크 실패 또는 명시적 disable. Agent-Service 는 호출 거부.
//
// 노드 분산은 host 필드로 흡수 — 노드 추가 시 코드 변경 없이 등록만 바꾸면 됨.
func OpencodeInstances() *mongo.Collection {
	return database.Collection("opencodeInstances")
}

func InitOpencodeInstanceIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 사용자당 1건 — 명시적 등록/해제로만 변경.
	_, err := OpencodeInstances().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Printf("index opencodeInstances userId unique: %v", err)
	}

	// 헬스체크/stale 정리용
	_, err = OpencodeInstances().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "status", Value: 1}, {Key: "lastHeartbeat", Value: 1}},
	})
	if err != nil {
		log.Printf("index opencodeInstances status+heartbeat: %v", err)
	}

	log.Println("opencodeInstance indexes initialized")
}

// FindOpencodeInstance 사용자의 인스턴스 1건 조회 (status 무관).
func FindOpencodeInstance(ctx context.Context, userID string) (bson.M, error) {
	var result bson.M
	err := OpencodeInstances().FindOne(ctx, bson.M{"userId": userID}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// FindActiveOpencodeInstance Agent-Service 가 라우팅에 쓸 active 인스턴스 조회.
// status != "active" 면 not found 처리.
func FindActiveOpencodeInstance(ctx context.Context, userID string) (bson.M, error) {
	var result bson.M
	err := OpencodeInstances().FindOne(ctx, bson.M{"userId": userID, "status": "active"}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// UpsertOpencodeInstance 등록/갱신. 등록 스크립트 또는 인스턴스 셀프-등록이 호출.
// host/port 가 바뀌면 같은 userId 위에 덮어쓴다.
func UpsertOpencodeInstance(ctx context.Context, userID, host string, port int) error {
	now := time.Now()
	opts := options.UpdateOne().SetUpsert(true)
	_, err := OpencodeInstances().UpdateOne(ctx,
		bson.M{"userId": userID},
		bson.M{
			"$set": bson.M{
				"host":          host,
				"port":          port,
				"status":        "active",
				"lastHeartbeat": now,
				"updatedAt":     now,
			},
			"$setOnInsert": bson.M{
				"createdAt": now,
			},
		},
		opts,
	)
	return err
}

// MarkOpencodeInstanceDown 헬스체크 실패 시 status=down. 사용자에게 보고용.
func MarkOpencodeInstanceDown(ctx context.Context, userID string) error {
	now := time.Now()
	_, err := OpencodeInstances().UpdateOne(ctx,
		bson.M{"userId": userID},
		bson.M{"$set": bson.M{"status": "down", "updatedAt": now}},
	)
	return err
}

// UpdateOpencodeInstanceHeartbeat 헬스체크 통과 시 호출. status=active 로 회복.
func UpdateOpencodeInstanceHeartbeat(ctx context.Context, userID string) error {
	now := time.Now()
	_, err := OpencodeInstances().UpdateOne(ctx,
		bson.M{"userId": userID},
		bson.M{"$set": bson.M{"status": "active", "lastHeartbeat": now, "updatedAt": now}},
	)
	return err
}

// DeleteOpencodeInstance 등록 해제 (gojira-user-remove 가 호출).
func DeleteOpencodeInstance(ctx context.Context, userID string) error {
	_, err := OpencodeInstances().DeleteOne(ctx, bson.M{"userId": userID})
	return err
}

// NextOpencodePort 등록된 인스턴스 중 가장 큰 port + 1 을 반환.
// 비어있으면 fallback 반환 (보통 4096). port 충돌 검사는 호출자가 (또는 systemd 가 fail-fast).
func NextOpencodePort(ctx context.Context, fallback int) (int, error) {
	opts := options.FindOne().SetSort(bson.D{{Key: "port", Value: -1}})
	var doc bson.M
	err := OpencodeInstances().FindOne(ctx, bson.M{}, opts).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return fallback, nil
	}
	if err != nil {
		return 0, err
	}
	switch v := doc["port"].(type) {
	case int32:
		return int(v) + 1, nil
	case int64:
		return int(v) + 1, nil
	case int:
		return v + 1, nil
	default:
		return fallback, nil
	}
}
