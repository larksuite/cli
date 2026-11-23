/* BEGIN USAGE */
// DesignCanvas.jsx — Figma-ish design canvas wrapper
// Sections + Artboards + PostIt notes.
// Exports (to window): DesignCanvas, DCSection, DCArtboard, DCPostIt.
// Artboards are reorderable (grip-drag), deletable, labels/titles are
// inline-editable, and any artboard can be opened in a fullscreen focus
// overlay (←/→/Esc). State persists to a .design-canvas.state.json sidecar
// via the host bridge. No assets, no deps.
//
// Usage:
//   <DesignCanvas>
//     <DCSection id="onboarding" title="Onboarding" subtitle="First-run variants">
//       <DCArtboard id="a" label="A · Dusk" width={260} height={480}>…</DCArtboard>
//       <DCArtboard id="b" label="B · Minimal" width={260} height={480}>…</DCArtboard>
//     </DCSection>
//   </DesignCanvas>
//
// Artboards are static design frames, not scroll regions — never use
// height: 100% + overflow: auto/scroll on inner elements; size each artboard
// to fit its content. Height behavior:
//   • omit height  → the card grows to fit its content (no vertical clipping);
//                    this is the right default for full pages / long screens.
//   • height={N}   → a fixed N-px frame that CLIPS overflow (overflow:hidden) —
//                    use only for deliberately cropped thumbnails (e.g. A/B tiles).
// width is always a fixed frame width (default 260); content wider than it is
// clipped, so set width to the real design width for full pages.
//
// When placing a device frame inside a DCArtboard, pass chromeless to
// suppress the card chrome and let the artboard size to its content:
//   <DCArtboard chromeless><IOSDevice>…</IOSDevice></DCArtboard>
//
// On mobile (UA-detected) the canvas is browse-only: one-finger drag pans
// (starting on artboards too), two-finger pinch zooms, the zoom control is
// hidden, and drag-reorder / inline-rename are disabled. Tap on an artboard
// label still opens the focus overlay.
/* END USAGE */

const DC = {
  bg: '#FCFCFC',
  label: '#8F959E',
  title: '#1F2329',
  subtitle: '#646A73',
  postitBg: '#fef4a8',
  postitText: '#5a4a2a',
  font: '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif',
};

// ── 移动端判定：复刻主仓 @apaas-ai/global-states isMobile() 的 UA 语义
// （跨域产物 iframe 无法 import 该包，同 deck-stage 的先例）。刻意不用视口
// 断点——桌面工作台的预览 iframe 本身就窄，视口宽度会把桌面预览误判成移动端。
// 启动时一次性判定，不随 resize 抖动。
const DC_MOBILE_UA_RE = /(phone|pad|pod|iPhone|iPod|ios|iPad|Android|Mobile|BlackBerry|IEMobile|MQQBrowser|JUC|Fennec|wOSBrowser|BrowserNG|WebOS|Symbian|Windows Phone)/i;
const dcIsMobile = typeof navigator !== 'undefined' &&
  (DC_MOBILE_UA_RE.test(navigator.userAgent) ||
    (/iPad|Tab|Tablet/i.test(navigator.userAgent) && !!navigator.maxTouchPoints && navigator.maxTouchPoints > 1));

// One-time CSS injection (classes are dc-prefixed so they don't collide with
// the hosted design's own styles).
if (typeof document !== 'undefined' && !document.getElementById('dc-styles')) {
  const s = document.createElement('style');
  s.id = 'dc-styles';
  s.textContent = [
    '.dc-editable{cursor:text;outline:none;white-space:nowrap;border-radius:4px;padding:0 2px;margin:0 -2px}',
    '.dc-editable:focus{background:#fff;box-shadow:0 0 0 1px #336DF4}',
    '[data-dc-slot]{transition:transform .18s cubic-bezier(.2,.7,.3,1)}',
    '[data-dc-slot].dc-dragging{transition:none;z-index:10;pointer-events:none}',
    '[data-dc-slot].dc-dragging .dc-card{box-shadow:0 12px 40px rgba(0,0,0,.25),0 0 0 2px #336DF4;transform:scale(1.02)}',
    // isolation:isolate contains artboard content's z-indexes so a
    // z-indexed child (sticky navbar etc.) can't paint over .dc-header or
    // the .dc-menu popover that drops into the top of the card.
    '.dc-card{isolation:isolate;transition:box-shadow .15s,transform .15s}',
    '.dc-card *{scrollbar-width:none}',
    '.dc-card *::-webkit-scrollbar{display:none}',
    // Focus-overlay content isn't inside .dc-card, so the scrollbar-hiding
    // rules above don't reach it — re-hide scrollbars for the scaled,
    // scrollable content shown in DCFocusOverlay.
    '.dc-focus-content *{scrollbar-width:none}',
    '.dc-focus-content *::-webkit-scrollbar{display:none}',
    // Per-artboard header: grip + label on the left, delete/expand on the
    // right. Single flex row; when the artboard's on-screen width is too
    // narrow for both the label yields (ellipsis, then hidden entirely below
    // ~4ch via the container query) and the buttons stay on the row.
    '.dc-header{position:absolute;bottom:100%;left:-4px;margin-bottom:calc(4px * var(--dc-inv-zoom,1));z-index:2;',
    '  display:flex;align-items:center;container-type:inline-size}',
    '.dc-labelrow{display:flex;align-items:center;gap:4px;height:24px;flex:1 1 auto;min-width:0}',
    '.dc-grip{flex:0 0 auto;cursor:grab;display:flex;align-items:center;padding:5px 4px;border-radius:4px;transition:background .12s,opacity .12s}',
    '.dc-grip:hover{background:rgba(0,0,0,.08)}',
    '.dc-grip:active{cursor:grabbing}',
    '.dc-labeltext{flex:1 1 auto;min-width:0;cursor:pointer;border-radius:4px;padding:3px 6px;',
    '  display:flex;align-items:center;transition:background .12s;overflow:hidden}',
    // Below ~4ch of label room: hide the label entirely, and drop the grip to
    // hover-only (same reveal rule as .dc-btns) so a narrow header is clean
    // until the card is moused.
    '@container (max-width: 110px){',
    '  .dc-labeltext{display:none}',
    '  .dc-grip{opacity:0}',
    '  [data-dc-slot]:hover .dc-grip{opacity:1}',
    '}',
    '.dc-labeltext:hover{background:rgba(31,35,41,0.05)}',
    '.dc-labeltext .dc-editable{overflow:hidden;text-overflow:ellipsis;max-width:100%}',
    '.dc-labeltext .dc-editable:focus{overflow:visible;text-overflow:clip}',
    '.dc-btns{flex:0 0 auto;margin-left:auto;display:flex;gap:2px;opacity:0;transition:opacity .12s}',
    '[data-dc-slot]:hover .dc-header,[data-dc-slot]:hover .dc-header .dc-editable{color:#646A73!important}',
    '[data-dc-slot]:hover .dc-btns,.dc-btns:has(.dc-menu){opacity:1}',
    // On touch devices (no hover) the per-artboard action bar (kebab + expand)
    // can't be reveal-on-hover and tends to stick visible after a tap — hide it
    // entirely on mobile. The grip + label stay for per-screen context.
    '@media (hover: none){.dc-btns{display:none}}',
    // Mobile (UA-detected, .dc-mobile on the viewport root) is browse-only:
    // drag-reorder is disabled there, so the grip is dead weight — hide it.
    '.dc-mobile .dc-grip{display:none}',
    // Mobile is browse-only: kill text selection + iOS long-press callout
    // inside the canvas. iOS interprets a drag over selectable TEXT as a
    // selection gesture and steals the pointer (pointercancel) — touch-action
    // only suppresses native pan/zoom, not selection — so text-dense
    // artboards became un-pannable while image-heavy ones panned fine.
    '.dc-mobile, .dc-mobile *{-webkit-user-select:none;user-select:none;-webkit-touch-callout:none}',
    // Prototype content routinely carries inner scrollers (the device/window
    // frames all have overflow:auto bodies). On iOS a touch starting inside a
    // scrollable element is consumed by THAT scroller (native scroll attempt
    // + pointercancel), so pans could never start on those artboards while
    // plain-HTML cards panned fine. Mobile canvas is browse-only: kill native
    // touch handling on everything inside the cards so gestures always reach
    // the viewport state machine. Desktop (no .dc-mobile) keeps inner scroll.
    '.dc-mobile [data-dc-slot] *{touch-action:none !important}',
    // Pinch updates --dc-inv-zoom per frame (PC semantics: chrome size AND
    // section spacing stay constant). The per-frame relayout that implies
    // white-flashed on WKWebView when the whole world was one paint layer —
    // every head resize invalidated and re-rasterized ALL content. Promote
    // each card (and head) to its own compositing layer: relayout then only
    // MOVES card layers (compositor, no repaint) and repaints just the tiny
    // head layers. contain:paint bounds invalidation to the card box.
    // contain:paint must live on .dc-card, NOT the slot: .dc-header is an
    // abs-positioned slot child sitting entirely ABOVE the slot box
    // (bottom:100%) — paint containment on the slot would clip it out of
    // rendering AND hit-testing, killing the label tap that is mobile's only
    // focus-overlay entry. will-change (the layer-promotion half of the
    // white-flash fix) stays on the slot so header + card share one layer.
    '.dc-mobile [data-dc-slot]{will-change:transform}',
    '.dc-mobile .dc-card{contain:paint}',
    '.dc-mobile .dc-sectionhead{will-change:transform}',
    '[data-dc-slot]:hover .dc-card.chromeless>*,  [data-dc-slot]:hover .dc-card:not(.chromeless){box-shadow:0 4px 8px -8px rgba(0, 0, 0, 0.06), 0 6px 12px 0 rgba(0, 0, 0, 0.04), 0 8px 24px 8px rgba(0, 0, 0, 0.04)}',
    '.dc-expand,.dc-kebab{width:22px;height:22px;border-radius:6px;border:none;cursor:pointer;padding:0;',
    '  background:transparent;color:#646A73;display:flex;align-items:center;justify-content:center;',
    '  font:inherit;transition:background .12s,color .12s}',
    '.dc-expand:hover,.dc-kebab:hover{background:rgba(31,35,41,0.05);color:#646A73}',
    // Slot hosting an open menu floats above later siblings (which otherwise
    // paint on top — same z-index:auto, later DOM order) so the popup isn't
    // clipped by the next card.
    '[data-dc-slot]:has(.dc-menu){z-index:10}',
    '.dc-menu{position:absolute;top:100%;right:0;margin-top:4px;background:#fff;border-radius:8px;border:1px solid #DEE0E3;',
    '  box-shadow:0 4px 8px 0 rgba(0,0,0,0.03),0 3px 6px -6px rgba(0,0,0,0.05),0 6px 18px 6px rgba(0,0,0,0.03);padding:3px;min-width:120px;z-index:10}',
    '.dc-menu button{display:block;width:100%;padding:4px 8px;border:0;background:transparent;',
    '  border-radius:6px;font-family:inherit;font-size:14px;font-weight:400;line-height:24px;',
    '  color:#1F2329;cursor:pointer;text-align:left;transition:background .12s;white-space:nowrap}',
    '.dc-menu button:hover{background:rgba(31,35,41,0.05)}',
    '.dc-menu hr{border:0;border-top:1px solid rgba(0,0,0,.08);margin:4px 2px}',
    '.dc-menu .dc-danger{color:#F54A45}',
    // Chrome (titles / labels / buttons) counter-scales against the viewport
    // zoom so it stays a constant on-screen size. --dc-inv-zoom is set by
    // DCViewport on every transform update and inherits to all descendants —
    // any overlay inside the world (e.g. a TweaksPanel on an artboard) can use
    // it the same way.
    //
    // The header uses transform:scale (out-of-flow, so layout impact doesn't
    // matter) with its world-space width set to card-width / inv-zoom so that
    // after counter-scaling its on-screen width exactly matches the card's —
    // that's what lets the container query + text-overflow behave against the
    // card's visible edge at every zoom level.
    //
    // The section head uses CSS zoom instead of transform so its layout box
    // grows with the counter-scale, pushing the card row down — otherwise the
    // constant-screen-size title would overflow into the (shrinking) world-
    // space gap and overlap the artboard headers at low zoom.
    '.dc-header{width:calc((100% + 4px) / var(--dc-inv-zoom,1));',
    '  transform:scale(var(--dc-inv-zoom,1));transform-origin:bottom left;will-change:transform}',
    '.dc-sectionhead{zoom:var(--dc-inv-zoom,1)}',
    // On-canvas zoom control (bottom-right pill). Stays screen-fixed because
    // it renders outside worldRef. z-index 50 keeps it above world content but
    // below the focus overlay (z-index 100). The dropdown reuses .dc-menu and
    // only overrides its positioning to open upward (.dc-zoom-menu).
    '.dc-zoom{position:absolute;bottom:16px;right:20px;z-index:50;display:flex;align-items:center;',
    '  background:#fff;border-radius:8px;padding:4px;font-family:inherit;',
    '  border:0.5px solid #DEE0E3;box-shadow:0 2px 4px -4px rgba(31,35,41,0.02),0 4px 8px 0 rgba(31,35,41,0.02),0 4px 16px 4px rgba(31,35,41,0.03)}',
    '.dc-zoom-btn{height:24px;min-width:24px;border-radius:6px;border:none;cursor:pointer;padding:0;',
    '  background:transparent;color:rgba(60,50,40,.7);display:flex;align-items:center;justify-content:center;',
    '  font:inherit;transition:background .12s,color .12s}',
    '.dc-zoom-btn:hover{background:#1f23290d}',
    '.dc-zoom-btn:disabled{opacity:.35;cursor:default;background:transparent;color:rgba(60,50,40,.7)}',
    '.dc-zoom-toggle{position:relative;display:flex}',
    '.dc-zoom-trigger{padding:0 6px;gap:4px}',
    '.dc-zoom-pct{min-width:36px;text-align:center;font-size:12px;font-weight:400;color:#1F2329}',
    '.dc-zoom-menu{top:auto;bottom:100%;left:50%;transform:translateX(-50%);margin-top:0;margin-bottom:6px;min-width:114px}',
    '.dc-zoom-menu button{display:flex;align-items:center;justify-content:space-between;gap:8px}',
  ].join('\n');
  document.head.appendChild(s);
}

const DCCtx = React.createContext(null);

// Recursively unwrap React.Fragment so <>…</> grouping doesn't hide
// DCSection/DCArtboard children from the type-based walks below.
function dcFlatten(children) {
  const out = [];
  React.Children.forEach(children, (c) => {
    if (c && c.type === React.Fragment) out.push(...dcFlatten(c.props.children));
    else out.push(c);
  });
  return out;
}

// ─────────────────────────────────────────────────────────────
// DesignCanvas — stateful wrapper around the pan/zoom viewport.
// Owns runtime state (per-section order, renamed titles/labels, hidden
// artboards, focused artboard). Order/titles/labels/hidden persist to a
// .design-canvas.state.json
// sidecar next to the HTML. Reads go via plain fetch() so the saved
// arrangement is visible anywhere the HTML + sidecar are served together
// (miaoda preview, direct link, downloaded zip). Writes post the sidecar
// content up to the miaoda host via postMessage — editing requires the miaoda runtime.
// Focus is ephemeral.
// ─────────────────────────────────────────────────────────────
const DC_STATE_FILE = '.design-canvas.state.json';

// Persist a sidecar file back to the host.
function miaodaWriteFile(path, content) {
  try {
    window.parent.postMessage({ type: 'miaoda:bridge:write-file', path: path, content: content }, '*');
  } catch (e) {
    /* no parent — ignore */
  }
  return Promise.resolve();
}

function DesignCanvas({ children, minScale, maxScale, style }) {
  const [state, setState] = React.useState({ sections: {}, focus: null });
  // Hold rendering until the sidecar read settles so the saved order/titles
  // appear on first paint (no source-order flash). didRead gates writes until
  // the read settles so the empty initial state can't clobber a slow read;
  // skipNextWrite suppresses the one echo-write that would otherwise follow
  // hydration.
  const [ready, setReady] = React.useState(false);
  const didRead = React.useRef(false);
  const skipNextWrite = React.useRef(false);

  React.useEffect(() => {
    let off = false;
    fetch('./' + DC_STATE_FILE)
      .then((r) => (r.ok ? r.json() : null))
      .then((saved) => {
        if (off || !saved || !saved.sections) return;
        skipNextWrite.current = true;
        setState((s) => ({ ...s, sections: saved.sections }));
      })
      .catch(() => {})
      .finally(() => { didRead.current = true; if (!off) setReady(true); });
    const t = setTimeout(() => { if (!off) setReady(true); }, 150);
    return () => { off = true; clearTimeout(t); };
  }, []);

  React.useEffect(() => {
    if (!didRead.current) return;
    if (skipNextWrite.current) { skipNextWrite.current = false; return; }
    const t = setTimeout(() => {
      miaodaWriteFile(DC_STATE_FILE, JSON.stringify({ sections: state.sections })).catch(() => {});
    }, 250);
    return () => clearTimeout(t);
  }, [state.sections]);

  // Build registries synchronously from children so FocusOverlay can read
  // them in the same render. Fragments are flattened; wrapping in other
  // elements still opts out of focus/reorder.
  const registry = {};     // slotId -> { sectionId, artboard }
  const sectionMeta = {};  // sectionId -> { title, subtitle, slotIds[] }
  const sectionOrder = [];
  dcFlatten(children).forEach((sec) => {
    if (!sec || sec.type !== DCSection) return;
    const sid = sec.props.id ?? sec.props.title;
    if (!sid) return;
    sectionOrder.push(sid);
    const persisted = state.sections[sid] || {};
    const abs = [];
    dcFlatten(sec.props.children).forEach((ab) => {
      if (!ab || ab.type !== DCArtboard) return;
      const aid = ab.props.id ?? ab.props.label;
      if (aid) abs.push([aid, ab]);
    });
    // hidden is scoped to one source revision — when the agent regenerates
    // (artboard-ID set changes), prior deletes don't apply to new content.
    const srcKey = abs.map(([k]) => k).join('\x1f');
    const hidden = persisted.srcKey === srcKey ? (persisted.hidden || []) : [];
    const srcIds = [];
    abs.forEach(([aid, ab]) => {
      if (hidden.includes(aid)) return;
      registry[`${sid}/${aid}`] = { sectionId: sid, artboard: ab };
      srcIds.push(aid);
    });
    const kept = (persisted.order || []).filter((k) => srcIds.includes(k));
    sectionMeta[sid] = {
      title: persisted.title ?? sec.props.title,
      subtitle: sec.props.subtitle,
      slotIds: [...kept, ...srcIds.filter((k) => !kept.includes(k))],
    };
  });

  const api = React.useMemo(() => ({
    state,
    section: (id) => state.sections[id] || {},
    patchSection: (id, p) => setState((s) => ({
      ...s,
      sections: { ...s.sections, [id]: { ...s.sections[id], ...(typeof p === 'function' ? p(s.sections[id] || {}) : p) } },
    })),
    setFocus: (slotId) => setState((s) => ({ ...s, focus: slotId })),
  }), [state]);

  // Esc exits focus; any outside pointerdown commits an in-progress rename.
  React.useEffect(() => {
    const onKey = (e) => { if (e.key === 'Escape') api.setFocus(null); };
    const onPd = (e) => {
      const ae = document.activeElement;
      if (ae && ae.isContentEditable && !ae.contains(e.target)) ae.blur();
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('pointerdown', onPd, true);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('pointerdown', onPd, true);
    };
  }, [api]);

  // dcOverlay opt-in: render outside DCViewport so the component is free from
  // worldRef's transform (pan/zoom). Unmarked children stay in the canvas.
  const flat = dcFlatten(children);
  const overlayChildren = flat.filter(c => c && c.type?.dcOverlay);
  const worldChildren = flat.filter(c => !c || !c.type?.dcOverlay);

  return (
    <DCCtx.Provider value={api}>
      <DCViewport minScale={minScale} maxScale={maxScale} style={style}>{ready && worldChildren}</DCViewport>
      {ready && overlayChildren}
      {state.focus && registry[state.focus] && (
        <DCFocusOverlay entry={registry[state.focus]} sectionMeta={sectionMeta} sectionOrder={sectionOrder} />
      )}
    </DCCtx.Provider>
  );
}

// ─────────────────────────────────────────────────────────────
// DCZoomControl — bottom-right zoom pill (zoom-in / % + presets / zoom-out)
//
// Renders outside worldRef so it stays screen-fixed. data-miaoda-chrome marks
// it as non-background so the viewport's pointerdown doesn't start a pan on it
// (the native vp listener can't be stopped by React's synthetic propagation,
// so the attribute — not stopPropagation — is what prevents the pan). Buttons
// step to the nearest 10% grid, clamped to the canvas scale range; the dropdown
// offers common presets and opens upward.
// ─────────────────────────────────────────────────────────────
const DC_ZOOM_PRESETS = [20, 35, 50, 75, 100, 125, 150, 175, 200];

function DCZoomControl({ scalePct, minScale, maxScale, onZoomTo }) {
  const [open, setOpen] = React.useState(false);
  const rootRef = React.useRef(null);
  const minPct = Math.round(minScale * 100);
  const maxPct = Math.round(maxScale * 100);
  const snapUp = Math.min(maxPct, (Math.floor(scalePct / 10) + 1) * 10);
  const snapDown = Math.max(minPct, (Math.ceil(scalePct / 10) - 1) * 10);

  React.useEffect(() => {
    if (!open) return;
    const onDocDown = (e) => {
      if (rootRef.current && !rootRef.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('pointerdown', onDocDown);
    return () => document.removeEventListener('pointerdown', onDocDown);
  }, [open]);

  return (
    <div
      ref={rootRef}
      className="dc-zoom"
      data-miaoda-chrome=""
      onPointerDown={(e) => e.stopPropagation()}
    >
      <button
        type="button"
        className="dc-zoom-btn"
        disabled={scalePct >= maxPct}
        onClick={() => onZoomTo(snapUp)}
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M12.4957 11.5517L14.9105 13.9668C15.1702 14.2266 15.1684 14.648 14.9087 14.9077C14.649 15.1674 14.2275 15.1692 13.9678 14.9095L11.5527 12.4943C10.3631 13.4696 8.87188 14.0016 7.33366 13.9993C3.65166 13.9993 0.666992 11.0147 0.666992 7.33268C0.666992 3.65068 3.65166 0.666016 7.33366 0.666016C11.0157 0.666016 14.0003 3.65068 14.0003 7.33268C14.0003 8.93368 13.436 10.4027 12.4957 11.5517ZM6.66699 6.66602V5.33268C6.66699 4.96449 6.96547 4.66602 7.33366 4.66602C7.70185 4.66602 8.00033 4.96449 8.00033 5.33268V6.66602H9.33366C9.70185 6.66602 10.0003 6.96449 10.0003 7.33268C10.0003 7.70087 9.70185 7.99935 9.33366 7.99935H8.00033V9.33268C8.00033 9.70087 7.70185 9.99935 7.33366 9.99935C6.96547 9.99935 6.66699 9.70087 6.66699 9.33268V7.99935H5.33366C4.96547 7.99935 4.66699 7.70087 4.66699 7.33268C4.66699 6.96449 4.96547 6.66602 5.33366 6.66602H6.66699ZM7.33366 12.666C10.2793 12.666 12.667 10.2783 12.667 7.33268C12.667 4.38702 10.2793 1.99935 7.33366 1.99935C4.38799 1.99935 2.00033 4.38702 2.00033 7.33268C2.00033 10.2783 4.38799 12.666 7.33366 12.666Z" fill="#1F2329"/>
        </svg>
      </button>

      <div className="dc-zoom-toggle">
        <button
          type="button"
          className="dc-zoom-btn dc-zoom-trigger"
          onClick={() => setOpen((o) => !o)}
        >
          <span className="dc-zoom-pct">{scalePct}%</span>
          <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M2 3.5L5 6.5L8 3.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
        {open && (
          <div className="dc-menu dc-zoom-menu" onPointerDown={(e) => e.stopPropagation()}>
            {DC_ZOOM_PRESETS.map((p) => (
              <button type="button" key={p} onClick={() => { onZoomTo(p); setOpen(false); }}>
                <span>{p}%</span>
                {p === scalePct && (
                  <svg width="12" height="12" viewBox="0 0 12 12" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M2.00002 5.64639C2.09378 5.55266 2.22094 5.5 2.35352 5.5C2.4861 5.5 2.61325 5.55266 2.70702 5.64639L4.74277 7.68189L9.27827 3.14639C9.37203 3.05266 9.49919 3 9.63177 3C9.76435 3 9.8915 3.05266 9.98527 3.14639L10.3388 3.49989C10.3852 3.54633 10.4221 3.60145 10.4472 3.66213C10.4723 3.72281 10.4853 3.78784 10.4853 3.85352C10.4853 3.9192 10.4723 3.98423 10.4472 4.04491C10.4221 4.10558 10.3852 4.16071 10.3388 4.20714L5.09627 9.44965C5.04984 9.4961 4.99471 9.53295 4.93403 9.55808C4.87336 9.58322 4.80832 9.59616 4.74264 9.59616C4.67697 9.59616 4.61193 9.58322 4.55125 9.55808C4.49058 9.53295 4.43545 9.4961 4.38902 9.44965L1.64652 6.70714C1.60007 6.66071 1.56322 6.60558 1.53808 6.54491C1.51294 6.48423 1.5 6.4192 1.5 6.35352C1.5 6.28784 1.51294 6.22281 1.53808 6.16213C1.56322 6.10145 1.60007 6.04633 1.64652 5.99989L2.00002 5.64639Z" fill="#1F2329"/>
                  </svg>
                )}
              </button>
            ))}
          </div>
        )}
      </div>

      <button
        type="button"
        className="dc-zoom-btn"
        disabled={scalePct <= minPct}
        onClick={() => onZoomTo(snapDown)}
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M12.4957 11.5517L14.9111 13.9674C15.1707 14.227 15.1697 14.6496 14.9102 14.9092C14.6506 15.1688 14.228 15.1697 13.9684 14.9101L11.5527 12.4943C10.3631 13.4696 8.87188 14.0016 7.33366 13.9993C3.65166 13.9993 0.666992 11.0147 0.666992 7.33268C0.666992 3.65068 3.65166 0.666016 7.33366 0.666016C11.0157 0.666016 14.0003 3.65068 14.0003 7.33268C14.0003 8.93368 13.436 10.4027 12.4957 11.5517ZM7.33366 12.666C10.2793 12.666 12.667 10.2783 12.667 7.33268C12.667 4.38702 10.2793 1.99935 7.33366 1.99935C4.38799 1.99935 2.00033 4.38702 2.00033 7.33268C2.00033 10.2783 4.38799 12.666 7.33366 12.666ZM4.66699 7.33268C4.66699 6.96449 4.96547 6.66602 5.33366 6.66602H9.33366C9.70185 6.66602 10.0003 6.96449 10.0003 7.33268C10.0003 7.70087 9.70185 7.99935 9.33366 7.99935H5.33366C4.96547 7.99935 4.66699 7.70087 4.66699 7.33268Z" fill="#1F2329"/>
        </svg>
      </button>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// DCViewport — transform-based pan/zoom (internal)
//
// Input mapping (Figma-style):
//   • trackpad pinch                     → zoom  (ctrlKey wheel; Safari gesture* events)
//   • Ctrl/⌘ + wheel                     → zoom
//   • trackpad two-finger / mouse wheel  → pan
//   • middle-drag / primary-drag-on-bg   → pan
//
// Transform state lives in a ref and is written straight to the DOM
// (translate3d + will-change) so wheel ticks don't go through React —
// keeps pans at 60fps on dense canvases.
// ─────────────────────────────────────────────────────────────
function DCViewport({ children, minScale = 0.1, maxScale = 8, style = {} }) {
  const vpRef = React.useRef(null);
  const worldRef = React.useRef(null);
  // Pre-fit fallback scale. A fresh canvas is reframed by the first-visit fit
  // (below) once content mounts; localStorage restore overrides this on
  // revisits. This 50% only shows in the gap before either runs.
  const tf = React.useRef({ x: 0, y: 0, scale: 0.5 });
  // Persist viewport across reloads so the user lands back where they were
  // after an agent edit or browser refresh. The sandbox origin is already
  // per-project; pathname keeps multiple canvas files in one project apart.
  const tfKey = 'dc-viewport:' + location.pathname;
  const saveT = React.useRef(0);

  const lastPostedScale = React.useRef();
  // Imperative handle for the on-canvas zoom control to zoom around the
  // viewport centre (same path as the host's set-zoom message). Assigned in
  // the pointer/wheel effect below where zoomAt is in scope.
  const zoomCenteredRef = React.useRef(null);
  // Percentage shown by the zoom control. Synced from apply() on every scale
  // change — React bails out when the rounded value is unchanged, so pan ticks
  // (scale unchanged) cost nothing. Lazily seeded from the persisted transform
  // so the first paint already shows the restored — or default 50% — zoom.
  const [scalePct, setScalePct] = React.useState(() => {
    try {
      const s = JSON.parse(localStorage.getItem(tfKey) || 'null');
      if (s && Number.isFinite(s.scale)) {
        return Math.round(Math.min(maxScale, Math.max(minScale, s.scale)) * 100);
      }
    } catch {}
    return Math.round(tf.current.scale * 100);
  });
  const apply = React.useCallback(() => {
    const { x, y, scale } = tf.current;
    const el = worldRef.current;
    if (!el) return;
    el.style.transform = `translate3d(${x}px, ${y}px, 0) scale(${scale})`;
    // Exposed for zoom-invariant chrome (labels, buttons, TweaksPanel).
    el.style.setProperty('--dc-inv-zoom', String(1 / scale));
    // Keep the on-canvas zoom control's % readout in sync. React bails out on
    // an unchanged rounded value, so pan ticks (scale unchanged) don't render.
    // Mobile renders no zoom control — a per-frame setState during pinch
    // would re-render the whole subtree every frame (visible repaint flicker
    // on phones) for a readout nobody consumes, so skip it there entirely.
    if (!dcIsMobile) setScalePct(Math.round(scale * 100));
    // Keep the host toolbar's % readout in sync with the canvas scale. Pan
    // ticks leave scale unchanged — skip the cross-frame post for those.
    if (lastPostedScale.current !== scale) {
      lastPostedScale.current = scale;
      window.parent.postMessage({ type: 'miaoda:canvas:zoom', scale }, '*');
    }
    clearTimeout(saveT.current);
    saveT.current = setTimeout(() => {
      try { localStorage.setItem(tfKey, JSON.stringify(tf.current)); } catch {}
    }, 200);
  }, [tfKey]);

  // Whether the mount-time restore found a saved viewport — when it did, the
  // first-visit fit below is skipped so revisits keep the user's own view.
  const restoredView = React.useRef(false);
  const didFit = React.useRef(false);

  React.useLayoutEffect(() => {
    const flush = () => {
      clearTimeout(saveT.current);
      try { localStorage.setItem(tfKey, JSON.stringify(tf.current)); } catch {}
    };
    try {
      const s = JSON.parse(localStorage.getItem(tfKey) || 'null');
      if (s && Number.isFinite(s.x) && Number.isFinite(s.y) && Number.isFinite(s.scale)) {
        tf.current = { x: s.x, y: s.y, scale: Math.min(maxScale, Math.max(minScale, s.scale)) };
        restoredView.current = true;
      }
    } catch {}
    // Apply before first paint whether we restored or fell back to the default
    // (scale 0.5) so the canvas never flashes at the wrong zoom. On a fresh
    // canvas the first-visit fit (below) reframes once content mounts.
    apply();
    // Flush on pagehide and unmount so a reload within the 200ms debounce
    // window doesn't drop the last pan/zoom.
    window.addEventListener('pagehide', flush);
    return () => { window.removeEventListener('pagehide', flush); flush(); };
  }, []);

  // First-visit framing: once the (ready-gated) content mounts, scale + centre
  // the whole canvas so every artboard fits with a ≥64px safe margin on all
  // sides. Skipped when a saved viewport was restored. Runs as a layout effect
  // on the commit that first paints content, so there's no un-framed flash.
  React.useLayoutEffect(() => {
    if (didFit.current || restoredView.current) return;
    const vp = vpRef.current, world = worldRef.current;
    if (!vp || !world) return;
    const slots = world.querySelectorAll('[data-dc-slot]');
    if (!slots.length) return; // content not mounted yet — wait for the ready commit
    const heads = world.querySelectorAll('.dc-sectionhead');
    // Fit margin from the screen edges. Mobile screens are narrow — 64px a
    // side eats a third of the width, so fit flush-ish at 16px there.
    const PAD = dcIsMobile ? 16 : 64;

    // Content bounding box in WORLD space at the currently applied transform.
    // X extent from cards only (section-head blocks are full width and would
    // over-report); Y from cards + titles so the top heading is kept in frame.
    const measure = () => {
      const vpr = vp.getBoundingClientRect();
      const t = tf.current;
      let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
      slots.forEach((el) => {
        const r = el.getBoundingClientRect();
        minX = Math.min(minX, (r.left - vpr.left - t.x) / t.scale);
        maxX = Math.max(maxX, (r.right - vpr.left - t.x) / t.scale);
        minY = Math.min(minY, (r.top - vpr.top - t.y) / t.scale);
        maxY = Math.max(maxY, (r.bottom - vpr.top - t.y) / t.scale);
      });
      heads.forEach((el) => {
        const r = el.getBoundingClientRect();
        minY = Math.min(minY, (r.top - vpr.top - t.y) / t.scale);
        maxY = Math.max(maxY, (r.bottom - vpr.top - t.y) / t.scale);
      });
      return { vpr, minX, minY, w: maxX - minX, h: maxY - minY };
    };

    // Content SCREEN width is linear in scale (screenW = s·Wworld) and SCREEN
    // height is affine (screenH = s·P + Q): titles and inter-section gaps
    // counter-scale with zoom, so they hold a constant screen size while cards
    // scale. Sample two scales to solve the coefficients, then pick the largest
    // scale that keeps both axes within the viewport minus a PAD margin — exact,
    // no iterate-to-converge.
    const s1 = tf.current.scale;
    const m1 = measure();
    if (!(m1.w > 0) || !(m1.h > 0)) { didFit.current = true; return; }
    const s2 = s1 * 0.7;
    tf.current.scale = s2; apply();
    const m2 = measure();

    const { vpr } = m1;
    const avW = vpr.width - PAD * 2, avH = vpr.height - PAD * 2;
    const Wworld = m1.w;                              // screenW / s, scale-invariant
    const P = (s1 * m1.h - s2 * m2.h) / (s1 - s2);    // d(screenH)/d(scale)
    const Q = s1 * m1.h - s1 * P;                     // constant screen-space part
    const fitW = avW / Wworld;
    const fitH = P > 0 ? (avH - Q) / P : Infinity;
    const next = Math.max(minScale, Math.min(maxScale, Math.min(fitW, fitH)));

    // Apply the solved scale, then centre using the extent measured AT that
    // scale so margins come out symmetric (exactly PAD on the binding axis).
    tf.current.scale = next; apply();
    const mf = measure();
    tf.current.x = (vpr.width - mf.w * next) / 2 - mf.minX * next;
    tf.current.y = (vpr.height - mf.h * next) / 2 - mf.minY * next;
    apply();
    didFit.current = true;
  });

  React.useEffect(() => {
    const vp = vpRef.current;
    if (!vp) return;

    const zoomAt = (cx, cy, factor) => {
      const r = vp.getBoundingClientRect();
      const px = cx - r.left, py = cy - r.top;
      const t = tf.current;
      const next = Math.min(maxScale, Math.max(minScale, t.scale * factor));
      const k = next / t.scale;
      // --dc-inv-zoom consumers (.dc-sectionhead's CSS zoom, each section's
      // marginBottom) reflow on every scale change, vertically shifting the
      // world layout — so a world point mathematically pinned under the cursor
      // drifts as you zoom (content creeps up on zoom-in, down on zoom-out).
      // Anchor the DOM element under the cursor instead: record its screen Y,
      // apply the transform + --dc-inv-zoom, then cancel whatever vertical
      // drift the reflow introduced so it stays put on screen.
      let marker = null, markerY0 = 0;
      if (k !== 1) {
        const hit = document.elementFromPoint(cx, cy);
        marker = hit && hit.closest ? hit.closest('[data-dc-slot],[data-dc-section]') : null;
        if (marker) markerY0 = marker.getBoundingClientRect().top;
      }
      // keep the world point under the cursor fixed
      t.x = px - (px - t.x) * k;
      t.y = py - (py - t.y) * k;
      t.scale = next;
      apply();
      if (marker) {
        // A pure zoom around (cx, cy) maps screen Y → cy + (Y - cy) * k. Any
        // departure after the --dc-inv-zoom reflow is the layout drift.
        const drift = marker.getBoundingClientRect().top - (cy + (markerY0 - cy) * k);
        if (Math.abs(drift) > 0.1) { t.y -= drift; apply(); }
      }
    };

    // On-canvas zoom control: zoom to a target scale around the viewport
    // centre, matching the host set-zoom path (onHostMsg). Closes over the
    // live zoomAt / vp / tf, so it always uses the current transform.
    zoomCenteredRef.current = (targetScale) => {
      const r = vp.getBoundingClientRect();
      const clamped = Math.min(maxScale, Math.max(minScale, targetScale));
      zoomAt(r.left + r.width / 2, r.top + r.height / 2, clamped / tf.current.scale);
    };

    // Zoom is gated behind a modifier only: the browser sets ctrlKey for a
    // trackpad pinch, and Ctrl/⌘ + wheel is the explicit mouse-zoom gesture.
    // Every unmodified scroll — trackpad two-finger OR mouse wheel — pans.
    // This is Figma's mapping, and it sidesteps a classification that can't be
    // done reliably: a fast vertical trackpad flick's peak frame is identical
    // to a mouse-wheel notch (deltaMode 0, deltaX 0, large integer deltaY), so
    // routing pan-vs-zoom from a single wheel event misfires — a quick two-
    // finger swipe used to jump into zoom. Keying off the modifier removes the
    // ambiguity at the source.
    //
    // isNotchedWheel now only picks the zoom *step size* inside the zoom
    // branch (fixed ratio for discrete wheels / Firefox line-mode, continuous
    // exp for a smooth pinch) — misjudging it is a harmless granularity
    // difference, never a pan↔zoom flip.
    const isNotchedWheel = (e) =>
      e.deltaMode !== 0 ||
      (e.deltaX === 0 && Number.isInteger(e.deltaY) && Math.abs(e.deltaY) >= 40);

    const onWheel = (e) => {
      e.preventDefault();
      if (isGesturing) return; // Safari: gesture* owns the pinch — discard concurrent wheels
      if (e.ctrlKey || e.metaKey) {
        zoomAt(e.clientX, e.clientY,
          isNotchedWheel(e) ? Math.exp(-Math.sign(e.deltaY) * 0.18) : Math.exp(-e.deltaY * 0.01));
      } else {
        // Unmodified scroll pans. Normalize line/page deltas so a notched
        // wheel moves a sensible distance instead of a few pixels per notch.
        const unit = e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? vp.clientHeight : 1;
        tf.current.x -= e.deltaX * unit;
        tf.current.y -= e.deltaY * unit;
        apply();
      }
    };

    // Safari sends native gesture* events for trackpad pinch with a smooth
    // e.scale; preferring these over the ctrl+wheel fallback gives a much
    // better feel there. No-ops on other browsers. Safari also fires
    // ctrlKey wheel events during the same pinch — isGesturing makes
    // onWheel drop those entirely so they neither zoom nor pan.
    //
    // On mobile the pointer-based two-finger pinch below is the single zoom
    // source. iOS fires BOTH pointer events and these WebKit gesture* events
    // for the same two fingers — zooming from both would double-apply, so on
    // mobile gesture* degrades to preventDefault only (still suppressing the
    // native page zoom).
    let gsBase = 1;
    let isGesturing = false;
    const onGestureStart = (e) => { e.preventDefault(); isGesturing = true; gsBase = tf.current.scale; };
    const onGestureChange = (e) => {
      e.preventDefault();
      if (dcIsMobile) return;
      zoomAt(e.clientX, e.clientY, (gsBase * e.scale) / tf.current.scale);
    };
    const onGestureEnd = (e) => { e.preventDefault(); isGesturing = false; };

    // Drag-pan: middle button anywhere, or primary button on canvas
    // background (anything that isn't an artboard or an inline editor).
    let drag = null;
    const onPointerDown = (e) => {
      const onBg = !e.target.closest('[data-dc-slot], .dc-editable, [data-miaoda-chrome]');
      if (!(e.button === 1 || (e.button === 0 && onBg))) return;
      e.preventDefault();
      vp.setPointerCapture(e.pointerId);
      drag = { id: e.pointerId, lx: e.clientX, ly: e.clientY };
      vp.style.cursor = 'grabbing';
    };
    const onPointerMove = (e) => {
      if (!drag || e.pointerId !== drag.id) return;
      tf.current.x += e.clientX - drag.lx;
      tf.current.y += e.clientY - drag.ly;
      drag.lx = e.clientX; drag.ly = e.clientY;
      apply();
    };
    const onPointerUp = (e) => {
      if (!drag || e.pointerId !== drag.id) return;
      vp.releasePointerCapture(e.pointerId);
      drag = null;
      vp.style.cursor = '';
    };

    // ── Mobile touch gestures (replaces the desktop pointer mapping above):
    // one-finger drag pans — starting on artboards too, since cards fill a
    // phone viewport and background is scarce; two-finger pinch zooms about
    // the midpoint while the midpoint's own travel pans (Figma semantics, one
    // gesture frames the view). A tap/pan slop keeps label taps working: no
    // capture happens within the slop, so a clean tap still clicks through to
    // focus; past it the viewport captures the pointer and the click retargets
    // harmlessly. No fling/inertia — release stops the canvas (spec §8).
    const touches = new Map(); // pointerId -> last {x, y}
    let mobMode = null;        // null | 'tap' | 'pan' | 'pinch'
    let tapStart = null;       // down point for slop arbitration
    let pinchDist = 0;         // previous-frame finger distance
    let pinchMid = null;       // previous-frame midpoint
    const MOB_SLOP = 9;        // px of travel before a tap becomes a pan

    // First two insertion-ordered touches define the pinch; a third finger is
    // tracked but inert (its moves shift neither midpoint nor distance).
    const pinchGeom = () => {
      const [a, b] = [...touches.values()];
      return { d: Math.hypot(a.x - b.x, a.y - b.y), mx: (a.x + b.x) / 2, my: (a.y + b.y) / 2 };
    };

    const onTouchDown = (e) => {
      touches.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (touches.size === 1) {
        mobMode = 'tap';
        tapStart = { x: e.clientX, y: e.clientY };
      } else if (touches.size === 2) {
        mobMode = 'pinch';
        const g = pinchGeom();
        pinchDist = g.d;
        pinchMid = { x: g.mx, y: g.my };
        try { vp.setPointerCapture(e.pointerId); } catch {}
      }
    };
    const onTouchMove = (e) => {
      const t = touches.get(e.pointerId);
      if (!t) return;
      const px = t.x, py = t.y;
      t.x = e.clientX; t.y = e.clientY;
      if (mobMode === 'tap' && Math.hypot(t.x - tapStart.x, t.y - tapStart.y) > MOB_SLOP) {
        mobMode = 'pan';
        try { vp.setPointerCapture(e.pointerId); } catch {}
      }
      if (mobMode === 'pan') {
        tf.current.x += t.x - px;
        tf.current.y += t.y - py;
        apply();
      } else if (mobMode === 'pinch') {
        const g = pinchGeom();
        tf.current.x += g.mx - pinchMid.x;
        tf.current.y += g.my - pinchMid.y;
        pinchMid = { x: g.mx, y: g.my };
        // zoomAt applies the pan mutation above too; degenerate distances
        // (fingers together) skip the zoom but still need the pan applied.
        // Same full zoomAt as the desktop trackpad-pinch path (gesturechange):
        // per-frame --dc-inv-zoom keeps chrome size AND section spacing
        // constant throughout the gesture, with the same drift compensation.
        if (pinchDist > 0 && g.d > 0) zoomAt(g.mx, g.my, g.d / pinchDist);
        else apply();
        pinchDist = g.d;
      }
    };
    const onTouchUp = (e) => {
      if (!touches.delete(e.pointerId)) return;
      try { vp.releasePointerCapture(e.pointerId); } catch {}
      if (touches.size === 0) {
        mobMode = null;
      } else if (touches.size === 1) {
        // Pinch collapsing to one finger hands off to pan with the surviving
        // finger as the new baseline — no jump.
        mobMode = 'pan';
      } else if (mobMode === 'pinch') {
        // 3+ fingers losing one: re-baseline so the new leading pair doesn't
        // read as a sudden midpoint/distance delta.
        const g = pinchGeom();
        pinchDist = g.d;
        pinchMid = { x: g.mx, y: g.my };
      }
    };

    // Host-driven zoom (toolbar % menu). Zooms around viewport centre so the
    // visible midpoint stays fixed — matching the host's iframe-zoom feel.
    const onHostMsg = (e) => {
      const d = e.data;
      if (d && d.type === 'miaoda:canvas:set-zoom' && typeof d.scale === 'number') {
        const r = vp.getBoundingClientRect();
        zoomAt(r.left + r.width / 2, r.top + r.height / 2, d.scale / tf.current.scale);
      } else if (d && d.type === 'miaoda:canvas:probe') {
        // Host's [readyGen] reset asks whether a canvas is present; it
        // fires on the iframe's native 'load', which for canvases with
        // images/fonts is after our mount-time announce, so re-announce.
        // Clear the pan-tick guard so apply() re-posts the current scale
        // even if it's unchanged — the host just reset dcScale to 1.
        window.parent.postMessage({ type: 'miaoda:canvas:present' }, '*');
        lastPostedScale.current = undefined;
        apply();
      }
    };
    window.addEventListener('message', onHostMsg);
    // Announce canvas mode so the host toolbar proxies its % control here
    // instead of scaling the iframe element (which would just shrink the
    // viewport window of an infinite canvas). The apply() that follows emits
    // the initial miaoda:canvas:zoom so the toolbar % is correct before first pinch.
    // lastPostedScale reset mirrors the miaoda:canvas:probe handler: the layout
    // effect's restore-path apply() may already have posted the restored
    // scale (before miaoda:canvas:present), so clear the guard to re-post it in order.
    window.parent.postMessage({ type: 'miaoda:canvas:present' }, '*');
    lastPostedScale.current = undefined;
    apply();

    // Mobile swaps the desktop pointer mapping (bg-only pan) for the touch
    // state machine; wheel/gesture listeners stay on both (harmless without
    // the hardware, and a bluetooth mouse on Android still pans via wheel).
    const pd = dcIsMobile ? onTouchDown : onPointerDown;
    const pm = dcIsMobile ? onTouchMove : onPointerMove;
    const pu = dcIsMobile ? onTouchUp : onPointerUp;
    vp.addEventListener('wheel', onWheel, { passive: false });
    vp.addEventListener('gesturestart', onGestureStart, { passive: false });
    vp.addEventListener('gesturechange', onGestureChange, { passive: false });
    vp.addEventListener('gestureend', onGestureEnd, { passive: false });
    vp.addEventListener('pointerdown', pd);
    vp.addEventListener('pointermove', pm);
    vp.addEventListener('pointerup', pu);
    vp.addEventListener('pointercancel', pu);
    return () => {
      window.removeEventListener('message', onHostMsg);
      vp.removeEventListener('wheel', onWheel);
      vp.removeEventListener('gesturestart', onGestureStart);
      vp.removeEventListener('gesturechange', onGestureChange);
      vp.removeEventListener('gestureend', onGestureEnd);
      vp.removeEventListener('pointerdown', pd);
      vp.removeEventListener('pointermove', pm);
      vp.removeEventListener('pointerup', pu);
      vp.removeEventListener('pointercancel', pu);
    };
  }, [apply, minScale, maxScale]);

  return (
    <div
      ref={vpRef}
      className={dcIsMobile ? 'design-canvas dc-mobile' : 'design-canvas'}
      style={{
        height: '100vh', width: '100vw',
        background: DC.bg,
        overflow: 'hidden',
        overscrollBehavior: 'none',
        touchAction: 'none',
        position: 'relative',
        fontFamily: DC.font,
        boxSizing: 'border-box',
        ...style,
      }}
    >
      <div
        ref={worldRef}
        style={{
          position: 'absolute', top: 0, left: 0,
          transformOrigin: '0 0',
          willChange: 'transform',
          width: 'max-content', minWidth: '100%',
          minHeight: '100%',
          padding: '60px 0 80px',
        }}
      >
        {children}
      </div>
      {/* Mobile zooms by pinch — the control is not rendered at all (host
          set-zoom messages still work; the protocol is independent of it). */}
      {!dcIsMobile && (
        <DCZoomControl
          scalePct={scalePct}
          minScale={minScale}
          maxScale={maxScale}
          onZoomTo={(pct) => zoomCenteredRef.current?.(pct / 100)}
        />
      )}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// DCSection — editable title + h-row of artboards in persisted order
// ─────────────────────────────────────────────────────────────
function DCSection({ id, title, subtitle, children, gap = 48 }) {
  const ctx = React.useContext(DCCtx);
  const sid = id ?? title;
  const all = React.Children.toArray(dcFlatten(children));
  const artboards = all.filter((c) => c && c.type === DCArtboard);
  const rest = all.filter((c) => !(c && c.type === DCArtboard));
  const sec = (ctx && sid && ctx.section(sid)) || {};
  // Must match DesignCanvas's srcKey computation exactly (it filters falsy
  // IDs), or onDelete persists a srcKey that DesignCanvas never recognizes.
  const allIds = artboards.map((a) => a.props.id ?? a.props.label).filter(Boolean);
  const srcKey = allIds.join('\x1f');
  const hidden = sec.srcKey === srcKey ? (sec.hidden || []) : [];
  const srcOrder = allIds.filter((k) => !hidden.includes(k));

  const order = React.useMemo(() => {
    const kept = (sec.order || []).filter((k) => srcOrder.includes(k));
    return [...kept, ...srcOrder.filter((k) => !kept.includes(k))];
  }, [sec.order, srcOrder.join('|')]);

  const byId = Object.fromEntries(artboards.map((a) => [a.props.id ?? a.props.label, a]));

  // marginBottom counter-scales so the on-screen gap between sections stays
  // constant — otherwise at low zoom the (world-space) gap collapses while
  // the screen-constant sectionhead below it doesn't, and the title reads as
  // belonging to the section above. paddingBottom below is just enough for
  // the 24px artboard-header (abs-positioned above each card) plus ~8px, so
  // the title sits tight against its own row at every zoom.
  return (
    <div data-dc-section={sid}
      style={{ marginBottom: 'calc(80px * var(--dc-inv-zoom, 1))', position: 'relative' }}>
      <div style={{ padding: '0 60px' }}>
        <div className="dc-sectionhead" style={{ paddingBottom: 36 }}>
          <DCEditable tag="div" value={sec.title ?? title}
            onChange={(v) => ctx && sid && ctx.patchSection(sid, { title: v })}
            style={{ fontSize: 20, fontWeight: 600, color: DC.title, letterSpacing: -0.4, marginBottom: 2, display: 'inline-block', lineHeight: '28px' }} />
          {subtitle && <div style={{ fontSize: 14, color: DC.subtitle, lineHeight: '22px' }}>{subtitle}</div>}
        </div>
      </div>
      <div style={{ display: 'flex', gap, padding: '0 60px', alignItems: 'flex-start', width: 'max-content' }}>
        {order.map((k) => (
          <DCArtboardFrame key={k} sectionId={sid} artboard={byId[k]} order={order}
            label={(sec.labels || {})[k] ?? byId[k].props.label}
            onRename={(v) => ctx && ctx.patchSection(sid, (x) => ({ labels: { ...x.labels, [k]: v } }))}
            onReorder={(next) => ctx && ctx.patchSection(sid, { order: next })}
            onDelete={() => ctx && ctx.patchSection(sid, (x) => ({
              hidden: [...(x.srcKey === srcKey ? (x.hidden || []) : []), k],
              srcKey,
            }))}
            onFocus={() => ctx && ctx.setFocus(`${sid}/${k}`)} />
        ))}
      </div>
      {rest}
    </div>
  );
}

// DCArtboard — marker; rendered by DCArtboardFrame via DCSection.
function DCArtboard() { return null; }

// Per-artboard export (kind: 'png' | 'html'). Both paths share the same
// self-contained clone: computed styles baked in, @font-face / <img> /
// inline-style background-image urls inlined as data URIs. PNG wraps the
// clone in foreignObject→canvas at 3× the artboard's natural width×height
// (same pipeline the host uses for page captures); HTML wraps it in a
// minimal standalone document. Both are independent of viewport zoom.
async function dcExport(node, w, h, name, kind) {
  try { await document.fonts.ready; } catch {}
  const toDataURL = (url) => fetch(url).then((r) => r.blob()).then((b) => new Promise((res) => {
    const fr = new FileReader(); fr.onload = () => res(fr.result); fr.onerror = () => res(url); fr.readAsDataURL(b);
  })).catch(() => url);

  // Collect @font-face rules. ss.cssRules throws SecurityError on
  // cross-origin sheets (e.g. fonts.googleapis.com) — in that case fetch
  // the CSS text directly (those endpoints send ACAO:*) and regex-extract
  // the blocks. @import and @media/@supports are walked so nested
  // @font-face rules aren't missed.
  const fontRules = [], pending = [], seen = new Set();
  const scrapeCss = (href) => {
    if (seen.has(href)) return; seen.add(href);
    pending.push(fetch(href).then((r) => r.text()).then((css) => {
      for (const m of css.match(/@font-face\s*{[^}]*}/g) || []) fontRules.push({ css: m, base: href });
      for (const m of css.matchAll(/@import\s+(?:url\()?['"]?([^'")\s;]+)/g))
        scrapeCss(new URL(m[1], href).href);
    }).catch(() => {}));
  };
  const walk = (rules, base) => {
    for (const r of rules) {
      if (r.type === CSSRule.FONT_FACE_RULE) fontRules.push({ css: r.cssText, base });
      else if (r.type === CSSRule.IMPORT_RULE && r.styleSheet) {
        const ibase = r.styleSheet.href || base;
        try { walk(r.styleSheet.cssRules, ibase); } catch { scrapeCss(ibase); }
      } else if (r.cssRules) walk(r.cssRules, base);
    }
  };
  for (const ss of document.styleSheets) {
    const base = ss.href || location.href;
    try { walk(ss.cssRules, base); } catch { if (ss.href) scrapeCss(ss.href); }
  }
  while (pending.length) await pending.shift();
  const fontCss = (await Promise.all(fontRules.map(async (rule) => {
    let out = rule.css, m; const re = /url\((['"]?)([^'")]+)\1\)/g;
    while ((m = re.exec(rule.css))) {
      if (m[2].indexOf('data:') === 0) continue;
      let abs; try { abs = new URL(m[2], rule.base).href; } catch { continue; }
      out = out.split(m[0]).join('url("' + await toDataURL(abs) + '")');
    }
    return out;
  }))).join('\n');

  const cloneStyled = (src) => {
    if (src.nodeType === 8 || (src.nodeType === 1 && src.tagName === 'SCRIPT')) return document.createTextNode('');
    const dst = src.cloneNode(false);
    if (src.nodeType === 1) {
      const cs = getComputedStyle(src); let txt = '';
      for (let i = 0; i < cs.length; i++) txt += cs[i] + ':' + cs.getPropertyValue(cs[i]) + ';';
      dst.setAttribute('style', txt + 'animation:none;transition:none;');
      if (src.tagName === 'CANVAS') try { const im = document.createElement('img'); im.src = src.toDataURL(); im.setAttribute('style', txt); return im; } catch {}
    }
    for (let c = src.firstChild; c; c = c.nextSibling) dst.appendChild(cloneStyled(c));
    return dst;
  };
  const clone = cloneStyled(node);
  clone.setAttribute('xmlns', 'http://www.w3.org/1999/xhtml');
  // Drop the card's own shadow/radius so the export is a flush w×h rect;
  // the artboard's own background (if any) is already in the computed style.
  clone.style.boxShadow = 'none'; clone.style.borderRadius = '0';

  const jobs = [];
  clone.querySelectorAll('img').forEach((el) => {
    const s = el.getAttribute('src');
    if (s && s.indexOf('data:') !== 0) jobs.push(toDataURL(el.src).then((d) => el.setAttribute('src', d)));
  });
  [clone, ...clone.querySelectorAll('*')].forEach((el) => {
    const bg = el.style.backgroundImage; if (!bg) return;
    let m; const re = /url\(["']?([^"')]+)["']?\)/g;
    while ((m = re.exec(bg))) {
      const tok = m[0], url = m[1];
      if (url.indexOf('data:') === 0) continue;
      jobs.push(toDataURL(url).then((d) => { el.style.backgroundImage = el.style.backgroundImage.split(tok).join('url("' + d + '")'); }));
    }
  });
  await Promise.all(jobs);

  const xml = new XMLSerializer().serializeToString(clone);
  const save = (blob, ext) => {
    if (!blob) return;
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob); a.download = name + '.' + ext; a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  };

  if (kind === 'html') {
    const html = '<!doctype html><html><head><meta charset="utf-8"><title>' + name + '</title>' +
      (fontCss ? '<style>' + fontCss + '</style>' : '') +
      '</head><body style="margin:0">' + xml + '</body></html>';
    return save(new Blob([html], { type: 'text/html' }), 'html');
  }

  // PNG: the SVG's own width/height must be the output resolution — an
  // <img>-loaded SVG rasterizes at its intrinsic size, so sizing it at 1×
  // and ctx.scale()-ing up would just upscale a 1× bitmap. viewBox maps the
  // w×h foreignObject onto the px·w × px·h SVG canvas so the browser renders
  // the HTML at full resolution.
  const px = 3;
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="' + w * px + '" height="' + h * px +
    '" viewBox="0 0 ' + w + ' ' + h + '"><foreignObject width="' + w + '" height="' + h + '">' +
    (fontCss ? '<style><![CDATA[' + fontCss + ']]></style>' : '') + xml + '</foreignObject></svg>';
  const img = new Image();
  await new Promise((res, rej) => {
    img.onload = res; img.onerror = () => rej(new Error('svg load failed'));
    img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
  });
  const cv = document.createElement('canvas');
  cv.width = w * px; cv.height = h * px;
  cv.getContext('2d').drawImage(img, 0, 0);
  cv.toBlob((blob) => save(blob, 'png'), 'image/png');
}

function DCArtboardFrame({ sectionId, artboard, label, order, onRename, onReorder, onFocus, onDelete }) {
  // Read height off the raw props (not a destructure default) so an *absent*
  // height can opt into auto-grow while an explicit height={N} — even 480 —
  // still means a fixed, content-clipping frame (thumbnail semantics).
  const { id: rawId, label: rawLabel, width = 260, chromeless = false, children, style = {} } = artboard.props;
  const hasHeight = artboard.props.height != null;
  const height = hasHeight ? artboard.props.height : undefined;
  const id = rawId ?? rawLabel;
  const ref = React.useRef(null);
  const cardRef = React.useRef(null);
  const menuRef = React.useRef(null);
  const [menuOpen, setMenuOpen] = React.useState(false);
  const [confirming, setConfirming] = React.useState(false);

  // ⋯ menu: close on any outside pointerdown. Two-click delete lives inside
  // the menu — first click arms the row, second commits; closing disarms.
  React.useEffect(() => {
    if (!menuOpen) { setConfirming(false); return; }
    const off = (e) => { if (!menuRef.current || !menuRef.current.contains(e.target)) setMenuOpen(false); };
    document.addEventListener('pointerdown', off, true);
    return () => document.removeEventListener('pointerdown', off, true);
  }, [menuOpen]);

  const doExport = (kind) => {
    setMenuOpen(false);
    if (!cardRef.current) return;
    // Unicode-aware sanitize: keep letters (incl. CJK/accented), digits,
    // whitespace, dot/underscore/hyphen; collapse everything else to '_'.
    // ASCII-only \w would strip Chinese labels down to a bare '_'.
    const name = String(label || id || 'artboard').replace(/[^\p{L}\p{N}\s._-]+/gu, '_');
    const ew = cardRef.current.offsetWidth || width;
    const eh = cardRef.current.offsetHeight || height;
    dcExport(cardRef.current, ew, eh, name, kind)
      .catch((e) => console.error('[design-canvas] export failed:', e));
  };

  // Live drag-reorder: dragged card sticks to cursor; siblings slide into
  // their would-be slots in real time via transforms. DOM order only
  // changes on drop.
  const onGripDown = (e) => {
    e.preventDefault(); e.stopPropagation();
    const me = ref.current;
    // translateX is applied in local (pre-scale) space but pointer deltas and
    // getBoundingClientRect().left are screen-space — divide by the viewport's
    // current scale so the dragged card tracks the cursor at any zoom level.
    const scale = me.getBoundingClientRect().width / me.offsetWidth || 1;
    const peers = Array.from(document.querySelectorAll(`[data-dc-section="${sectionId}"] [data-dc-slot]`));
    const homes = peers.map((el) => ({ el, id: el.dataset.dcSlot, x: el.getBoundingClientRect().left }));
    const slotXs = homes.map((h) => h.x);
    const startIdx = order.indexOf(id);
    const startX = e.clientX;
    let liveOrder = order.slice();
    me.classList.add('dc-dragging');

    const layout = () => {
      for (const h of homes) {
        if (h.id === id) continue;
        const slot = liveOrder.indexOf(h.id);
        h.el.style.transform = `translateX(${(slotXs[slot] - h.x) / scale}px)`;
      }
    };

    const move = (ev) => {
      const dx = ev.clientX - startX;
      me.style.transform = `translateX(${dx / scale}px)`;
      const cur = homes[startIdx].x + dx;
      let nearest = 0, best = Infinity;
      for (let i = 0; i < slotXs.length; i++) {
        const d = Math.abs(slotXs[i] - cur);
        if (d < best) { best = d; nearest = i; }
      }
      if (liveOrder.indexOf(id) !== nearest) {
        liveOrder = order.filter((k) => k !== id);
        liveOrder.splice(nearest, 0, id);
        layout();
      }
    };

    const up = () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
      const finalSlot = liveOrder.indexOf(id);
      me.classList.remove('dc-dragging');
      me.style.transform = `translateX(${(slotXs[finalSlot] - homes[startIdx].x) / scale}px)`;
      // After the settle transition, kill transitions + clear transforms +
      // commit the reorder in the same frame so there's no visual snap-back.
      setTimeout(() => {
        for (const h of homes) { h.el.style.transition = 'none'; h.el.style.transform = ''; }
        if (liveOrder.join('|') !== order.join('|')) onReorder(liveOrder);
        requestAnimationFrame(() => requestAnimationFrame(() => {
          for (const h of homes) h.el.style.transition = '';
        }));
      }, 180);
    };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
  };

  return (
    <div ref={ref} data-dc-slot={id} style={{ position: 'relative', flexShrink: 0 }}>
      {/* Desktop stops pointerdown so header interactions never start a
          bg-pan; mobile pans FROM chrome too (slop arbitration keeps label
          taps working), so the event must bubble to the viewport there. */}
      <div className="dc-header" data-miaoda-chrome="" style={{ color: DC.label }} onPointerDown={dcIsMobile ? undefined : (e) => e.stopPropagation()}>
        <div className="dc-labelrow">
          <div className="dc-grip" onPointerDown={dcIsMobile ? undefined : onGripDown} title="Drag to reorder">
            <svg width="9" height="13" viewBox="0 0 9 13" fill="currentColor"><circle cx="2" cy="2" r="1.1"/><circle cx="7" cy="2" r="1.1"/><circle cx="2" cy="6.5" r="1.1"/><circle cx="7" cy="6.5" r="1.1"/><circle cx="2" cy="11" r="1.1"/><circle cx="7" cy="11" r="1.1"/></svg>
          </div>
          <div className="dc-labeltext" onClick={onFocus} title="Click to focus">
            <DCEditable value={label} onChange={onRename} onClick={(e) => e.stopPropagation()}
              style={{ fontSize: 14, fontWeight: 300, color: DC.label, lineHeight: '22px' }} />
          </div>
        </div>
        <div className="dc-btns">
          <div ref={menuRef} style={{ position: 'relative' }}>
            <button className="dc-kebab" title="More" onClick={() => setMenuOpen((o) => !o)}>
              <svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor"><circle cx="2.5" cy="6" r="1.1"/><circle cx="6" cy="6" r="1.1"/><circle cx="9.5" cy="6" r="1.1"/></svg>
            </button>
            {menuOpen && (
              <div className="dc-menu" onPointerDown={(e) => e.stopPropagation()}>
                <button onClick={() => doExport('png')}>下载 PNG</button>
                <button onClick={() => doExport('html')}>下载 HTML</button>
                <button className="dc-danger"
                  onClick={() => { if (confirming) { setMenuOpen(false); onDelete(); } else setConfirming(true); }}>
                  {confirming ? '确认删除' : '删除'}
                </button>
              </div>
            )}
          </div>
          <button className="dc-expand" onClick={onFocus} title="Focus">
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M7 1h4v4M5 11H1V7M11 1L7.5 4.5M1 11l3.5-3.5"/></svg>
          </button>
        </div>
      </div>
      <div ref={cardRef} className={"dc-card" + (chromeless ? ' chromeless' : '')}
        style={{ ...(chromeless ? {} : {
          width,
          // explicit height → fixed frame (overflow:hidden clips a tall page);
          // no height → the box grows to its content height. overflow:hidden
          // stays either way so the rounded corners clip and content wider than
          // the frame is trimmed to `width`; with auto height nothing is clipped
          // vertically because the box is exactly the content height.
          ...(hasHeight ? { height } : {}),
          borderRadius: 4, border: '1px solid #DEE0E3', overflow: 'hidden', background: '#fff',
        }), ...style }}>
        {children || <div style={{ minHeight: 160, height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#bbb', fontSize: 13, fontFamily: DC.font }}>{id}</div>}
      </div>
    </div>
  );
}

// Inline rename — commits on blur or Enter.
function DCEditable({ value, onChange, style, tag = 'span', onClick }) {
  const T = tag;
  // Mobile is browse-only: render plain text — no contentEditable (a tap
  // would pop the soft keyboard), no stopPropagation (pans start on labels
  // too), and no onClick swallow (desktop uses it to keep an edit-click from
  // bubbling; on mobile the tap should bubble so a label tap opens focus).
  if (dcIsMobile) {
    return <T className="dc-editable" style={style}>{value}</T>;
  }
  return (
    <T className="dc-editable" contentEditable suppressContentEditableWarning
      onClick={onClick}
      onPointerDown={(e) => e.stopPropagation()}
      onBlur={(e) => onChange && onChange(e.currentTarget.textContent)}
      onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); e.currentTarget.blur(); } }}
      style={style}>{value}</T>
  );
}

// ─────────────────────────────────────────────────────────────
// Focus mode — overlay one artboard; ←/→ within section, ↑/↓ across
// sections, Esc or backdrop click to exit.
// ─────────────────────────────────────────────────────────────
function DCFocusOverlay({ entry, sectionMeta, sectionOrder }) {
  const ctx = React.useContext(DCCtx);
  const { sectionId, artboard } = entry;
  const meta = sectionMeta[sectionId];
  const peers = meta.slotIds;
  const aid = artboard.props.id ?? artboard.props.label;
  const idx = peers.indexOf(aid);
  const secIdx = sectionOrder.indexOf(sectionId);

  const go = (d) => { const n = peers[(idx + d + peers.length) % peers.length]; if (n) ctx.setFocus(`${sectionId}/${n}`); };
  const goSection = (d) => {
    // Sections whose artboards are all deleted have slotIds:[] — step past
    // them to the next non-empty section so ↑/↓ doesn't dead-end.
    const n = sectionOrder.length;
    for (let i = 1; i < n; i++) {
      const ns = sectionOrder[(((secIdx + d * i) % n) + n) % n];
      const first = sectionMeta[ns] && sectionMeta[ns].slotIds[0];
      if (first) { ctx.setFocus(`${ns}/${first}`); return; }
    }
  };

  React.useEffect(() => {
    const k = (e) => {
      if (e.key === 'ArrowLeft') { e.preventDefault(); go(-1); }
      if (e.key === 'ArrowRight') { e.preventDefault(); go(1); }
      if (e.key === 'ArrowUp') { e.preventDefault(); goSection(-1); }
      if (e.key === 'ArrowDown') { e.preventDefault(); goSection(1); }
    };
    document.addEventListener('keydown', k);
    return () => document.removeEventListener('keydown', k);
  });

  const { width = 260, chromeless = false, children } = artboard.props;
  const hasHeight = artboard.props.height != null;
  const height = hasHeight ? artboard.props.height : undefined;
  // A fixed-frame card (non-chromeless WITH an explicit height) uses width/
  // height directly. Otherwise the card has NO fixed height and its rendered
  // size describes the content, not a frame — either a chromeless artboard
  // (device frame / auto layout, both axes intrinsic) or a non-chromeless
  // artboard that omitted height and grows to its content (frame `width`, auto
  // height). Fitting such content to a default height would wrap it in an
  // undersized clip box and magnify the clipped slice — so measure the real
  // rendered size first and fit-scale to that, mirroring the canvas card.
  const autoSize = chromeless || !hasHeight;
  const measureRef = React.useRef(null);
  const [nat, setNat] = React.useState(null); // { aid, w, h } measured natural size
  React.useLayoutEffect(() => {
    if (!autoSize) return;
    const el = measureRef.current;
    if (el) setNat({ aid, w: el.offsetWidth, h: el.offsetHeight });
  }, [aid, autoSize]);
  const useMeasured = autoSize && nat && nat.aid === aid && nat.w > 0;
  const needsMeasure = autoSize && !useMeasured;
  const cw = useMeasured ? nat.w : width;
  const ch = useMeasured ? nat.h : (height ?? 480);

  const [vp, setVp] = React.useState({ w: window.innerWidth, h: window.innerHeight });
  React.useEffect(() => { const r = () => setVp({ w: window.innerWidth, h: window.innerHeight }); window.addEventListener('resize', r); return () => window.removeEventListener('resize', r); }, []);
  // Fill the viewport leaving a safe margin — 70px left/right, 32px top/bottom.
  // Take the smaller of the width-fit / height-fit ratios so the whole artboard
  // stays fully visible; no upper cap, so small frames still fill the screen.
  const MARGIN_X = 70;
  const MARGIN_Y = 32;
  const scale = Math.max(0.05, Math.min((vp.w - MARGIN_X * 2) / cw, (vp.h - MARGIN_Y * 2) / ch));

  // Chrome (arrows / close / dots) is hidden until the cursor sits on the
  // dark backdrop (anywhere that isn't the interactive content). Hiding also
  // sets pointer-events:none so hidden controls are click-through and never
  // block content interaction or a backdrop-click exit.
  const [chromeVisible, setChromeVisible] = React.useState(false);
  const contentRef = React.useRef(null);
  const chromeStyle = { opacity: chromeVisible ? 1 : 0, pointerEvents: chromeVisible ? 'auto' : 'none' };
  const Arrow = ({ dir, onClick }) => (
    <button onClick={(e) => { e.stopPropagation(); onClick(); }}
      style={{ position: 'absolute', top: '50%', [dir]: 20, transform: 'translateY(-50%)',
        border: 'none', background: 'rgba(255,255,255,.08)', color: 'rgb(255,255,255)',
        width: 36, height: 36, borderRadius: 22, fontSize: 16, cursor: 'pointer',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        transition: 'background .15s, opacity .18s ease', ...chromeStyle }}
      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(255,255,255,.18)')}
      onMouseLeave={(e) => (e.currentTarget.style.background = 'rgba(255,255,255,.08)')}>
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d={dir === 'left' ? 'M10.8619 1.52925C11.1223 1.7896 11.1223 2.21171 10.8619 2.47206L5.33333 8.00065L10.8619 13.5292C11.1223 13.7896 11.1223 14.2117 10.8619 14.4721C10.6016 14.7324 10.1795 14.7324 9.91912 14.4721L4.39052 8.94346C3.86983 8.42276 3.86982 7.57854 4.39052 7.05784L9.91912 1.52925C10.1795 1.2689 10.6016 1.2689 10.8619 1.52925Z' : 'M5.13765 14.4721C4.8773 14.2117 4.8773 13.7896 5.13765 13.5292L10.6662 8.00065L5.13765 2.47206C4.8773 2.21171 4.8773 1.7896 5.13764 1.52925C5.39799 1.2689 5.8201 1.2689 6.08045 1.52925L11.609 7.05784C12.1297 7.57854 12.1298 8.42276 11.6091 8.94346L6.08045 14.4721C5.82011 14.7324 5.398 14.7324 5.13765 14.4721Z'} fill="currentColor" /></svg>
    </button>
  );

  // Portal to body so position:fixed is the real viewport regardless of any
  // transform on DesignCanvas's ancestors (including the canvas zoom itself).
  return ReactDOM.createPortal(
    <div onClick={() => ctx.setFocus(null)}
      onMouseMove={(e) => setChromeVisible(!(contentRef.current && contentRef.current.contains(e.target)))}
      onMouseLeave={() => setChromeVisible(false)}
      onWheel={(e) => { if (!(contentRef.current && contentRef.current.contains(e.target))) e.preventDefault(); }}
      style={{ position: 'fixed', inset: 0, zIndex: 100, background: 'rgba(0,0,0,.8)', backdropFilter: 'blur(4px)',
        fontFamily: DC.font, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>

      {/* content fills the viewport to its safe margin (70px left/right, 32px
          top/bottom), centered — interactive & scrollable. Only the content
          stops propagation, so any backdrop click (the dark margin around it)
          exits focus. */}
      <div ref={contentRef} className="dc-focus-content" onClick={(e) => e.stopPropagation()}
        style={{ width: cw * scale, height: ch * scale, position: 'relative',
          // hidden until the auto-size measure pass below reports the real size
          visibility: needsMeasure ? 'hidden' : 'visible' }}>
        {/* Auto-size cards render a hidden measure pass first, then re-render
            scaled. Chromeless measures both axes (inline-block → intrinsic
            size); a non-chromeless auto card keeps its frame `width` and lets
            height run natural. Card chrome (bg / radius / shadow / overflow:auto)
            is added only for a non-chromeless box — matching the canvas card —
            giving it a scroll viewport for anything it still clips; scrollbars
            are hidden by the .dc-focus-content CSS. A chromeless box carries
            none of it, so its content owns its look. */}
        <div ref={measureRef} style={needsMeasure
          ? (chromeless ? { display: 'inline-block' } : { width })
          : { width: cw, height: ch, transform: `scale(${scale})`, transformOrigin: 'top left',
            ...(chromeless ? {} : { background: '#fff', borderRadius: 2, overflow: 'auto', boxShadow: '0 20px 80px rgba(0,0,0,.4)' }) }}>
          {children || <div style={{ minHeight: 160, height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#bbb' }}>{aid}</div>}
        </div>
      </div>

      {/* close — top-right, revealed on backdrop hover */}
      <button onClick={() => ctx.setFocus(null)}
        onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(255,255,255,.18)')}
        onMouseLeave={(e) => (e.currentTarget.style.background = 'rgba(255,255,255,.08)')}
        style={{ position: 'absolute', top: 20, right: 24, border: 'none', background: 'rgba(255,255,255,.08)', color: 'rgb(255,255,255)',
          width: 36, height: 36, borderRadius: 22, fontSize: 16, cursor: 'pointer', lineHeight: 1,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          transition: 'background .12s, opacity .18s ease', ...chromeStyle }}>
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M13.4713 13.4722C13.7301 13.2134 13.732 12.7957 13.4732 12.5369L8.93693 8.00064L13.4731 3.46444C13.7319 3.20562 13.7301 2.78788 13.4713 2.52907C13.2125 2.27026 12.7947 2.26838 12.5359 2.5272C12.2771 2.78601 7.99969 7.0634 7.99969 7.0634L3.46345 2.52716C3.20464 2.26835 2.7869 2.27022 2.52809 2.52903C2.26928 2.78784 2.2674 3.20558 2.52621 3.4644L7.06246 8.00064L2.52618 12.5369C2.26737 12.7957 2.26924 13.2135 2.52806 13.4723C2.78687 13.7311 3.20461 13.733 3.46342 13.4742L7.99969 8.93788L12.5359 13.4741C12.7947 13.7329 13.2125 13.731 13.4713 13.4722Z" fill="currentColor"/>
          </svg>
        </button>

      <Arrow dir="left" onClick={() => go(-1)} />
      <Arrow dir="right" onClick={() => go(1)} />

      {/* dots — revealed on backdrop hover */}
      <div onClick={(e) => e.stopPropagation()}
        style={{ position: 'absolute', bottom: 13, left: '50%', transform: 'translateX(-50%)', display: 'flex', gap: 8,
          transition: 'opacity .18s ease', ...chromeStyle }}>
        {peers.map((p, i) => (
          <button key={p} onClick={() => ctx.setFocus(`${sectionId}/${p}`)}
            style={{ border: 'none', padding: 0, cursor: 'pointer', width: 6, height: 6, borderRadius: 3,
              background: i === idx ? '#fff' : 'rgba(255,255,255,.3)' }} />
        ))}
      </div>
    </div>,
    document.body,
  );
}

// ─────────────────────────────────────────────────────────────
// Post-it — absolute-positioned sticky note
// ─────────────────────────────────────────────────────────────
function DCPostIt({ children, top, left, right, bottom, rotate = -2, width = 180 }) {
  return (
    <div style={{
      position: 'absolute', top, left, right, bottom, width,
      background: DC.postitBg, padding: '14px 16px',
      fontFamily: '"Comic Sans MS", "Marker Felt", "Segoe Print", cursive',
      fontSize: 14, lineHeight: 1.4, color: DC.postitText,
      boxShadow: '0 2px 8px rgba(0,0,0,0.12), 0 1px 2px rgba(0,0,0,0.08)',
      transform: `rotate(${rotate}deg)`,
      zIndex: 5,
    }}>{children}</div>
  );
}

Object.assign(window, { DesignCanvas, DCSection, DCArtboard, DCPostIt });

