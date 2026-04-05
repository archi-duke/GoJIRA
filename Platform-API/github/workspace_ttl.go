package github

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"platform-api/config"
)

// worktree 마지막 접근 시간 추적
var (
	wsAccess   = map[string]time.Time{}
	wsAccessMu sync.Mutex
)

// TouchWorkspace workspace 접근 시간 갱신
func TouchWorkspace(owner, repo, userID string) {
	key := owner + "/" + repo + "/" + userID
	wsAccessMu.Lock()
	defer wsAccessMu.Unlock()
	wsAccess[key] = time.Now()
}

// GetWorkspaceLastAccess workspace 마지막 접근 시간 조회
func GetWorkspaceLastAccess(owner, repo, userID string) time.Time {
	key := owner + "/" + repo + "/" + userID
	wsAccessMu.Lock()
	defer wsAccessMu.Unlock()
	if t, ok := wsAccess[key]; ok {
		return t
	}
	return time.Time{}
}

// RemoveWorkspaceAccess workspace 접근 기록 삭제
func removeWorkspaceAccess(owner, repo, userID string) {
	key := owner + "/" + repo + "/" + userID
	wsAccessMu.Lock()
	defer wsAccessMu.Unlock()
	delete(wsAccess, key)
}

// CleanExpiredWorkspaces 만료된 workspace 정리
// checkSessionFn: 활성 세션 여부를 확인하는 콜백 (nil이면 세션 체크 안 함)
func CleanExpiredWorkspaces(ttl time.Duration, checkSessionFn func(owner, repo, userID string) bool) int {
	wsAccessMu.Lock()
	// 스냅샷 복사
	snapshot := make(map[string]time.Time, len(wsAccess))
	for k, v := range wsAccess {
		snapshot[k] = v
	}
	wsAccessMu.Unlock()

	cleaned := 0
	cutoff := time.Now().Add(-ttl)

	for key, lastAccess := range snapshot {
		if lastAccess.After(cutoff) {
			continue
		}

		parts := splitKey(key)
		if parts == nil {
			continue
		}
		owner, repo, userID := parts[0], parts[1], parts[2]

		// 활성 세션이 있으면 건너뜀
		if checkSessionFn != nil && checkSessionFn(owner, repo, userID) {
			continue
		}

		// 미커밋 작업이 있으면 건너뜀 (사용자에게 경고는 UI 단에서 처리)
		if HasPendingWork(owner, repo, userID) {
			continue
		}

		wtDir := worktreeDir(owner, repo, userID)
		if _, err := os.Stat(filepath.Join(wtDir, ".git")); os.IsNotExist(err) {
			removeWorkspaceAccess(owner, repo, userID)
			continue
		}

		log.Printf("TTL 만료 workspace 삭제: %s/%s user=%s (last access: %s)", owner, repo, userID, lastAccess.Format(time.RFC3339))
		DiscardWorktree(owner, repo, userID)
		removeWorkspaceAccess(owner, repo, userID)
		cleaned++
	}

	return cleaned
}

func splitKey(key string) []string {
	// owner/repo/userID
	parts := make([]string, 0, 3)
	i := 0
	for j := 0; j < len(key); j++ {
		if key[j] == '/' {
			parts = append(parts, key[i:j])
			i = j + 1
		}
	}
	parts = append(parts, key[i:])
	if len(parts) != 3 {
		return nil
	}
	return parts
}

// StartTTLWorker 백그라운드에서 만료된 workspace를 주기적으로 정리
func StartTTLWorker(checkSessionFn func(owner, repo, userID string) bool) {
	ttl := config.EditSessionTTL
	interval := ttl / 2
	if interval < time.Minute {
		interval = time.Minute
	}

	go func() {
		log.Printf("Workspace TTL worker 시작 (TTL=%v, 점검주기=%v)", ttl, interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			cleaned := CleanExpiredWorkspaces(ttl, checkSessionFn)
			if cleaned > 0 {
				log.Printf("TTL worker: %d개 workspace 정리", cleaned)
			}
		}
	}()
}
