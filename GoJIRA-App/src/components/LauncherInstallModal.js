// 런처 미감지시 표시하는 설치 안내 모달.
// Platform-API 가 /api/v2/launcher/manifest 로 OS별 호스팅 상태를 알려주고,
// /api/v2/launcher/download/{os} 가 실제 바이너리를 stream 한다.
import React, { useEffect, useState } from 'react';
import config from '../config';
import './LauncherInstallModal.css';

const PLATFORM_API = config.PLATFORM_API;

function detectOS() {
  const ua = (navigator.userAgent || '').toLowerCase();
  if (ua.includes('windows')) return 'windows';
  if (ua.includes('mac')) return 'darwin';
  return 'linux';
}

function installInstructions(osKey) {
  if (osKey === 'windows') {
    return [
      '내려받은 gojira-launcher.exe 를 PATH 가 닿는 폴더 (예: %USERPROFILE%\\.local\\bin) 에 둡니다.',
      '~/.gojira/launcher.json 설정 파일을 작성합니다 (관리자에게 양식 요청).',
      '바이너리를 실행하면 127.0.0.1:5599 에서 동작합니다.',
    ];
  }
  if (osKey === 'darwin') {
    return [
      'chmod +x gojira-launcher && mv gojira-launcher ~/.local/bin/',
      '~/.gojira/launcher.json 작성 (관리자에게 양식 요청)',
      './gojira-launcher & 또는 launchd 에 등록 → 127.0.0.1:5599',
      'Gatekeeper: xattr -dr com.apple.quarantine ~/.local/bin/gojira-launcher (서명 전)',
    ];
  }
  return [
    'chmod +x gojira-launcher && mv gojira-launcher ~/.local/bin/',
    '~/.gojira/launcher.json 작성 (관리자에게 양식 요청)',
    'nohup ~/.local/bin/gojira-launcher > ~/.gojira/logs/launcher.log 2>&1 & → 127.0.0.1:5599',
  ];
}

export default function LauncherInstallModal({ onClose, onRetry }) {
  const [manifest, setManifest] = useState(null);
  const [osKey, setOSKey] = useState(detectOS());
  const [loadErr, setLoadErr] = useState('');

  useEffect(() => {
    fetch(`${PLATFORM_API}/launcher/manifest`)
      .then(r => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
      .then(d => setManifest(d.platforms || []))
      .catch(e => setLoadErr(e.message));
  }, []);

  const entry = (manifest || []).find(p => p.os === osKey);
  const downloadHref = `${PLATFORM_API}/launcher/download/${osKey}`;

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
            {installInstructions(osKey).map((s, i) => <li key={i}>{s}</li>)}
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
