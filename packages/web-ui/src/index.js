// @platform/web-ui — 플랫폼 공통 UI/런타임 (계약 docs/platform-contract/04)
// 소비: vite.config resolve.alias '@platform/web-ui' → 이 패키지 src/.
// 서브패스 import 권장: '@platform/web-ui/components/AlertModal' 등.
export { default as AlertModal, modalAlert, modalConfirm, modalPrompt } from './components/AlertModal';
export { default as DataGrid } from './components/DataGrid';
export { default as CommonBar } from './components/CommonBar';
export { default as NoticePopup } from './components/NoticePopup';
export { default as DiffAnalysis } from './components/DiffAnalysis';
export * as exportUtils from './utils/exportUtils';
export { getAuthUser } from './utils/authUser';
export { subscribe, onStatus } from './runtime/sseManager';
export { createEventQueue } from './runtime/eventQueue';
export * as launcher from './runtime/launcher';
export * as env from './runtime/env';
