// pex-meta.js
// Embed/extract a JSON metadata block inside a PlantUML source so layout
// edits travel together with the diagram in a single file.
//
// Format (after `@enduml`, every line is a `'` comment so the PlantUML
// parser ignores the block even if it ever stops short of EOF):
//
//   ' @startmeta
//   ' {
//   '   "schema": 1,
//   '   "layout": { "nodes": {...}, "edges": {...} },
//   '   ...
//   ' }
//   ' @endmeta
//
// All helpers operate on plain strings; no IO.

(function (root, factory) {
  if (typeof module === 'object' && module.exports) module.exports = factory();
  else root.PexMeta = factory();
})(typeof self !== 'undefined' ? self : this, function () {
  const META_START = "' @startmeta";
  const META_END = "' @endmeta";

  function escapeRe(s) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  // Remove any existing meta block (and surrounding blank lines) from `full`.
  function stripMeta(full) {
    if (!full) return full;
    const re = new RegExp(`\\n*${escapeRe(META_START)}[\\s\\S]*?${escapeRe(META_END)}\\s*$`, 'm');
    return full.replace(re, '').replace(/\s+$/, '') + '\n';
  }

  // Extract the meta JSON object from `full`. Returns null if absent or invalid.
  function extractMeta(full) {
    if (!full) return null;
    const i = full.indexOf(META_START);
    if (i === -1) return null;
    const j = full.indexOf(META_END, i + META_START.length);
    if (j === -1) return null;
    const block = full.slice(i + META_START.length, j);
    // Strip the leading `'` (and one optional space) from every line.
    const json = block.split('\n').map((l) => l.replace(/^\s*'\s?/, '')).join('\n').trim();
    if (!json) return null;
    try { return JSON.parse(json); } catch { return null; }
  }

  // Split `full` into the clean PlantUML source (no meta) and the parsed meta.
  function parseSource(full) {
    return { source: stripMeta(full || ''), meta: extractMeta(full || '') };
  }

  // Embed `meta` into the clean `source`, replacing any existing meta block.
  function embedMeta(source, meta) {
    const clean = stripMeta(source || '').replace(/\s+$/, '');
    if (!meta) return clean + '\n';
    const json = JSON.stringify(meta, null, 2);
    const commented = json.split('\n').map((l) => `' ${l}`.replace(/\s+$/, '')).join('\n');
    return `${clean}\n\n${META_START}\n${commented}\n${META_END}\n`;
  }

  // Detect entity renames between a saved layout and the current SVG by
  // matching orphaned meta keys (in layout, not in svg) to fresh entity
  // qnames (in svg, not in layout). Returns a `{ old: new }` map of
  // confidently-detected renames, mutates `layout` in place to reassign
  // node and edge keys to the new names.
  //
  // Strategy (in order, stops at the first match):
  //   1. 1 orphan + 1 fresh           → unambiguous rename
  //   2. 1 orphan + 1 substring fresh → substring-related rename
  //      (e.g. `Order` → `PurchaseOrder`, `User` → `UserV2`)
  //   3. any other shape              → skip (caller may prompt)
  //
  // svgQnames must be the set/array of entity qnames currently in the
  // rendered SVG. Layout shape: { nodes: { qname: {dx,dy} }, edges: { "A__B": {...} } }.
  function migrateRenamedKeys(svgQnames, layout) {
    if (!layout || !layout.nodes) return {};
    const present = new Set(svgQnames);
    const orphaned = Object.keys(layout.nodes).filter((k) => !present.has(k));
    const fresh = [...present].filter((q) => !(q in layout.nodes));
    if (orphaned.length !== 1) return {};

    const oldName = orphaned[0];
    let newName = null;
    if (fresh.length === 1) {
      newName = fresh[0];
    } else {
      // Substring fallback: find fresh entities whose qname overlaps the
      // orphan as a substring on either side. Most refactors append/prepend
      // (`Order` → `PurchaseOrder`, `OrderV2`).
      const oldLow = oldName.toLowerCase();
      const subs = fresh.filter((q) => {
        const low = q.toLowerCase();
        return low.includes(oldLow) || oldLow.includes(low);
      });
      if (subs.length === 1) newName = subs[0];
    }
    if (!newName) return {};
    const renames = { [oldName]: newName };

    // Migrate the node entry
    layout.nodes[newName] = layout.nodes[oldName];
    delete layout.nodes[oldName];

    // Migrate edge keys that referenced the renamed node on either side
    if (layout.edges) {
      for (const eKey of Object.keys(layout.edges)) {
        const idx = eKey.indexOf('__');
        if (idx === -1) continue;
        const a = eKey.slice(0, idx);
        const b = eKey.slice(idx + 2);
        const newA = renames[a] || a;
        const newB = renames[b] || b;
        if (newA === a && newB === b) continue;
        const newKey = `${newA}__${newB}`;
        if (newKey !== eKey) {
          layout.edges[newKey] = layout.edges[eKey];
          delete layout.edges[eKey];
        }
      }
    }
    return renames;
  }

  return { embedMeta, extractMeta, parseSource, stripMeta, migrateRenamedKeys, META_START, META_END };
});
