// @ts-check
/**
 * TC2: 프로젝트 생성
 *
 * 사전 조건:
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

test.describe.serial('TC2: 프로젝트 생성', () => {
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
  // Step 1.1: ProjectMap 진입 및 + Add 버튼 클릭
  // ─────────────────────────────────────────────────
  test('Step 1.1: ProjectMap 진입 후 + Add 클릭', async () => {
    await page.goto('http://localhost:3000');
    await waitForRemoteLoad(page);
    await dismissModals(page);
    await switchUser(page, 'batman');
    await waitForRemoteLoad(page);
    await dismissModals(page);

    // ProjectMap 표시 확인
    const addBtn = page.locator('text=+ Add');
    await expect(addBtn).toBeVisible({ timeout: 10_000 });

    // + Add 버튼 클릭
    await addBtn.click();
    await page.waitForTimeout(500);

    // 검증: 프로젝트 생성 다이얼로그 표시
    await expect(page.locator('.dlg-overlay')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.dlg-title')).toBeVisible();
  });

  // ─────────────────────────────────────────────────
  // Step 1.2: 프로젝트 정보 기입
  // ─────────────────────────────────────────────────
  test('Step 1.2: 프로젝트 정보 기입', async () => {
    // 과제명 입력
    const nameInput = page.locator('.dlg-field').filter({ hasText: '과제명' }).locator('.dlg-input');
    await expect(nameInput).toBeVisible({ timeout: 5000 });
    await nameInput.fill(PROJECT_NAME);

    // JIRA Code 입력
    const codeInput = page.locator('.dlg-field').filter({ hasText: 'JIRA Code' }).locator('.dlg-input');
    await expect(codeInput).toBeVisible({ timeout: 5000 });
    await codeInput.fill(JIRA_CODE);

    // 검증: 입력값 반영
    await expect(nameInput).toHaveValue(PROJECT_NAME);
    await expect(codeInput).toHaveValue(JIRA_CODE);
  });

  // ─────────────────────────────────────────────────
  // Step 1.3: 저장하여 프로젝트 생성
  // ─────────────────────────────────────────────────
  test('Step 1.3: 저장하여 프로젝트 생성', async () => {
    // 저장 버튼 클릭
    const saveBtn = page.locator('.dlg-btn-primary', { hasText: '저장' });
    await expect(saveBtn).toBeVisible({ timeout: 5000 });
    await saveBtn.click();
    await page.waitForTimeout(2000);

    // 다이얼로그 닫힘 확인
    await expect(page.locator('.dlg-overlay')).not.toBeVisible({ timeout: 5000 });
  });

  // ─────────────────────────────────────────────────
  // Pass 조건 검증
  // ─────────────────────────────────────────────────
  test('P1: ProjectMap에 과제 표시', async () => {
    // ProjectMap에서 생성한 프로젝트가 보이는지 확인
    const projectRow = page.locator('.pm-sidebar-row', { hasText: PROJECT_NAME });
    await expect(projectRow).toBeVisible({ timeout: 10_000 });
  });

  test('P2: Gitea에 과제 레포지토리 생성', async () => {
    // Gitea API로 repo 존재 확인
    const response = await page.request.get(
      `${GITEA_BASE}/api/v1/repos/${GITEA_ORG}/${JIRA_CODE}`,
      { failOnStatusCode: false }
    );

    // repo 이름이 JIRA_CODE로 시작하는 것을 찾기 (타임스탬프 접미사 포함)
    if (response.status() === 404) {
      // 타임스탬프 접미사가 붙은 경우 검색으로 찾기
      const searchRes = await page.request.get(
        `${GITEA_BASE}/api/v1/repos/search?q=${JIRA_CODE}`
      );
      expect(searchRes.status()).toBe(200);
      const body = await searchRes.json();
      const repos = body.data || body;
      const found = repos.some((r) => r.name.startsWith(JIRA_CODE));
      expect(found).toBeTruthy();
    } else {
      expect(response.status()).toBe(200);
    }
  });

  test('P3: Repository 메뉴에서 정보 확인', async () => {
    // 생성한 프로젝트 클릭
    const projectRow = page.locator('.pm-sidebar-row', { hasText: PROJECT_NAME });
    await projectRow.click();
    await page.waitForTimeout(1000);

    // Docs 탭 클릭
    const docsTab = page.locator('.cb-nav-btn', { hasText: 'Docs' });
    await expect(docsTab).toBeVisible({ timeout: 5000 });
    await docsTab.click();
    await page.waitForTimeout(1500);

    // 파일 목록에 초기 파일이 존재하는지 확인
    // business/ 또는 architecture/ 폴더가 보여야 함
    const fileTree = page.locator('.de-sidebar');
    await expect(fileTree).toBeVisible({ timeout: 10_000 });

    // business 또는 architecture 폴더/파일 존재 확인
    const hasBusinessOrArch = page.locator('text=/business|architecture/');
    await expect(hasBusinessOrArch.first()).toBeVisible({ timeout: 10_000 });
  });
});
