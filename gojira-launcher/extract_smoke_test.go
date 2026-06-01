package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractProvisionsAllAgentAssets — embed 에서 .claude/.opencode/opencode.json 가
// 모두 추출되는지(opencode 까지 커버) 스모크 검증. 개발 빌드(임베드 없음)면 스킵.
func TestExtractProvisionsAllAgentAssets(t *testing.T) {
	if !hasEmbeddedSkills() {
		t.Skip("임베드 자산 없음 — assets 동기화 후 빌드한 경우만 검증")
	}
	dest := t.TempDir()
	if err := ExtractSkills(dest, "test", true); err != nil {
		t.Fatalf("ExtractSkills: %v", err)
	}
	for _, p := range []string{
		filepath.Join(dest, ".claude", "skills"),
		filepath.Join(dest, ".opencode", "agents", "gojira.md"),
		filepath.Join(dest, "opencode.json"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("provision 누락: %s (%v)", p, err)
		}
	}
}
