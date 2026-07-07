package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// issueFile — 이슈 YAML 1개 (파일명 + raw 내용). readIssueFiles 의 결과 요소.
type issueFile struct {
	Name    string // 파일명 (예: VOS_001.yaml) — 디렉터리 prefix 없음.
	Content string // raw YAML 원문.
}

// readIssueFiles — repoPath/data/issues 하위의 *.yaml, *.yml 파일을 이름순으로 읽는다.
// handleIssues(GET /issues, 웹 로컬뷰) 가 쓴다.
// 디렉터리가 없으면 (nil, nil) — 호출측이 "이슈 없음"으로 구분해 처리한다.
func readIssueFiles(issuesDir string) ([]issueFile, error) {
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("issues 디렉터리 읽기: %w", err)
	}
	var files []issueFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(issuesDir, name))
		if err != nil {
			return nil, fmt.Errorf("%s 읽기: %w", name, err)
		}
		files = append(files, issueFile{Name: name, Content: string(b)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}
