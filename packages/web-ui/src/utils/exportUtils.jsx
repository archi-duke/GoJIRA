import html2canvas from 'html2canvas';

// --- HTML 파일 export (원본 스타일시트 포함) ---
function getStyledHTML(el) {
  const clone = el.cloneNode(true);
  const styles = [];
  for (const sheet of document.styleSheets) {
    try {
      for (const rule of sheet.cssRules) {
        styles.push(rule.cssText);
      }
    } catch { /* cross-origin sheets */ }
  }
  return `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>${styles.join('\n')}</style></head>
<body style="margin:0;padding:16px;background:#fff">${clone.outerHTML}</body></html>`;
}

// --- Confluence storage format 변환 (인라인 스타일, 정리) ---
function inlineStyles(source, clone) {
  const srcAll = source.querySelectorAll('*');
  const cloneAll = clone.querySelectorAll('*');
  const props = [
    'display', 'width', 'height', 'min-width', 'max-width',
    'margin', 'padding', 'border', 'border-radius', 'border-collapse', 'border-spacing',
    'background', 'background-color', 'color',
    'font-size', 'font-weight', 'font-family',
    'text-align', 'text-decoration', 'vertical-align', 'line-height',
    'white-space', 'overflow',
  ];
  for (let i = 0; i < srcAll.length && i < cloneAll.length; i++) {
    const computed = window.getComputedStyle(srcAll[i]);
    let style = '';
    for (const p of props) {
      const v = computed.getPropertyValue(p);
      if (v && v !== 'none' && v !== 'normal' && v !== 'auto') style += `${p}:${v};`;
    }
    cloneAll[i].setAttribute('style', style);
  }
}

function sanitize(clone) {
  const allEls = [clone, ...clone.querySelectorAll('*')];
  const removeAttrs = [];
  for (const el of allEls) {
    for (const attr of el.attributes) {
      const n = attr.name.toLowerCase();
      if (n.startsWith('on') || n.startsWith('data-') || n === 'tabindex' || n === 'draggable' || n === 'class') {
        removeAttrs.push({ el, name: attr.name });
      }
    }
  }
  for (const { el, name } of removeAttrs) el.removeAttribute(name);
  clone.querySelectorAll('script, noscript').forEach((s) => s.remove());
  clone.querySelectorAll('input, select, textarea').forEach((el) => {
    const span = document.createElement('span');
    span.textContent = el.value || '';
    el.replaceWith(span);
  });
  clone.querySelectorAll('button').forEach((btn) => btn.remove());
  clone.querySelectorAll('svg').forEach((svg) => svg.remove());
}

function toConfluenceHTML(el) {
  const clone = el.cloneNode(true);
  inlineStyles(el, clone);
  sanitize(clone);
  return clone.innerHTML;
}

// --- Export 메인 함수 ---
export async function exportContent(el, type, filename = 'export', confluenceOpts = null) {
  if (!el) return;

  if (type === 'html') {
    const html = getStyledHTML(el);
    const blob = new Blob([html], { type: 'text/html;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${filename}.html`;
    a.click();
    URL.revokeObjectURL(url);
  }

  if (type === 'jpg') {
    const canvas = await html2canvas(el, { backgroundColor: '#fff', scale: 2, useCORS: true });
    const url = canvas.toDataURL('image/jpeg', 0.95);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${filename}.jpg`;
    a.click();
  }

  if (type === 'clipboard') {
    const html = getStyledHTML(el);
    try {
      await navigator.clipboard.write([
        new ClipboardItem({ 'text/html': new Blob([html], { type: 'text/html' }) }),
      ]);
      alert('HTML이 클립보드에 복사되었습니다.');
    } catch {
      await navigator.clipboard.writeText(html);
      alert('HTML 텍스트가 클립보드에 복사되었습니다.');
    }
  }

  if (type === 'confluence') {
    const body = toConfluenceHTML(el);
    const html = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>${filename}</title></head>
<body>${body}</body></html>`;
    const blob = new Blob([html], { type: 'text/html;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${filename}_confluence.html`;
    a.click();
    URL.revokeObjectURL(url);
  }
}
