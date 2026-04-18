// @ts-check
/**
 * TC2: 문서 이력관리 및 PR 알림 흐름
 *
 * 사전 조건:
 *   - Platform-App(:3000), GoJIRA-App(:3001), Platform-API(:9000), GoJIRA-API(:9001) 가동
 *   - Gitea 정상 동작
 *   - TC1(tc1-project-create.spec.js) 성공 — E2E-TEST-PRJ 프로젝트 + DocRepo 존재
 *
 * 브라우저 A: duke.kimm (리뷰어)
 * 브라우저 B: batman (작성자)
 */
const { test, expect } = require('@playwright/test');
const {
  dismissModals,
  switchUser,
  clickProject,
  clickNavTab,
  clickAppNav,
  waitForRemoteLoad,
} = require('./helpers');

const PROJECT_NAME = 'E2E-TEST-PRJ';

test.describe.serial('TC2: 문서 이력관리 및 PR 알림 흐름', () => {
  /** @type {import('@playwright/test').Page} */
  let pageA; // duke.kimm (리뷰어)
  /** @type {import('@playwright/test').Page} */
  let pageB; // batman (작성자)
  /** @type {import('@playwright/test').BrowserContext} */
  let contextA;
  /** @type {import('@playwright/test').BrowserContext} */
  let contextB;

  test.beforeAll(async ({ browser }) => {
    // 두 개의 독립 브라우저 컨텍스트 생성 (별도 세션 = 별도 쿠키)
    contextA = await browser.newContext();
    contextB = await browser.newContext();
    pageA = await contextA.newPage();
    pageB = await contextB.newPage();
  });

  test.afterAll(async () => {
    await contextA.close();
    await contextB.close();
  });

  // ─────────────────────────────────────────────────
  // Step 1: 초기 상태 — 두 브라우저에서 ProjectMap 진입
  // ─────────────────────────────────────────────────
  test('Step 1: 두 브라우저에서 ProjectMap 진입 및 사용자 전환', async () => {
    // A: duke.kimm 으로 전환
    await pageA.goto('http://localhost:3000');
    await waitForRemoteLoad(pageA);
    await dismissModals(pageA);
    await switchUser(pageA, 'duke.kimm');
    await waitForRemoteLoad(pageA);
    await dismissModals(pageA);

    // B: batman 으로 전환
    await pageB.goto('http://localhost:3000');
    await waitForRemoteLoad(pageB);
    await dismissModals(pageB);
    await switchUser(pageB, 'batman');
    await waitForRemoteLoad(pageB);
    await dismissModals(pageB);

    // 검증: 양쪽 모두 ProjectMap 표시 (+ Add 버튼 존재)
    await expect(pageA.locator('text=+ Add')).toBeVisible({ timeout: 10_000 });
    await expect(pageB.locator('text=+ Add')).toBeVisible({ timeout: 10_000 });

    // 검증: 프로젝트 목록에 E2E-TEST-PRJ 존재
    await expect(pageA.locator('.pm-sidebar-row', { hasText: PROJECT_NAME })).toBeVisible();
    await expect(pageB.locator('.pm-sidebar-row', { hasText: PROJECT_NAME })).toBeVisible();
  });

  // ─────────────────────────────────────────────────
  // Step 2: 작업공간 동기화 (Pull)
  // ─────────────────────────────────────────────────
  test('Step 2-A: duke.kimm — DocEditor 진입 후 Pull 동기화', async () => {
    // E2E-TEST-PRJ 클릭 → Issues 페이지로 이동
    await clickProject(pageA, PROJECT_NAME);

    // Docs 탭 클릭 → DocEditor 진입
    await clickNavTab(pageA, 'Docs');

    // 작업공간 탭 확인
    const wsTab = pageA.locator('.de-sidebar-tab', { hasText: '작업공간' });
    await expect(wsTab).toBeVisible({ timeout: 5000 });
    await wsTab.click();
    await pageA.waitForTimeout(500);

    // Pull 버튼 클릭 (title="Pull")
    const pullBtn = pageA.locator('button[title="Pull"]');
    await expect(pullBtn).toBeVisible({ timeout: 5000 });
    await pullBtn.click();

    // 확인 다이얼로그 — "main 브랜치의 최신 내용을 작업공간에 동기화" 확인
    const confirmBtn = pageA.locator('button', { hasText: '확인' });
    await expect(confirmBtn).toBeVisible({ timeout: 5000 });
    await confirmBtn.click();

    // "동기화 완료" 알림 확인 → 닫기
    await pageA.waitForTimeout(1500);
    await dismissModals(pageA, 'confirm');
    await dismissModals(pageA);

    // Project 탭 클릭 → ProjectMap 복귀
    await clickAppNav(pageA, 'Project');
    await waitForRemoteLoad(pageA);
    await dismissModals(pageA);

    // 검증: ProjectMap 복귀
    await expect(pageA.locator('text=+ Add')).toBeVisible({ timeout: 10_000 });
  });

  test('Step 2-B: batman — DocEditor 진입 후 Pull 동기화', async () => {
    await clickProject(pageB, PROJECT_NAME);
    await clickNavTab(pageB, 'Docs');

    const wsTab = pageB.locator('.de-sidebar-tab', { hasText: '작업공간' });
    await expect(wsTab).toBeVisible({ timeout: 5000 });
    await wsTab.click();
    await pageB.waitForTimeout(500);

    const pullBtn = pageB.locator('button[title="Pull"]');
    await expect(pullBtn).toBeVisible({ timeout: 5000 });
    await pullBtn.click();

    const confirmBtn = pageB.locator('button', { hasText: '확인' });
    await expect(confirmBtn).toBeVisible({ timeout: 5000 });
    await confirmBtn.click();

    await pageB.waitForTimeout(1500);
    await dismissModals(pageB, 'confirm');
    await dismissModals(pageB);

    await clickAppNav(pageB, 'Project');
    await waitForRemoteLoad(pageB);
    await dismissModals(pageB);

    await expect(pageB.locator('text=+ Add')).toBeVisible({ timeout: 10_000 });
  });

  // ─────────────────────────────────────────────────
  // Step 3: 문서 편집 및 저장 (B: batman)
  // ─────────────────────────────────────────────────
  test('Step 3: batman — 문서 편집 및 저장', async () => {
    // E2E-TEST-PRJ 클릭 → Docs 탭
    await clickProject(pageB, PROJECT_NAME);
    await clickNavTab(pageB, 'Docs');

    // 작업공간 탭 클릭
    const wsTab = pageB.locator('.de-sidebar-tab', { hasText: '작업공간' });
    await wsTab.click();
    await pageB.waitForTimeout(500);

    // 편집모드 ON 클릭
    const editToggle = pageB.locator('.cb-edit-toggle');
    await expect(editToggle).toBeVisible({ timeout: 5000 });
    await editToggle.click();
    await pageB.waitForTimeout(300);

    // 검증: 편집모드 ON 상태
    await expect(editToggle).toHaveText('편집모드 ON');
    await expect(editToggle).toHaveClass(/cb-edit-active/);

    // 검증: 발행신청 버튼 표시
    await expect(pageB.locator('button', { hasText: '발행신청' })).toBeVisible();

    // 첫 번째 파일 클릭
    const firstFile = pageB.locator('.de-file').first();
    await expect(firstFile).toBeVisible({ timeout: 5000 });
    const fileName = await firstFile.locator('.de-file-name').textContent();
    await firstFile.click();
    await pageB.waitForTimeout(500);

    // 편집 버튼 클릭 (de-toolbar 내)
    const editBtn = pageB.locator('.de-actions .de-btn', { hasText: '편집' });
    await expect(editBtn).toBeVisible({ timeout: 5000 });
    await editBtn.click();
    await pageB.waitForTimeout(300);

    // 텍스트 편집 (textarea에 내용 추가)
    const textarea = pageB.locator('.de-textarea');
    await expect(textarea).toBeVisible({ timeout: 5000 });
    const original = await textarea.inputValue();
    const timestamp = new Date().toISOString();
    await textarea.fill(original + `\n\n<!-- E2E 테스트 수정: ${timestamp} -->`);

    // 검증: 변경됨 표시 (●)
    await expect(pageB.locator('.de-modified')).toBeVisible({ timeout: 3000 });

    // 저장 버튼 클릭
    const saveBtn = pageB.locator('.de-actions .de-btn', { hasText: '저장' });
    await expect(saveBtn).toBeEnabled();
    await saveBtn.click();

    // "저장(커밋)하시겠습니까?" 확인
    const confirmBtn = pageB.locator('button', { hasText: '확인' });
    await expect(confirmBtn).toBeVisible({ timeout: 5000 });
    await confirmBtn.click();

    // "저장 완료" 알림 확인
    await pageB.waitForTimeout(2000);
    await dismissModals(pageB, 'confirm');
    await dismissModals(pageB);

    // 검증: 발행신청 버튼이 활성화 (de-btn-primary 클래스)
    // 저장 후 fileStatus가 비동기로 갱신되므로 충분한 타임아웃
    const prBtn = pageB.locator('button', { hasText: '발행신청' });
    await expect(prBtn).toHaveClass(/de-btn-primary/, { timeout: 10_000 });
    await expect(prBtn).toBeEnabled();
  });

  // ─────────────────────────────────────────────────
  // Step 4: 발행신청 — PR 생성 (B: batman)
  // ─────────────────────────────────────────────────
  test('Step 4: batman — 발행신청(PR 생성)', async () => {
    // 발행신청 버튼 클릭
    const prBtn = pageB.locator('button', { hasText: '발행신청' });
    await expect(prBtn).toBeVisible({ timeout: 5000 });
    await prBtn.click();

    // "발행 신청(PR)하시겠습니까?" 확인 다이얼로그
    const confirmBtn = pageB.locator('button', { hasText: '확인' });
    await expect(confirmBtn).toBeVisible({ timeout: 5000 });
    await confirmBtn.click();

    // PR 생성 성공 — 알림 대기 후 닫기
    await pageB.waitForTimeout(2000);
    await dismissModals(pageB, 'confirm');
    await dismissModals(pageB);

    // 검증: PR 생성 후 발행신청 버튼이 비활성화되거나, 변경사항이 해소됨
    // (PR 생성 후에도 변경 사항이 남아있을 수 있으므로 에러 메시지 없는 것으로 확인)
    await expect(pageB.locator('text=GoJIRA-App에 연결할 수 없습니다')).not.toBeVisible();
  });

  // ─────────────────────────────────────────────────
  // Step 5+6: PR 확인 (A: duke.kimm — Repository에서 PR 도착 확인)
  // ─────────────────────────────────────────────────
  test('Step 5: duke.kimm — Repository에서 PR 확인', async () => {
    // A의 ProjectMap 새로고침
    await pageA.goto('http://localhost:3000');
    await waitForRemoteLoad(pageA);
    await dismissModals(pageA);

    // Repository 탭 클릭
    await clickAppNav(pageA, 'Repository');
    await pageA.waitForTimeout(1500);

    // DocManager: 저장소 목록에서 E2E-TEST 관련 항목 선택
    const repoItems = pageA.locator('.dm-repo-item');
    const count = await repoItems.count();

    let found = false;
    for (let i = 0; i < count; i++) {
      const text = await repoItems.nth(i).textContent();
      if (text && /e2e-test/i.test(text)) {
        await repoItems.nth(i).click();
        found = true;
        break;
      }
    }

    // 저장소가 없으면 첫 번째 항목이라도 선택
    if (!found && count > 0) {
      await repoItems.first().click();
    }
    await pageA.waitForTimeout(1000);

    // PRs 탭 클릭 — 저장소 선택 후 dm-view-tabs 내부에 노출
    const prTab = pageA.locator('.dm-view-tabs .dm-tab', { hasText: 'PRs' });
    await expect(prTab).toBeVisible({ timeout: 5000 });
    await prTab.click();
    await pageA.waitForTimeout(1500);

    // 검증: PR 목록에서 방금 생성된 PR 확인
    // title에 "[E2ETEST] 설계문서 배포 요청" 포함
    const prRow = pageA.locator('text=설계문서 배포 요청');
    await expect(prRow).toBeVisible({ timeout: 10_000 });

    // 검증: PR 작성자가 batman (PR 테이블 셀 내)
    await expect(pageA.locator('.dm-main td', { hasText: 'batman' })).toBeVisible();
  });
});
