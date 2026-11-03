/*
 * SATAM Risk / Sensitivity / Mitigation(R/S/M) 파싱·직렬화·사전 빌드 (순수 로직).
 *
 * 두 가지 Mitigation 포맷을 모두 지원한다.
 *   - 레거시(웹 편집기 생성):  QA : [KEY]  +  Counter Risk : ["R1:KEY", ...]
 *   - SATAM v2(satam-analyze 스킬 생성, cross-ADSN):
 *       - ID : M1
 *         Target ADSN : ATALK-87      (key 또는 subkey)
 *         Target Risk : R2
 *         Description : ...
 *     → QA 가 없으므로 표시 위치(effQa)는 대상 Risk 의 QA 를 상속하고,
 *       해결 판정은 "대상 ADSN key + Risk ID" 로 한다(ADSN 간 R ID 충돌 방지).
 *
 * 인식하지 못한 필드(ReqType 등)는 extra 로 보존해 직렬화 시 그대로 되돌려 쓴다
 * — 웹에서 편집·저장해도 스킬이 기록한 필드가 유실되지 않도록.
 */

export function parseRST(yamlText) {
  if (!yamlText || typeof yamlText !== 'string') return [];
  const items = [];
  const lines = yamlText.split('\n');
  let current = null;

  for (const raw of lines) {
    const line = raw.trim();
    if (!line) continue;

    // 새 항목 시작: "- ID : R1" or "- ID: R1"
    const idMatch = line.match(/^-\s*ID\s*:\s*(.+)/i);
    if (idMatch) {
      if (current) items.push(current);
      current = {
        id: idMatch[1].trim(), qa: [], counterRisk: [], description: '',
        reqType: '', targetAdsn: '', targetRisk: '', extra: [],
      };
      continue;
    }

    if (!current) continue;

    // QA : [GOJIRA-001, GOJIRA-002] — bare 형식 공식, ["..", ".."] quoted 도 허용
    const qaMatch = line.match(/^QA\s*:\s*\[([^\]]*)\]/i);
    if (qaMatch) {
      current.qa = qaMatch[1]
        .split(',')
        .map((s) => s.trim().replace(/^["']|["']$/g, ''))
        .filter(Boolean);
      continue;
    }

    // Counter Risk : ["R1:GOJIRA-001", ...]
    const crMatch = line.match(/^Counter\s*Risk\s*:\s*\[([^\]]*)\]/i);
    if (crMatch) {
      current.counterRisk = crMatch[1]
        .split(',')
        .map((s) => s.trim().replace(/"/g, ''))
        .filter(Boolean);
      continue;
    }

    const rtMatch = line.match(/^ReqType\s*:\s*(.*)/i);
    if (rtMatch) {
      current.reqType = rtMatch[1].trim();
      continue;
    }

    const taMatch = line.match(/^Target\s*ADSN\s*:\s*(.*)/i);
    if (taMatch) {
      current.targetAdsn = taMatch[1].trim().replace(/^["']|["']$/g, '');
      continue;
    }

    const trMatch = line.match(/^Target\s*Risk\s*:\s*(.*)/i);
    if (trMatch) {
      current.targetRisk = trMatch[1].trim().replace(/^["']|["']$/g, '');
      continue;
    }

    // Description : ...
    const descMatch = line.match(/^Description\s*:\s*(.*)/i);
    if (descMatch) {
      current.description = descMatch[1].trim();
      continue;
    }

    current.extra.push(line);
  }
  if (current) items.push(current);
  return items;
}

/* risk/sensitivity/mitigation 데이터를 직렬화 (파생 필드 effQa/targetKey 는 쓰지 않는다) */
export function serializeRST(items) {
  return items.map((item) => {
    let s = `- ID : ${item.id}`;
    if (item.qa && item.qa.length > 0) {
      s += `\n  QA : [${item.qa.join(', ')}]`;
    }
    if (item.reqType) {
      s += `\n  ReqType : ${item.reqType}`;
    }
    if (item.targetAdsn) {
      s += `\n  Target ADSN : ${item.targetAdsn}`;
    }
    if (item.targetRisk) {
      s += `\n  Target Risk : ${item.targetRisk}`;
    }
    if (item.counterRisk && item.counterRisk.length > 0) {
      s += `\n  Counter Risk : [${item.counterRisk.map((cr) => `"${cr}"`).join(', ')}]`;
    }
    (item.extra || []).forEach((line) => {
      s += `\n  ${line}`;
    });
    if (item.description) {
      s += `\n  Description : ${item.description}`;
    }
    return s;
  }).join('\n');
}

/* Mitigation 이 셀에 표시될 requirement key 목록 (명시 QA ∪ 대상 Risk 의 QA) */
export function mitigationQa(m) {
  return m.effQa || m.qa;
}

export function buildDictionaries(designIssues) {
  // designKey -> { risks: [...], sensitivities: [...], mitigations: [...] }
  const byDesign = {};
  // qaKey -> { risks: [...], sensitivities: [...], mitigations: [...] } (requirement key 기준)
  const byQA = {};
  // 전역 risk/sensitivity/mitigation 딕셔너리: id -> item (with designKey)
  const riskDict = {};
  const sensDict = {};
  const mitiDict = {};
  // key/subkey -> key, key -> subkey (Target ADSN 은 key·subkey 어느 쪽으로든 기록될 수 있다)
  const keyOf = {};
  const subkeyOf = {};

  designIssues.forEach((d) => {
    keyOf[d.key] = d.key;
    if (d.subkey) {
      keyOf[d.subkey] = d.key;
      subkeyOf[d.key] = d.subkey;
    }
    byDesign[d.key] = {
      risks: parseRST(d.risk),
      sensitivities: parseRST(d.sensitivity),
      mitigations: parseRST(d.mitigation),
      concerns: parseRST(d.concerns),
    };
  });

  // v2 Mitigation: 대상 Risk 를 찾아 표시 위치(effQa)와 정규화된 대상 key 를 파생
  Object.values(byDesign).forEach((data) => {
    data.mitigations.forEach((m) => {
      m.effQa = [...m.qa];
      if (!m.targetRisk) return;
      m.targetKey = keyOf[m.targetAdsn] || m.targetAdsn;
      const target = byDesign[m.targetKey];
      const risk = target && target.risks.find((r) => r.id === m.targetRisk);
      if (risk) {
        risk.qa.forEach((q) => {
          if (!m.effQa.includes(q)) m.effQa.push(q);
        });
      }
    });
  });

  const ensureQA = (qaKey) => {
    if (!byQA[qaKey]) byQA[qaKey] = { risks: [], sensitivities: [], mitigations: [] };
    return byQA[qaKey];
  };

  designIssues.forEach((d) => {
    const { risks, sensitivities, mitigations } = byDesign[d.key];

    risks.forEach((r) => {
      riskDict[`${d.key}:${r.id}`] = { ...r, designKey: d.key };
      r.qa.forEach((qaKey) => ensureQA(qaKey).risks.push({ ...r, designKey: d.key }));
    });

    sensitivities.forEach((s) => {
      sensDict[`${d.key}:${s.id}`] = { ...s, designKey: d.key };
      s.qa.forEach((qaKey) => ensureQA(qaKey).sensitivities.push({ ...s, designKey: d.key }));
    });

    mitigations.forEach((m) => {
      mitiDict[`${d.key}:${m.id}`] = { ...m, designKey: d.key };
      mitigationQa(m).forEach((qaKey) => ensureQA(qaKey).mitigations.push({ ...m, designKey: d.key }));
    });
  });

  return { byDesign, byQA, riskDict, sensDict, mitiDict, subkeyOf };
}

/* 셀 교차점 계산 */
export function getCellInfo(designIssue, reqIssue, dicts) {
  const designData = dicts.byDesign[designIssue.key] || { risks: [], sensitivities: [], mitigations: [] };

  // 해당 requirement key가 등록된 R/S/M
  const risks = designData.risks.filter((r) => r.qa.includes(reqIssue.key));
  const sensitivities = designData.sensitivities.filter((s) => s.qa.includes(reqIssue.key));
  const mitigations = designData.mitigations.filter((m) => mitigationQa(m).includes(reqIssue.key));

  // issueLink contains 확인
  const hasContains = (reqIssue.issueLinks || []).some((link) => {
    const linkType = link.type?.name || '';
    const isContains = linkType.toLowerCase().includes('contains');
    const targetKey = link.inwardIssue?.key || link.outwardIssue?.key;
    return isContains && targetKey === designIssue.key;
  });

  return { risks, sensitivities, mitigations, hasContains };
}

/*
 * 해결된 Risk 집합.
 *   qualified — v2 Mitigation 의 "대상ADSNkey:RiskID" (정확 판정)
 *   bare      — 레거시 Counter Risk 의 RiskID (ADSN 정보가 없어 ID 만으로 판정 — 기존 동작 유지)
 */
export function buildResolvedRisks(byDesign) {
  const qualified = new Set();
  const bare = new Set();
  Object.values(byDesign).forEach((data) => {
    data.mitigations.forEach((m) => {
      if (m.targetRisk && m.targetKey) qualified.add(`${m.targetKey}:${m.targetRisk}`);
      m.counterRisk.forEach((cr) => {
        const [rId] = cr.split(':');
        if (rId) bare.add(rId.trim());
      });
    });
  });
  return { qualified, bare };
}

export function isRiskResolved(resolved, designKey, riskId) {
  return resolved.qualified.has(`${designKey}:${riskId}`) || resolved.bare.has(riskId);
}

/* Mitigation 의 대상 표기 (Effects 테이블·팝업용) */
export function mitigationTargetText(m, subkeyOf) {
  const parts = [];
  if (m.targetRisk) {
    const adsn = (subkeyOf && subkeyOf[m.targetKey]) || m.targetAdsn;
    parts.push(`Target: ${adsn ? `${adsn} ` : ''}${m.targetRisk}`);
  }
  if (m.counterRisk && m.counterRisk.length > 0) {
    parts.push(`Counter: ${m.counterRisk.join(', ')}`);
  }
  return parts.length > 0 ? ` [${parts.join(' / ')}]` : '';
}
