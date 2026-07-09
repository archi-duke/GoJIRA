package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	return dir
}

func resetTreeHashCache() {
	treeHashCacheMu.Lock()
	treeHashCache = map[string]map[string]cachedFileEntry{}
	treeHashCacheMu.Unlock()
}

func TestListLocalTreeCacheReuseAndInvalidation(t *testing.T) {
	resetTreeHashCache()
	dir := setupTestRepo(t)
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fileA, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries1, err := listLocalTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries1) != 2 {
		t.Fatalf("want 2 entries, got %d: %v", len(entries1), entries1)
	}
	shaByPath := map[string]string{}
	for _, e := range entries1 {
		shaByPath[e["path"].(string)] = e["sha"].(string)
	}
	if shaByPath["a.txt"] == "" || shaByPath["b.txt"] == "" {
		t.Fatalf("empty sha in %v", shaByPath)
	}

	// 캐시가 채워졌는지 확인 (white-box).
	treeHashCacheMu.Lock()
	cacheSize := len(treeHashCache[dir])
	treeHashCacheMu.Unlock()
	if cacheSize != 2 {
		t.Fatalf("want 2 cached entries after first call, got %d", cacheSize)
	}

	// mtime 이 확실히 달라지도록 대기 후 a.txt 내용만 변경.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(fileA, []byte("hello changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries2, err := listLocalTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	sha2ByPath := map[string]string{}
	for _, e := range entries2 {
		sha2ByPath[e["path"].(string)] = e["sha"].(string)
	}
	if sha2ByPath["a.txt"] == shaByPath["a.txt"] {
		t.Error("a.txt 변경 후에도 sha 가 그대로 — 캐시 무효화 실패")
	}
	if sha2ByPath["b.txt"] != shaByPath["b.txt"] {
		t.Error("변경 안 한 b.txt 의 sha 가 바뀜 — 캐시 오염")
	}

	// b.txt 삭제 후 캐시에서도 사라지는지(누적 방지) 확인.
	if err := os.Remove(fileB); err != nil {
		t.Fatal(err)
	}
	entries3, err := listLocalTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries3) != 1 {
		t.Fatalf("want 1 entry after delete, got %d: %v", len(entries3), entries3)
	}
	treeHashCacheMu.Lock()
	_, stillCached := treeHashCache[dir]["b.txt"]
	treeHashCacheMu.Unlock()
	if stillCached {
		t.Error("삭제된 b.txt 가 캐시에 남아있음")
	}
}
