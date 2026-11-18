package handlers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	authmw "platform-api/auth"
	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
)

// 과제 내보내기 / 가져오기.
//
// 목적은 "과제 하나를 다른 GoJIRA 시스템으로 옮기기"다. 그래서 묶음은 한 파일이고,
// 안에는 이 시스템에 의존하는 값(저장소 URL 등)이 아니라 복원에 필요한 내용만 담는다.
//
//	manifest.json                 schemaVersion · 원본 코드/이름/시스템 · 요약
//	mongo/dump.json               { project, issues }  (GoJIRA-API /Project/{code}/dump)
//	mongo/usecase-artifacts.json  유즈케이스 산출물 (없으면 생략)
//	repo/<repoName>.bundle        git bundle --all  — 전 브랜치·전체 이력
//
// 가져오는 쪽은 저장소 URL 을 자기 Gitea 기준으로 새로 만든다. 과제 코드를 바꿔
// 받으면 이슈 키(OLD-12 → NEW-12)를 Mongo 와 저장소 YAML 양쪽에서 다시 쓴다.

const archiveSchemaVersion = 2

// maxImportBytes 업로드 상한. bundle 이 들어가도 과제 하나는 수 MB 수준이다.
const maxImportBytes = 256 << 20 // 256MB

// transferManifest zip 의 manifest.json.
type transferManifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Code          string              `json:"code"`
	Name          string              `json:"name"`
	CreatedAt     string              `json:"createdAt"`
	CreatedBy     string              `json:"createdBy"`
	SourceSystem  string              `json:"sourceSystem"` // 원본 Gitea base URL (추적용)
	RepoOwner     string              `json:"repoOwner"`
	RepoName      string              `json:"repoName"`
	DefaultBranch string              `json:"defaultBranch"`
	IssueCount    int                 `json:"issueCount"`
	BundlePath    string              `json:"bundlePath"` // zip 안의 경로
	Repo          *github.BundleStats `json:"repo,omitempty"`
}

// archiveBuild 내보내기/삭제백업이 공유하는 결과물.
type archiveBuild struct {
	ZipPath  string
	Manifest transferManifest
	Cleanup  func()
}

// buildProjectArchive 과제 하나를 zip 한 벌로 묶는다. 호출자가 Cleanup 을 불러야 한다.
//
// 저장소 bundle 이 실패하면 에러로 올린다 — 문서 없는 묶음을 "성공"으로 돌려주면
// 삭제 백업 경로에서 복구 불가능한 손실이 된다(2026-11-18 발견: Gitea 이관 후
// config.RepoBaseDir 이 비어 bare 복사가 조용히 건너뛰어지고 있었다).
func buildProjectArchive(ctx context.Context, code, user string) (*archiveBuild, error) {
	dumpData, err := callGoJira(ctx, "GET", fmt.Sprintf("/Project/%s/dump", code), nil, user)
	if err != nil {
		return nil, fmt.Errorf("dump 호출 실패: %w", err)
	}
	var dump struct {
		Project map[string]interface{}   `json:"project"`
		Issues  []map[string]interface{} `json:"issues"`
	}
	if err := json.Unmarshal(dumpData, &dump); err != nil {
		return nil, fmt.Errorf("dump 파싱 실패: %w", err)
	}

	prj, prjErr := db.FindProjectByCode(ctx, code)
	if prjErr != nil || prj == nil {
		return nil, fmt.Errorf("project %q not found", code)
	}
	docs := db.BsonGetDoc(prj, "documents")
	repoOwner := db.BsonGetStr(docs, "repoOwner")
	repoName := db.BsonGetStr(docs, "repoName")
	defaultBranch := db.BsonGetStr(docs, "defaultBranch")
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if repoOwner == "" || repoName == "" {
		return nil, fmt.Errorf("과제 %s 에 설계문서 저장소 정보가 없다 (repoOwner/repoName 비어 있음)", code)
	}
	projectName, _ := prj["name"].(string)

	tmpDir, err := os.MkdirTemp("", "gojira-export-")
	if err != nil {
		return nil, fmt.Errorf("임시 디렉터리 생성 실패: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	bundleName := repoName + ".bundle"
	bundlePath := filepath.Join(tmpDir, bundleName)
	stats, err := github.BundleRepo(repoOwner, repoName, bundlePath, defaultBranch)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("설계문서 저장소 묶기 실패: %w", err)
	}

	man := transferManifest{
		SchemaVersion: archiveSchemaVersion,
		Code:          code,
		Name:          projectName,
		CreatedAt:     time.Now().Format(time.RFC3339),
		CreatedBy:     user,
		SourceSystem:  strings.TrimRight(config.GitHubURL, "/"),
		RepoOwner:     repoOwner,
		RepoName:      repoName,
		DefaultBranch: defaultBranch,
		IssueCount:    len(dump.Issues),
		BundlePath:    "repo/" + bundleName,
		Repo:          stats,
	}

	zipPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%s.zip", code, time.Now().Format("20060102150405")))
	zf, err := os.Create(zipPath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("zip 생성 실패: %w", err)
	}
	zw := zip.NewWriter(zf)
	fail := func(e error) (*archiveBuild, error) {
		_ = zw.Close()
		_ = zf.Close()
		cleanup()
		return nil, e
	}

	manBytes, _ := json.MarshalIndent(man, "", "  ")
	if err := writeZipBytes(zw, "manifest.json", manBytes); err != nil {
		return fail(fmt.Errorf("zip manifest: %w", err))
	}
	if err := writeZipBytes(zw, "mongo/dump.json", dumpData); err != nil {
		return fail(fmt.Errorf("zip dump: %w", err))
	}
	// 유즈케이스 산출물은 과제에 따라 없을 수 있다 — 없으면 조용히 건너뛴다.
	if ua, err := callGoJira(ctx, "GET", fmt.Sprintf("/Project/%s/usecase-artifacts", code), nil, user); err == nil && len(ua) > 0 {
		if err := writeZipBytes(zw, "mongo/usecase-artifacts.json", ua); err != nil {
			return fail(fmt.Errorf("zip usecase-artifacts: %w", err))
		}
	}
	bf, err := os.Open(bundlePath)
	if err != nil {
		return fail(fmt.Errorf("bundle 열기 실패: %w", err))
	}
	zEntry, err := zw.Create(man.BundlePath)
	if err != nil {
		_ = bf.Close()
		return fail(fmt.Errorf("zip bundle entry: %w", err))
	}
	if _, err := io.Copy(zEntry, bf); err != nil {
		_ = bf.Close()
		return fail(fmt.Errorf("zip bundle 복사 실패: %w", err))
	}
	_ = bf.Close()

	if err := zw.Close(); err != nil {
		_ = zf.Close()
		cleanup()
		return nil, fmt.Errorf("zip close: %w", err)
	}
	if err := zf.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("file close: %w", err)
	}
	return &archiveBuild{ZipPath: zipPath, Manifest: man, Cleanup: cleanup}, nil
}

// GET /api/v{version}/Project/{code}/export — 과제 한 벌을 zip 으로 내려준다.
// 아무것도 지우거나 바꾸지 않는다 (삭제 백업과 다른 점).
func ProjectExportHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "code path param required")
		return
	}
	user := authmw.UserID(r.Context())
	if user == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	defer cancel()

	build, err := buildProjectArchive(ctx, code, user)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer build.Cleanup()

	f, err := os.Open(build.ZipPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "zip 열기 실패: "+err.Error())
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "zip stat 실패: "+err.Error())
		return
	}

	filename := fmt.Sprintf("%s-%s.gojira.zip", code, time.Now().Format("20060102-1504"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
	// 브라우저 fetch 에서 요약을 읽을 수 있게 — 본문은 바이너리라 헤더로 싣는다.
	w.Header().Set("X-Gojira-Issue-Count", fmt.Sprintf("%d", build.Manifest.IssueCount))
	if build.Manifest.Repo != nil {
		w.Header().Set("X-Gojira-File-Count", fmt.Sprintf("%d", build.Manifest.Repo.FileCount))
	}
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, X-Gojira-Issue-Count, X-Gojira-File-Count")
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("export 스트리밍 중단 (%s): %v", code, err)
	}
}

// ---------------------------------------------------------------- 가져오기

// importPayload zip 에서 꺼낸 내용. 신뢰할 수 없는 입력이므로 경로는 쓰지 않고
// 우리가 정한 임시 경로에만 쓴다 (zip-slip 차단).
type importPayload struct {
	Manifest   transferManifest
	Project    map[string]interface{}
	Issues     []map[string]interface{}
	Usecase    json.RawMessage
	BundlePath string
	Cleanup    func()
}

var codeRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,19}$`)

// readImportZip 업로드된 zip 을 열어 내용을 꺼낸다.
func readImportZip(file multipart.File) (*importPayload, error) {
	tmpDir, err := os.MkdirTemp("", "gojira-import-")
	if err != nil {
		return nil, fmt.Errorf("임시 디렉터리 생성 실패: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	zipPath := filepath.Join(tmpDir, "upload.zip")
	dst, err := os.Create(zipPath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("업로드 저장 실패: %w", err)
	}
	written, err := io.Copy(dst, io.LimitReader(file, maxImportBytes+1))
	_ = dst.Close()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("업로드 읽기 실패: %w", err)
	}
	if written > maxImportBytes {
		cleanup()
		return nil, fmt.Errorf("파일이 너무 크다 (상한 %dMB)", maxImportBytes>>20)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("zip 열기 실패 — GoJIRA 내보내기 파일이 맞는지 확인: %w", err)
	}
	defer zr.Close()

	p := &importPayload{Cleanup: cleanup}
	var bundleEntry *zip.File
	for _, f := range zr.File {
		// 이름은 비교에만 쓴다. 경로로 사용하지 않으므로 ../ 가 섞여도 무해하다.
		name := path.Clean(f.Name)
		switch {
		case name == "manifest.json":
			b, err := readZipEntry(f)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("manifest 읽기 실패: %w", err)
			}
			if err := json.Unmarshal(b, &p.Manifest); err != nil {
				cleanup()
				return nil, fmt.Errorf("manifest 파싱 실패: %w", err)
			}
		case name == "mongo/dump.json":
			b, err := readZipEntry(f)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("dump 읽기 실패: %w", err)
			}
			var dump struct {
				Project map[string]interface{}   `json:"project"`
				Issues  []map[string]interface{} `json:"issues"`
			}
			if err := json.Unmarshal(b, &dump); err != nil {
				cleanup()
				return nil, fmt.Errorf("dump 파싱 실패: %w", err)
			}
			p.Project, p.Issues = dump.Project, dump.Issues
		case name == "mongo/usecase-artifacts.json":
			b, err := readZipEntry(f)
			if err == nil {
				p.Usecase = b
			}
		case strings.HasPrefix(name, "repo/") && strings.HasSuffix(name, ".bundle"):
			bundleEntry = f
		}
	}

	if p.Manifest.SchemaVersion == 0 {
		cleanup()
		return nil, fmt.Errorf("manifest.json 이 없다 — GoJIRA 과제 내보내기 파일이 아니다")
	}
	if p.Manifest.SchemaVersion > archiveSchemaVersion {
		cleanup()
		return nil, fmt.Errorf("더 새로운 형식이다 (schemaVersion %d > %d) — 이 시스템을 먼저 올려야 한다",
			p.Manifest.SchemaVersion, archiveSchemaVersion)
	}
	if p.Project == nil {
		cleanup()
		return nil, fmt.Errorf("mongo/dump.json 이 없다 — 과제 정보를 복원할 수 없다")
	}
	if bundleEntry == nil {
		cleanup()
		return nil, fmt.Errorf("설계문서 묶음(repo/*.bundle)이 없다 — schemaVersion 1 묶음은 문서가 빠져 있어 가져올 수 없다")
	}

	// bundle 은 우리가 정한 이름으로만 쓴다 — zip 안의 경로를 신뢰하지 않는다.
	p.BundlePath = filepath.Join(tmpDir, "repo.bundle")
	rc, err := bundleEntry.Open()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("bundle 열기 실패: %w", err)
	}
	bf, err := os.Create(p.BundlePath)
	if err != nil {
		_ = rc.Close()
		cleanup()
		return nil, fmt.Errorf("bundle 저장 실패: %w", err)
	}
	_, copyErr := io.Copy(bf, io.LimitReader(rc, maxImportBytes))
	_ = rc.Close()
	_ = bf.Close()
	if copyErr != nil {
		cleanup()
		return nil, fmt.Errorf("bundle 복사 실패: %w", copyErr)
	}
	if _, err := github.VerifyBundle(p.BundlePath); err != nil {
		cleanup()
		return nil, err
	}
	return p, nil
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 64<<20))
}

// POST /api/v{version}/Project/import/inspect — zip 을 읽어 내용만 알려준다.
// 아무것도 만들지 않는다. 다이얼로그가 폼을 채우는 데 쓴다.
func ProjectImportInspectHandler(w http.ResponseWriter, r *http.Request) {
	if authmw.UserID(r.Context()) == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	file, _, err := openUploadedFile(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer file.Close()

	p, err := readImportZip(file)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer p.Cleanup()

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	// 이 시스템에서 원본 코드/저장소명을 그대로 쓸 수 있는지 미리 알려준다.
	codeTaken := false
	if existing, _ := db.FindProjectByCode(ctx, p.Manifest.Code); existing != nil {
		codeTaken = true
	}
	repoTaken := false
	if p.Manifest.RepoName != "" {
		if exists, err := github.RepoExists(config.GitHubOrg, p.Manifest.RepoName); err == nil {
			repoTaken = exists
		}
	}

	resp := map[string]interface{}{
		"manifest":      p.Manifest,
		"issueCount":    len(p.Issues),
		"codeTaken":     codeTaken,
		"repoNameTaken": repoTaken,
		"suggested": map[string]interface{}{
			"name":     p.Manifest.Name,
			"code":     p.Manifest.Code,
			"repoName": p.Manifest.RepoName,
		},
		"project": sanitizeImportedProject(p.Project),
	}
	respondJSON(w, http.StatusOK, resp)
}

// sanitizeImportedProject 다이얼로그 폼을 채우기 위한 필드만 추린다.
// documents(저장소 URL 등)·_id 는 이 시스템 기준으로 새로 만들 값이라 넘기지 않는다.
func sanitizeImportedProject(prj map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, k := range []string{
		"name", "status", "devTeam", "devGroup", "feature",
		"architect", "PL", "members", "auditors", "schedules", "issueQuery",
	} {
		if v, ok := prj[k]; ok {
			out[k] = v
		}
	}
	return out
}

// POST /api/v{version}/Project/import — 실제 복원.
//
// 흐름:
//  1. zip 검증 + 목표 코드/이름/저장소명 확인
//  2. Gitea 저장소 생성 → bundle push (코드가 바뀌면 YAML 키 재작성) → 팀/웹훅/브랜치보호
//  3. GoJIRA-API POST /Project/{code}/restore 로 Mongo 복원(프로젝트·이슈·카운터·산출물)
//
// 2 가 실패하면 만든 저장소를 되돌린다. 3 이 실패하면 저장소는 남기고 에러를 올린다
// (저장소만 있는 상태는 사용자가 같은 코드로 재시도하면 복구 가능하다).
func ProjectImportHandler(w http.ResponseWriter, r *http.Request) {
	user := authmw.UserID(r.Context())
	if user == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	file, form, err := openUploadedFile(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer file.Close()

	p, err := readImportZip(file)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer p.Cleanup()

	targetCode := strings.ToUpper(strings.TrimSpace(formValue(form, "code", p.Manifest.Code)))
	targetName := strings.TrimSpace(formValue(form, "name", p.Manifest.Name))
	targetRepo := slugifyRepoName(formValue(form, "repoName", p.Manifest.RepoName))

	if !codeRe.MatchString(targetCode) {
		respondError(w, http.StatusBadRequest, "과제 코드는 영문 대문자로 시작하는 2~20자(대문자/숫자/밑줄)여야 한다: "+targetCode)
		return
	}
	if targetName == "" {
		respondError(w, http.StatusBadRequest, "과제명이 비어 있다")
		return
	}
	if targetRepo == "" {
		respondError(w, http.StatusBadRequest, "Doc Repo 이름이 비어 있다 (영문/숫자/하이픈)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
	defer cancel()

	if existing, _ := db.FindProjectByCode(ctx, targetCode); existing != nil {
		respondError(w, http.StatusConflict, fmt.Sprintf("과제 코드 %s 가 이미 있다 — 다른 코드로 가져오기", targetCode))
		return
	}
	if exists, err := github.RepoExists(config.GitHubOrg, targetRepo); err != nil {
		respondError(w, http.StatusBadGateway, "저장소 중복 검사 실패: "+err.Error())
		return
	} else if exists {
		respondError(w, http.StatusConflict, fmt.Sprintf("저장소 %s 가 이미 있다 — 다른 이름으로 가져오기", targetRepo))
		return
	}

	// --- 2. Gitea 저장소 ---
	desc := fmt.Sprintf("%s의 설계 문서 (가져온 과제: %s)", targetName, p.Manifest.Code)
	ghRepo, err := github.CreateRepo(targetRepo, desc, true)
	if err != nil {
		respondError(w, http.StatusBadGateway, "저장소 생성 실패: "+err.Error())
		return
	}
	owner := config.GitHubOrg
	if owner == "" {
		if o, ok := ghRepo["owner"].(map[string]interface{}); ok {
			owner, _ = o["login"].(string)
		}
	}
	repoURL, _ := ghRepo["html_url"].(string)
	rollbackRepo := func() {
		if err := github.DeleteRepo(owner, targetRepo); err != nil {
			log.Printf("import 롤백: 저장소 삭제 실패 (%s/%s): %v", owner, targetRepo, err)
		}
	}

	// 코드가 바뀌면 저장소 안의 이슈 YAML 키도 같이 바뀌어야 한다.
	// 안 바꾸면 "내 작업본" 뷰가 옛 키를 들고 있어 공식본과 어긋난다.
	var rewrite github.RewriteFunc
	codeChanged := targetCode != p.Manifest.Code
	if codeChanged {
		rewrite = makeKeyRewriter(p.Manifest.Code, targetCode)
	}
	msg := fmt.Sprintf("가져오기: 이슈 키 %s- → %s-", p.Manifest.Code, targetCode)
	branches, err := github.PushBundleToRepo(p.BundlePath, owner, targetRepo, msg, rewrite)
	if err != nil {
		rollbackRepo()
		respondError(w, http.StatusBadGateway, "설계문서 복원 실패: "+err.Error())
		return
	}

	if err := github.AddRepoToOrgTeam(targetRepo); err != nil {
		log.Printf("import: 조직 팀에 repo 추가 실패: %v", err)
	}
	if config.WebhookURL != "" {
		if err := github.CreateWebhook(owner, targetRepo, config.WebhookURL, config.WebhookSecret,
			[]string{"pull_request", "pull_request_review", "push"}); err != nil {
			log.Printf("import: 웹훅 등록 실패: %v", err)
		}
	}
	// 브랜치 보호는 push 가 끝난 뒤에 — 먼저 걸면 bundle push 가 거부된다.
	reviewCount := 1
	if a := toStringSlice(p.Project["auditors"]); len(a) > 0 {
		reviewCount = len(a)
	}
	if err := github.SetBranchProtection(owner, targetRepo, "main", reviewCount); err != nil {
		log.Printf("import: 브랜치 보호 설정 실패 (나중에 재시도 가능): %v", err)
	}

	// --- 3. Mongo 복원 (GoJIRA-API) ---
	restoreBody, _ := json.Marshal(map[string]interface{}{
		"sourceCode": p.Manifest.Code,
		"name":       targetName,
		"project":    sanitizeImportedProject(p.Project),
		"archSettings": func() interface{} {
			if v, ok := p.Project["archSettings"]; ok {
				return v
			}
			return nil
		}(),
		"issues": p.Issues,
		"documents": map[string]interface{}{
			"defaultBranch": "main",
			"repoName":      targetRepo,
			"repoOwner":     owner,
			"repoUrl":       repoURL,
		},
		"usecaseArtifacts": p.Usecase,
	})
	restoreResp, err := callGoJira(ctx, "POST", fmt.Sprintf("/Project/%s/restore", targetCode), restoreBody, user)
	if err != nil {
		respondError(w, http.StatusBadGateway,
			fmt.Sprintf("저장소는 만들어졌으나 Mongo 복원이 실패했다 (%s/%s 는 남아 있다): %v", owner, targetRepo, err))
		return
	}
	var restoreResult map[string]interface{}
	_ = json.Unmarshal(restoreResp, &restoreResult)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"code":         targetCode,
		"name":         targetName,
		"sourceCode":   p.Manifest.Code,
		"sourceSystem": p.Manifest.SourceSystem,
		"keyRewritten": codeChanged,
		"repoOwner":    owner,
		"repoName":     targetRepo,
		"repoUrl":      repoURL,
		"branches":     branches,
		"issueCount":   len(p.Issues),
		"restore":      restoreResult,
	})
}

// makeKeyRewriter 작업 트리의 추적 파일에서 OLD-<숫자> 를 NEW-<숫자> 로 바꾼다.
//
// 키만 바꾼다 — 산문에 쓰인 제품명은 사용자 판단 영역이라 건드리지 않는다.
// 바이너리 파일은 건너뛴다.
func makeKeyRewriter(oldCode, newCode string) github.RewriteFunc {
	re := regexp.MustCompile(regexp.QuoteMeta(oldCode) + `-(\d)`)
	repl := newCode + "-$1"
	return func(worktree string) (bool, error) {
		out, err := github.GitCmd(worktree, "ls-files", "-z")
		if err != nil {
			return false, fmt.Errorf("ls-files 실패: %s", out)
		}
		changed := false
		for _, rel := range strings.Split(out, "\x00") {
			if rel == "" {
				continue
			}
			full := filepath.Join(worktree, rel)
			info, err := os.Lstat(full)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			b, err := os.ReadFile(full)
			if err != nil {
				return false, fmt.Errorf("%s 읽기 실패: %w", rel, err)
			}
			if isBinary(b) {
				continue
			}
			nb := re.ReplaceAll(b, []byte(repl))
			if string(nb) == string(b) {
				continue
			}
			if err := os.WriteFile(full, nb, info.Mode().Perm()); err != nil {
				return false, fmt.Errorf("%s 쓰기 실패: %w", rel, err)
			}
			changed = true
		}
		return changed, nil
	}
}

// isBinary NUL 바이트가 있으면 바이너리로 본다 (git 과 같은 휴리스틱).
func isBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}

func openUploadedFile(r *http.Request) (multipart.File, *multipart.Form, error) {
	// 상한을 전송 계층에서 먼저 막는다 — 핸들러가 디스크로 spool 한 뒤 거절하면
	// 그만큼은 이미 받아 쓴 것이 된다.
	r.Body = http.MaxBytesReader(nil, r.Body, maxImportBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, fmt.Errorf("multipart 파싱 실패: %w", err)
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		return nil, nil, fmt.Errorf("file 필드가 없다 (zip 업로드 필요): %w", err)
	}
	return f, r.MultipartForm, nil
}

func formValue(form *multipart.Form, key, fallback string) string {
	if form != nil {
		if vs := form.Value[key]; len(vs) > 0 && strings.TrimSpace(vs[0]) != "" {
			return vs[0]
		}
	}
	return fallback
}
