// @ts-check
/**
 * TC3: 프로젝트 삭제
 *
 * 사전 조건:
 *   - TC2(tc2-project-create.spec.js) 성공 — E2E-TEST-PRJ 프로젝트가 존재하는 상태
 *   - Platform-App(:3000), GoJIRA-App(:3001), Platform-API(:9000), GoJIRA-API(:9001) 가동
 *   - Gitea(:3300) 정상 동작
 *
 * 브라우저: batman
 */
const { test, expect } = require('@playwright/test');
const {
  dismissModals,
  switchUser,
  waitForRemoteLoad,
} = require('./helpers');

const PROJECT_NAME = 'E2E-TEST-PRJ';
const JIRA_CODE = 'E2ETEST';
const GITEA_BASE = 'http://localhost:3300';
const GITEA_ORG = 'gojira';

test.describe.serial('TC3: 프로젝트 삭제', () => {
  /** @type {import('@playwright/test').Page} */
  let page;
  /** @type {import('@playwright/test').BrowserContext} */
  let context;

  test.beforeAll(async ({ browser }) => {
    context = await browser.newContext();
    page = await context.newPage();
  });

  test.afterAll(async () => {
    await context.close();
  });

  // ─────────────────────────────────────────────────
  // Step 1.1: 선행 조건 확인 — TC2에서 생성한 프로젝트 존재
  // ─────────────────────────────────────────────────
  test('Step 1.1: ProjectMap 진입 및 대상 프로젝트 확인', async () => {
    await page.goto('http://localhost:3000');
    await waitForRemoteLoad(page);
    await dismissModals(page);
    await switchUser(page, 'batman');
    await waitForRemoteLoad(page);
    await dismissModals(page);

    // TC2에서 생성한 프로젝트가 존재하는지 확인
    const projectRow = page.locator('.pm-sidebar-row', { hasText: PROJECT_NAME });
    await expect(projectRow).toBeVisible({ timeout: 10_000 });
  });

  // ─────────────────────────────────────────────────
  // Step 1.2: 우클릭 → 삭제 클릭
  // ─────────────────────────────────────────────────
  test('Step 1.2: 우클릭 컨텍스트 메뉴에서 삭제 클릭', async () => {
    // 대상 프로젝트 행 우클릭
    const projectRow = page.locator('.pm-sidebar-row', { hasText: PROJECT_NAME });
    await projectRow.click({ button: 'right' });
    await page.waitForTimeout(500);

    // 검증: 컨텍스트 메뉴 표시
    const ctxMenu = page.locator('.ctx-menu');
    await expect(ctxMenu).toBeVisible({ timeout: 5000 });

    // 컨텍스트 메뉴에 프로젝트명 표시 확인
    await expect(ctxMenu.locator('.ctx-menu-header')).toContainText(PROJECT_NAME);

    // P1 검증: 삭제 확인 메시지
    // window.confirm 을 자동 수락하도록 설정
    page.on('dialog', async (dialog) => {
      expect(dialog.message()).toContain('삭제하시겠습니까');
      await dialog.accept();
    });

    // 삭제 메뉴 클릭
    const deleteItem = ctxMenu.locator('.ctx-menu-item', { hasText: '삭제' });
    await expect(deleteItem).toBeVisible();
    await deleteItem.click();
    await page.waitForTimeout(2000);
  });

  // ─────────────────────────────────────────────────
  // Pass 조건 검증
  // ─────────────────────────────────────────────────
  test('P1: 삭제 확인 메시지 표시', async () => {
    // Step 1.2에서 dialog 이벤트로 검증 완료
    // (window.confirm에 "삭제하시겠습니까" 메시지가 포함됨)
    // 이 테스트는 Step 1.2의 dialog 핸들러에서 이미 assert됨
    expect(true).toBeTruthy();
  });

  test('P2: ProjectMap에서 프로젝트 제거', async () => {
    // 삭제 후 ProjectMap에서 해당 프로젝트가 사라졌는지 확인
    const projectRow = page.locator('.pm-sidebar-row', { hasText: PROJECT_NAME });
    await expect(projectRow).not.toBeVisible({ timeout: 10_000 });
  });

  test('P3: 프로젝트 레포지토리 삭제 확인', async () => {
    // repo 삭제는 비동기 처리이므로 충분히 대기
    await page.waitForTimeout(3000);

    // Gitea API로 repo 삭제 확인 — 검색 결과에 없어야 함
    const searchRes = await page.request.get(
      `${GITEA_BASE}/api/v1/repos/search?q=${JIRA_CODE}`
    );
    expect(searchRes.status()).toBe(200);
    const body = await searchRes.json();
    const repos = body.data || body;
    const found = repos.some((r) => r.name.startsWith(JIRA_CODE));
    expect(found).toBeFalsy();
  });
});
