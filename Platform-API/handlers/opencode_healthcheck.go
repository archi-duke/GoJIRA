package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"platform-api/db"
)

// OpenCode 인스턴스 헬스체크 — Mongo opencodeInstances 의 모든 항목에 GET /global/health.
// 응답 OK 시 status=active 로 갱신, 실패 시 status=down.
//
// 주기적 실행은 StartOpencodeHealthcheck() goroutine. 운영자/디버깅용 즉시 트리거는
// POST /admin/opencode-healthcheck (RunOpencodeHealthcheckHandler).

const (
	healthCheckInterval = 5 * time.Minute
	healthCheckTimeout  = 3 * time.Second
)

// 인스턴스별 연속 실패 카운터 — 일시적 글리치 (한 번 응답 안 옴) 는 down 으로 마킹 안 함.
var (
	failureCounts   = make(map[string]int)
	failureCountsMu sync.Mutex
	failureThreshold = 3 // 3회 연속 실패 시 down
)

// StartOpencodeHealthcheck 메인 goroutine — 컨테이너 종료 전까지 5분 주기로 헬스체크.
func StartOpencodeHealthcheck(ctx context.Context) {
	log.Printf("opencode healthcheck: 시작 (interval=%s, timeout=%s, threshold=%d)",
		healthCheckInterval, healthCheckTimeout, failureThreshold)

	// 첫 실행은 약간 지연 후 — 컨테이너 startup race 회피
	time.Sleep(10 * time.Second)
	runOpencodeHealthcheck(ctx)

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("opencode healthcheck: 종료")
			return
		case <-ticker.C:
			runOpencodeHealthcheck(ctx)
		}
	}
}

// runOpencodeHealthcheck 모든 인스턴스에 한 번씩 ping.
func runOpencodeHealthcheck(ctx context.Context) (active int, down int) {
	cur, err := db.OpencodeInstances().Find(ctx, bson.M{})
	if err != nil {
		log.Printf("opencode healthcheck: 컬렉션 조회 실패: %v", err)
		return 0, 0
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		userID, _ := doc["userId"].(string)
		host, _ := doc["host"].(string)
		var port int
		switch v := doc["port"].(type) {
		case int32:
			port = int(v)
		case int64:
			port = int(v)
		case int:
			port = v
		}
		if userID == "" || host == "" || port == 0 {
			continue
		}
		if pingOpencodeInstance(ctx, host, port) {
			handleHealthSuccess(ctx, userID)
			active++
		} else {
			handleHealthFailure(ctx, userID)
			down++
		}
	}
	log.Printf("opencode healthcheck: tick 완료 active=%d down/pending=%d", active, down)
	return active, down
}

func pingOpencodeInstance(ctx context.Context, host string, port int) bool {
	url := "http://" + host + ":" + strconv.Itoa(port) + "/global/health"
	reqCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return false
	}
	// OpenCode 서버는 password 가 설정돼 있으면 Basic auth 를 요구. 단, /global/health
	// 는 공개 endpoint 라 인증 없어도 200 응답 — 안전하게 인증 헤더는 옵션으로 첨부.
	if pw := os.Getenv("OPENCODE_SERVER_PASSWORD"); pw != "" {
		user := os.Getenv("OPENCODE_SERVER_USERNAME")
		if user == "" {
			user = "opencode"
		}
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pw))
		req.Header.Set("Authorization", "Basic "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func handleHealthSuccess(ctx context.Context, userID string) {
	failureCountsMu.Lock()
	prev := failureCounts[userID]
	delete(failureCounts, userID)
	failureCountsMu.Unlock()

	if err := db.UpdateOpencodeInstanceHeartbeat(ctx, userID); err != nil {
		log.Printf("opencode healthcheck: %s heartbeat 갱신 실패: %v", userID, err)
		return
	}
	if prev >= failureThreshold {
		log.Printf("opencode healthcheck: %s 회복 (이전 실패 %d회)", userID, prev)
	}
}

func handleHealthFailure(ctx context.Context, userID string) {
	failureCountsMu.Lock()
	failureCounts[userID]++
	count := failureCounts[userID]
	failureCountsMu.Unlock()

	if count < failureThreshold {
		log.Printf("opencode healthcheck: %s 실패 %d/%d", userID, count, failureThreshold)
		return
	}
	// threshold 도달 — status=down 마킹 (이미 down 이라도 idempotent)
	if err := db.MarkOpencodeInstanceDown(ctx, userID); err != nil {
		log.Printf("opencode healthcheck: %s down 마킹 실패: %v", userID, err)
		return
	}
	if count == failureThreshold {
		log.Printf("opencode healthcheck: %s status=down 마킹 (3회 연속 실패)", userID)
	}
}

// RunOpencodeHealthcheckHandler 운영자가 즉시 한 번 실행 트리거.
//   POST /admin/opencode-healthcheck
// 응답: {"active": N, "down": M}
func RunOpencodeHealthcheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	active, down := runOpencodeHealthcheck(ctx)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{"active": active, "down": down})
}
