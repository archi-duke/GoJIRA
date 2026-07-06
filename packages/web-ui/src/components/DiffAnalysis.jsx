import React, { useMemo, useState } from 'react';
import * as Diff from 'diff';

/* ─── Markdown Section Parser ─── */
function parseMarkdownSections(text) {
  const lines = (text || '').split('\n');
  const sections = [];
  let cur = { title: '(intro)', level: 0, blocks: [] };
  let buf = [];

  const flushBuf = () => {
    const t = buf.join('\n').trim();
    if (t) cur.blocks.push(t);
    buf = [];
  };

  for (const line of lines) {
    const m = line.match(/^(#{1,6})\s+(.+)/);
    if (m) {
      flushBuf();
      if (cur.blocks.length > 0 || cur.title !== '(intro)') sections.push(cur);
      cur = { title: m[2].trim(), level: m[1].length, blocks: [] };
    } else if (line.trim() === '' && buf.some(l => l.trim())) {
      flushBuf();
    } else {
      buf.push(line);
    }
  }
  flushBuf();
  sections.push(cur);
  return sections.filter(s => s.blocks.length > 0 || s.title !== '(intro)');
}

/* ─── 유사도 계산 ─── */
function similarity(a, b) {
  const wa = new Set(a.toLowerCase().split(/\s+/).filter(Boolean));
  const wb = new Set(b.toLowerCase().split(/\s+/).filter(Boolean));
  if (wa.size === 0 && wb.size === 0) return 1;
  const inter = [...wa].filter(w => wb.has(w)).length;
  return inter / Math.max(wa.size, wb.size);
}

/* ─── Semantic Diff 계산 ─── */
function computeSemanticDiff(oldText, newText) {
  const oldSections = parseMarkdownSections(oldText);
  const newSections = parseMarkdownSections(newText);

  const normalize = t => t.trim().toLowerCase().replace(/\s+/g, ' ');
  const oldMap = {};
  oldSections.forEach(s => { oldMap[normalize(s.title)] = s; });
  const matched = new Set();

  const result = { summary: { sectionsAdded: 0, sectionsModified: 0, sectionsDeleted: 0, blocksChanged: 0 }, sections: [] };

  // 새 섹션 순회
  for (const ns of newSections) {
    const key = normalize(ns.title);
    const os = oldMap[key];
    if (!os) {
      result.summary.sectionsAdded++;
      result.summary.blocksChanged += ns.blocks.length;
      result.sections.push({
        title: ns.title, changeType: 'added',
        blocks: ns.blocks.map(b => ({ changeType: 'added', before: '', after: b, inlineDiff: [{ type: 'added', text: b }] })),
      });
    } else {
      matched.add(key);
      const blocks = diffBlocks(os.blocks, ns.blocks);
      const hasChanges = blocks.some(b => b.changeType !== 'unchanged');
      if (hasChanges) {
        result.summary.sectionsModified++;
        result.summary.blocksChanged += blocks.filter(b => b.changeType !== 'unchanged').length;
      }
      result.sections.push({ title: ns.title, changeType: hasChanges ? 'modified' : 'unchanged', blocks });
    }
  }

  // 삭제된 섹션
  for (const os of oldSections) {
    if (!matched.has(normalize(os.title))) {
      result.summary.sectionsDeleted++;
      result.summary.blocksChanged += os.blocks.length;
      result.sections.push({
        title: os.title, changeType: 'deleted',
        blocks: os.blocks.map(b => ({ changeType: 'deleted', before: b, after: '', inlineDiff: [{ type: 'removed', text: b }] })),
      });
    }
  }

  return result;
}

/* ─── 블록(문단) 비교 ─── */
function diffBlocks(oldBlocks, newBlocks) {
  const results = [];
  const used = new Set();

  for (const nb of newBlocks) {
    let bestIdx = -1, bestSim = 0.4;
    oldBlocks.forEach((ob, i) => {
      if (used.has(i)) return;
      const s = similarity(ob, nb);
      if (s > bestSim) { bestSim = s; bestIdx = i; }
    });

    if (bestIdx >= 0) {
      used.add(bestIdx);
      const ob = oldBlocks[bestIdx];
      if (ob === nb) {
        results.push({ changeType: 'unchanged', before: ob, after: nb, inlineDiff: [] });
      } else {
        const wd = Diff.diffWords(ob, nb);
        const inlineDiff = wd.map(p => ({
          type: p.added ? 'added' : p.removed ? 'removed' : 'equal',
          text: p.value,
        }));
        results.push({ changeType: 'modified', before: ob, after: nb, inlineDiff });
      }
    } else {
      results.push({ changeType: 'added', before: '', after: nb, inlineDiff: [{ type: 'added', text: nb }] });
    }
  }

  oldBlocks.forEach((ob, i) => {
    if (!used.has(i)) {
      results.push({ changeType: 'deleted', before: ob, after: '', inlineDiff: [{ type: 'removed', text: ob }] });
    }
  });

  return results;
}

/* ─── Summary 생성 ─── */
function generateSummary(sem) {
  const lines = [];
  const changed = sem.sections.filter(s => s.changeType !== 'unchanged');
  if (changed.length === 0) { lines.push('변경사항이 없습니다.'); return lines; }

  lines.push(`총 ${changed.length}개 섹션에 변경사항이 있습니다.`);

  for (const s of changed) {
    if (s.changeType === 'added') {
      lines.push({ icon: '+', color: '#27ae60', text: `"${s.title}" 섹션이 새로 추가되었습니다.` });
    } else if (s.changeType === 'deleted') {
      lines.push({ icon: '−', color: '#e74c3c', text: `"${s.title}" 섹션이 삭제되었습니다.` });
    } else {
      const added = s.blocks.filter(b => b.changeType === 'added').length;
      const deleted = s.blocks.filter(b => b.changeType === 'deleted').length;
      const modified = s.blocks.filter(b => b.changeType === 'modified').length;
      const parts = [];
      if (modified) parts.push(`${modified}개 문단 수정`);
      if (added) parts.push(`${added}개 문단 추가`);
      if (deleted) parts.push(`${deleted}개 문단 삭제`);
      lines.push({ icon: '~', color: '#e67e22', text: `"${s.title}" 섹션: ${parts.join(', ')}` });

      // 핵심 변경 내용 표시
      s.blocks.filter(b => b.changeType === 'modified').forEach(block => {
        const removed = block.inlineDiff.filter(d => d.type === 'removed').map(d => d.text.trim()).filter(Boolean);
        const added = block.inlineDiff.filter(d => d.type === 'added').map(d => d.text.trim()).filter(Boolean);
        if (removed.length && added.length) {
          lines.push({ icon: '→', color: '#888', text: `"${removed.join(' ').slice(0, 40)}" → "${added.join(' ').slice(0, 40)}"`, indent: true });
        }
      });
    }
  }
  return lines;
}

/* ═══════════════════════════════════════════
   React Components
   ═══════════════════════════════════════════ */

/* ─── Unified Diff View ─── */
function UnifiedDiffView({ oldText, newText, fileName }) {
  const hunks = useMemo(() => {
    const patch = Diff.structuredPatch(fileName, fileName, oldText || '', newText || '', '', '', { context: 3 });
    return patch.hunks;
  }, [oldText, newText, fileName]);

  if (hunks.length === 0) return <div className="da-empty">차이가 없습니다.</div>;

  return (
    <div className="da-unified">
      {hunks.map((hunk, hi) => (
        <div key={hi} className="da-hunk">
          <div className="da-hunk-header">@@ -{hunk.oldStart},{hunk.oldLines} +{hunk.newStart},{hunk.newLines} @@</div>
          {hunk.lines.map((line, li) => {
            const cls = line.startsWith('+') ? 'da-line-add' : line.startsWith('-') ? 'da-line-del' : 'da-line-ctx';
            return <div key={li} className={`da-line ${cls}`}>{line}</div>;
          })}
        </div>
      ))}
    </div>
  );
}

/* ─── Semantic Diff View ─── */
function SemanticDiffView({ oldText, newText }) {
  const sem = useMemo(() => computeSemanticDiff(oldText, newText), [oldText, newText]);
  const changed = sem.sections.filter(s => s.changeType !== 'unchanged');

  if (changed.length === 0) return <div className="da-empty">차이가 없습니다.</div>;

  return (
    <div className="da-semantic">
      <div className="da-stats">
        {sem.summary.sectionsAdded > 0 && <span className="da-stat da-stat-add">+{sem.summary.sectionsAdded} 섹션 추가</span>}
        {sem.summary.sectionsModified > 0 && <span className="da-stat da-stat-mod">~{sem.summary.sectionsModified} 섹션 수정</span>}
        {sem.summary.sectionsDeleted > 0 && <span className="da-stat da-stat-del">−{sem.summary.sectionsDeleted} 섹션 삭제</span>}
        <span className="da-stat">{sem.summary.blocksChanged}개 문단 변경</span>
      </div>

      <div className="da-nav">
        {changed.map((s, i) => (
          <a key={i} href={`#da-section-${i}`} className={`da-nav-item da-nav-${s.changeType}`}>{s.title}</a>
        ))}
      </div>

      {changed.map((section, si) => (
        <div key={si} id={`da-section-${si}`} className={`da-section da-section-${section.changeType}`}>
          <div className="da-section-title">
            <span className={`da-badge da-badge-${section.changeType}`}>
              {section.changeType === 'added' ? '추가' : section.changeType === 'deleted' ? '삭제' : '수정'}
            </span>
            {section.title}
          </div>
          {section.blocks.filter(b => b.changeType !== 'unchanged').map((block, bi) => (
            <div key={bi} className={`da-block da-block-${block.changeType}`}>
              {block.changeType === 'modified' ? (
                <div className="da-inline-diff">
                  {block.inlineDiff.map((d, di) => (
                    <span key={di} className={d.type === 'added' ? 'da-word-add' : d.type === 'removed' ? 'da-word-del' : ''}>
                      {d.text}
                    </span>
                  ))}
                </div>
              ) : block.changeType === 'added' ? (
                <div className="da-block-text da-text-add">{block.after}</div>
              ) : (
                <div className="da-block-text da-text-del">{block.before}</div>
              )}
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

/* ─── Summary View ─── */
function SummaryView({ oldText, newText }) {
  const sem = useMemo(() => computeSemanticDiff(oldText, newText), [oldText, newText]);
  const lines = useMemo(() => generateSummary(sem), [sem]);

  return (
    <div className="da-summary">
      {lines.map((item, i) => {
        if (typeof item === 'string') return <div key={i} className="da-summary-head">{item}</div>;
        return (
          <div key={i} className={`da-summary-item ${item.indent ? 'da-summary-indent' : ''}`}>
            <span className="da-summary-icon" style={{ color: item.color }}>{item.icon}</span>
            <span>{item.text}</span>
          </div>
        );
      })}
    </div>
  );
}

/* ─── Main DiffAnalysis Component ─── */
export default function DiffAnalysis({ oldText, newText, fileName, activeTab, onTabChange }) {
  const tabs = [
    { key: 'semantic', label: 'Semantic Diff' },
    { key: 'diff', label: 'Diff' },
    { key: 'summary', label: 'Summary' },
  ];

  return (
    <div className="da-container">
      <div className="da-tabs">
        {tabs.map(t => (
          <button key={t.key}
            className={`da-tab ${activeTab === t.key ? 'da-tab-active' : ''}`}
            onClick={() => onTabChange(t.key)}>
            {t.label}
          </button>
        ))}
      </div>
      <div className="da-content">
        {activeTab === 'diff' && <UnifiedDiffView oldText={oldText} newText={newText} fileName={fileName} />}
        {activeTab === 'semantic' && <SemanticDiffView oldText={oldText} newText={newText} />}
        {activeTab === 'summary' && <SummaryView oldText={oldText} newText={newText} />}
      </div>
    </div>
  );
}
