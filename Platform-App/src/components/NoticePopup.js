import React, { useEffect, useState, useCallback } from 'react';
import config from '../config';
import './NoticePopup.css';

const API = config.API_BASE + '/Notice';
export default function NoticePopup({ user }) {
  const HIDE_KEY = `gojira_notice_hide_until_${user || 'anon'}`;
  const [notices, setNotices] = useState([]);
  const [currentIdx, setCurrentIdx] = useState(0);
  const [visible, setVisible] = useState(false);
  const [hideFor3Days, setHideFor3Days] = useState(false);

  const showNotices = useCallback((data) => {
    if (!Array.isArray(data) || data.length === 0) return;
    setNotices(data);
    setCurrentIdx(0);
    setVisible(true);
    data.forEach(n => {
      fetch(`${API}/${n._id}/view`, { method: 'POST' }).catch(() => {});
    });
  }, []);

  // 접속 시 게시중 공지 로딩
  useEffect(() => {
    const hideUntil = localStorage.getItem(HIDE_KEY);
    if (hideUntil && new Date(hideUntil) > new Date()) return;

    fetch(`${API}/active`)
      .then(r => r.json())
      .then(showNotices)
      .catch(() => {});
  }, [showNotices]);

  // SSE: 비상 공지 실시간 수신
  useEffect(() => {
    const es = new EventSource(`${API}/stream`);
    es.addEventListener('emergency', (e) => {
      try {
        const notice = JSON.parse(e.data);
        // 비상 공지는 3일 보지 않기와 무관하게 즉시 표시
        showNotices([notice]);
      } catch {}
    });
    return () => es.close();
  }, [showNotices]);

  function handleClose() {
    if (hideFor3Days) {
      const until = new Date();
      until.setDate(until.getDate() + 3);
      localStorage.setItem(HIDE_KEY, until.toISOString());
    }
    setVisible(false);
  }

  if (!visible || notices.length === 0) return null;

  const notice = notices[currentIdx];
  const isMarkdown = notice.bodyType === 'markdown';

  return (
    <div className="np-overlay">
      <div className="np-popup">
        <div className="np-header">
          <span className="np-title">
            {notice.isEmergency && <span className="np-emergency">긴급</span>}
            {notice.title}
          </span>
          <span className="np-pager">
            {notices.length > 1 && `${currentIdx + 1} / ${notices.length}`}
          </span>
        </div>
        <div className="np-body">
          {isMarkdown
            ? <pre className="np-markdown">{notice.body}</pre>
            : <div className="np-html" dangerouslySetInnerHTML={{ __html: notice.body }} />
          }
        </div>
        <div className="np-footer">
          <label className="np-hide-check">
            <input type="checkbox" checked={hideFor3Days} onChange={e => setHideFor3Days(e.target.checked)} />
            3일동안 보지 않기
          </label>
          <div className="np-btns">
            {currentIdx > 0 && <button className="np-btn" onClick={() => setCurrentIdx(i => i - 1)}>이전</button>}
            {currentIdx < notices.length - 1
              ? <button className="np-btn np-btn-primary" onClick={() => setCurrentIdx(i => i + 1)}>다음</button>
              : <button className="np-btn np-btn-primary" onClick={handleClose}>닫기</button>
            }
          </div>
        </div>
      </div>
    </div>
  );
}
