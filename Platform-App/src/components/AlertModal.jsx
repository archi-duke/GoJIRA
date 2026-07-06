import React, { useState, useCallback, useEffect } from 'react';
import './AlertModal.css';

let showAlertFn = null;
let showConfirmFn = null;
let showPromptFn = null;

// alert() 대체
export function modalAlert(message) {
  return new Promise(resolve => {
    if (showAlertFn) showAlertFn({ message, resolve });
  });
}

// confirm() 대체
export function modalConfirm(message) {
  return new Promise(resolve => {
    if (showConfirmFn) showConfirmFn({ message, resolve });
  });
}

// prompt() 대체
export function modalPrompt(message, defaultValue = '') {
  return new Promise(resolve => {
    if (showPromptFn) showPromptFn({ message, defaultValue, resolve });
  });
}

export default function AlertModal() {
  const [modal, setModal] = useState(null); // { type, message, defaultValue, resolve }

  const handleAlert = useCallback((data) => {
    setModal({ type: 'alert', ...data });
  }, []);

  const handleConfirm = useCallback((data) => {
    setModal({ type: 'confirm', ...data });
  }, []);

  const handlePrompt = useCallback((data) => {
    setModal({ type: 'prompt', ...data, inputValue: data.defaultValue || '' });
  }, []);

  useEffect(() => {
    showAlertFn = handleAlert;
    showConfirmFn = handleConfirm;
    showPromptFn = handlePrompt;
    return () => { showAlertFn = null; showConfirmFn = null; showPromptFn = null; };
  }, [handleAlert, handleConfirm, handlePrompt]);

  if (!modal) return null;

  function close(result) {
    modal.resolve(result);
    setModal(null);
  }

  return (
    <div className="am-overlay" onClick={() => {
      if (modal.type === 'alert') close(undefined);
      if (modal.type === 'confirm') close(false);
      if (modal.type === 'prompt') close(null);
    }}>
      <div className="am-dialog" onClick={e => e.stopPropagation()}>
        <div className="am-body">
          <pre className="am-message">{modal.message}</pre>
          {modal.type === 'prompt' && (
            <input className="am-input" autoFocus
              value={modal.inputValue}
              onChange={e => setModal(m => ({ ...m, inputValue: e.target.value }))}
              onKeyDown={e => { if (e.key === 'Enter') close(modal.inputValue); }}
            />
          )}
        </div>
        <div className="am-footer">
          {modal.type === 'confirm' && (
            <button className="am-cancel-btn" onClick={() => close(false)}>취소</button>
          )}
          {modal.type === 'prompt' && (
            <button className="am-cancel-btn" onClick={() => close(null)}>취소</button>
          )}
          <button className="am-ok-btn" autoFocus={modal.type !== 'prompt'}
            onClick={() => {
              if (modal.type === 'alert') close(undefined);
              if (modal.type === 'confirm') close(true);
              if (modal.type === 'prompt') close(modal.inputValue);
            }}>확인</button>
        </div>
      </div>
    </div>
  );
}
