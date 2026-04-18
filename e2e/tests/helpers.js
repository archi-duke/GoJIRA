// @ts-check

/**
 * E2E 테스트 공통 헬퍼
 */

/** 알림 모달(취소/확인)이 떠 있으면 닫는다 */
async function dismissModals(page, action = 'cancel') {
  const btnText = action === 'confirm' ? '확인' : '취소';
  for (let i = 0; i < 5; i++) {
    const btn = page.locator('button', { hasText: btnText });
    try {
      if (await btn.isVisible({ timeout: 1000 })) {
        await btn.click();
        await page.waitForTimeout(300);
        continue;
      }
    } catch { /* 모달 없음 */ }
    break;
  }
}

/** 모달 확인 버튼 클릭 (발행신청, 저장 등) */
async function confirmModal(page) {
  const btn = page.locator('button', { hasText: '확인' });
  await expect(btn).toBeVisible({ timeout: 5000 });
  await btn.click();
  await page.waitForTimeout(300);
}

/** 사용자 전환 — app-user-switch 셀렉터에서 사용자 선택 */
async function switchUser(page, knoxId) {
  const selector = page.locator('.app-user-switch');
  await expect(selector).toBeVisible({ timeout: 10_000 });
  await selector.selectOption(knoxId);
  await page.waitForTimeout(500);
}

/** ProjectMap에서 특정 프로젝트 행 클릭 */
async function clickProject(page, projectName) {
  // ProjectMap 좌측 사이드바의 프로젝트 행
  const row = page.locator('.pm-sidebar-row', { hasText: projectName });
  await expect(row).toBeVisible({ timeout: 10_000 });
  await row.click();
  await page.waitForTimeout(500);
}

/** CommonBar 네비게이션에서 특정 탭 클릭 (Docs, Issues 등) */
async function clickNavTab(page, label) {
  const btn = page.locator('.cb-nav-btn', { hasText: label });
  await expect(btn).toBeVisible({ timeout: 5000 });
  await btn.click();
  await page.waitForTimeout(500);
}

/** 상단 앱 네비게이션에서 탭 클릭 (Project, Account 등) */
async function clickAppNav(page, label) {
  const btn = page.locator('.app-nav-btn', { hasText: label });
  await expect(btn).toBeVisible({ timeout: 5000 });
  await btn.click();
  await page.waitForTimeout(500);
}

/** Module Federation Remote 로딩 대기 */
async function waitForRemoteLoad(page) {
  await expect(page.locator('text=로딩 중...')).not.toBeVisible({ timeout: 20_000 });
  await expect(page.locator('text=GoJIRA-App에 연결할 수 없습니다')).not.toBeVisible({ timeout: 5000 });
}

const { expect } = require('@playwright/test');

const GITEA_BASE = 'http://localhost:3300';
const GITEA_ORG = 'gojira';
const GITEA_TOKEN = process.env.GITEA_TOKEN || '58b690cf0c26186a06a752b5b18ba67f1d11b229';

function giteaHeaders() {
  return { Authorization: `token ${GITEA_TOKEN}` };
}

async function giteaRepoExists(request, jiraCode) {
  const res = await request.get(
    `${GITEA_BASE}/api/v1/repos/search?q=${jiraCode}`,
    { headers: giteaHeaders() }
  );
  if (res.status() !== 200) return false;
  const body = await res.json();
  const repos = body.data || body;
  return repos.some((r) => r.name.startsWith(jiraCode));
}

module.exports = {
  dismissModals,
  confirmModal,
  switchUser,
  clickProject,
  clickNavTab,
  clickAppNav,
  waitForRemoteLoad,
  GITEA_BASE,
  GITEA_ORG,
  GITEA_TOKEN,
  giteaHeaders,
  giteaRepoExists,
};
