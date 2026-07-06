// 런처 미감지시 표시하는 설치 안내 모달.
// Platform-API 가 /api/v2/launcher/manifest 로 OS별 호스팅 상태를 알려주고,
// /api/v2/launcher/download/{os} 가 실제 바이너리를 stream 한다.
import React, { useEffect, useState } from 'react';
import config from '../config';
import './LauncherInstallModal.css';
import { authFetch } from '../platform/auth';

const PLATFORM_API = config.PLATFORM_API;

function detectOS() {
  const ua = (navigator.userAgent || '').toLowerCase();
  if (ua.includes('windows')) return 'windows';
  if (ua.includes('mac')) return 'darwin';
  return 'linux';
}

function installInstructions(osKey, gwOrigin) {
  // 현재 웹 origin(=게이트웨이 주소)을 install --gateway 로 넣어주면 launcher.json 의
  // backend URL/webOrigins 가 자동 파생된다 — 손편집 불필요. origin 미확인 시 플래그 생략.
  const gw = gwOrigin ? ` --gateway ${gwOrigin}` : '';
  if (osKey === 'windows') {
    return [
      '내려받은 gojira-launcher.exe 를 적당한 폴더에 둡니다.',
      `명령 프롬프트에서 gojira-launcher.exe install${gw} 을 1회 실행합니다 — 스킬 배포 + 설정파일 생성(gateway 자동 기록) + 로그인 시 자동시작 등록 + 데몬 기동(127.0.0.1:5599)까지 자동.`,
      '~/.gojira/launcher.json 의 git.user / git.token (폐쇄망 인증)을 채우고 런처를 재시작합니다 (양식은 관리자에게 요청).',
      '이후 런처는 기동할 때마다 최신 버전을 확인하고 자동 업데이트합니다.',
    ];
  }
  if (osKey === 'darwin') {
    return [
      'chmod +x gojira-launcher',
      'Gatekeeper: xattr -dr com.apple.quarantine gojira-launcher (서명 전)',
      `./gojira-launcher install${gw} — 스킬 배포 + 설정파일 생성(gateway 자동 기록) + 로그인 항목 등록 + 데몬 기동(127.0.0.1:5599)까지 자동.`,
      '~/.gojira/launcher.json 의 git.user / git.token 을 채우고 재시작 (양식은 관리자에게 요청). 이후 자동 업데이트.',
    ];
  }
  return [
    'chmod +x gojira-launcher',
    `./gojira-launcher install${gw} — 스킬 배포 + 설정파일 생성(gateway 자동 기록) + 자동시작(.desktop) 등록 + 데몬 기동(127.0.0.1:5599)까지 자동.`,
    '~/.gojira/launcher.json 의 git.user / git.token 을 채우고 재시작 (양식은 관리자에게 요청). 이후 자동 업데이트.',
  ];
}

export default function LauncherInstallModal({ onClose, onRetry }) {
  const [manifest, setManifest] = useState(null);
  const [osKey, setOSKey] = useState(detectOS());
  const [loadErr, setLoadErr] = useState('');

  useEffect(() => {
    authFetch(`${PLATFORM_API}/launcher/manifest`)
      .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
      .then(d => setManifest(d.platforms || []))
      .catch(e => setLoadErr(e.message));
  }, []);

  const entry = (manifest || []).find(p => p.os === osKey);
  const downloadHref = `${PLATFORM_API}/launcher/download/${osKey}`;
  // 현재 접속 origin = 게이트웨이 주소. install --gateway 로 그대로 넣어준다.
  const gwOrigin = (typeof window !== 'undefined' && window.location && window.location.origin) || '';

  return (
    <div className="lim-backdrop" onClick={onClose}>
      <div className="lim-modal" onClick={e => e.stopPropagation()}>
        <div className="lim-header">
          <h3>로컬 런처 설치</h3>
          <button className="lim-close" onClick={onClose}>×</button>
        </div>
        <div className="lim-body">
          <p className="lim-intro">
            "로컬 수정" 은 사용자 PC 의 <code>gojira-launcher</code> 데몬이 필요합니다.
            VS Code 와 (선택) 에이전트 CLI 를 띄우는 경량 127.0.0.1 서버입니다.
          </p>
          <div className="lim-os-row">
            {['linux', 'darwin', 'windows'].map(k => (
              <button
                key={k}
                className={`lim-os-tab ${osKey === k ? 'active' : ''}`}
                onClick={() => setOSKey(k)}
              >{k === 'darwin' ? 'macOS' : k.charAt(0).toUpperCase() + k.slice(1)}</button>
            ))}
          </div>
          <div className="lim-download">
            {loadErr && <div className="lim-warn">manifest 조회 실패: {loadErr}</div>}
            {!loadErr && entry && !entry.ready && (
              <div className="lim-warn">이 OS 의 바이너리는 아직 호스팅되어 있지 않습니다. 관리자에게 문의하세요.</div>
            )}
            {!loadErr && entry && entry.ready && (
              <a className="lim-dl-btn" href={downloadHref} download>
                ⬇ {osKey} 런처 다운로드 ({(entry.size / 1024 / 1024).toFixed(1)} MB)
              </a>
            )}
          </div>
          <ol className="lim-steps">
            {installInstructions(osKey, gwOrigin).map((s, i) => <li key={i}>{s}</li>)}
          </ol>
        </div>
        <div className="lim-footer">
          <button className="lim-btn" onClick={onClose}>닫기</button>
          <button className="lim-btn lim-btn-primary" onClick={onRetry}>설치 완료 — 다시 시도</button>
        </div>
      </div>
    </div>
  );
}
