// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

/* BEGIN USAGE */
/**
 * <deck-stage> — reusable web component for HTML decks.
 *
 * Handles:
 *  (a) speaker notes — reads <script type="application/json" id="speaker-notes">
 *      (a JSON array of per-slide note strings), shows the current slide's
 *      note in an editable bottom dock (edit mode only; drag its grip to
 *      resize, height persisted to localStorage), posts miaoda:deck:slide-changed
 *      to the parent on nav, and posts miaoda:deck:notes-changed (the full
 *      array) when a note is edited so the host can persist it. The dock is
 *      hidden while presenting / in preview / on narrow / noscale / no-rail.
 *  (b) keyboard navigation — ←/→, PgUp/PgDn, Space, Home/End, number keys.
 *      On touch devices, tapping the left/right half of the stage goes
 *      prev/next (desktop / presenting mode; mobile browse mode scrolls
 *      instead — see (g)) — taps on links, buttons and other interactive
 *      slide content are left alone.
 *  (c) press R to reset to slide 0.
 *  (d) auto-scaling — inner canvas is a fixed design size (default 1920×1080)
 *      scaled with `transform: scale()` to fit the viewport, letterboxed.
 *      Set the `noscale` attribute to render at authored size (1:1) — the
 *      PPTX exporter sets this so its DOM capture sees unscaled geometry.
 *  (e) print — `@media print` lays every slide out as its own page at the
 *      design size, so the browser's Print → Save as PDF produces a clean
 *      one-page-per-slide PDF with no extra setup.
 *  (f) thumbnail rail — resizable left-hand column of per-slide thumbnails
 *      (static clones). Click to navigate; ↑/↓ with a thumbnail focused to
 *      step between slides; drag to reorder; right-click for
 *      Skip / Move up / Move down / Delete. Drag the rail's right edge
 *      to resize; width persists to
 *      localStorage. Skipped slides carry `data-deck-skip`, are dimmed in
 *      the rail, omitted from prev/next navigation, and hidden at print.
 *      The rail is suppressed in presenting mode, in the host's Preview
 *      mode (ViewerMode='none'), on `noscale`, on narrow viewports
 *      (≤640px), and via the `no-rail` attribute. Rail mutations dispatch
 *      a `deckchange`
 *      CustomEvent on the element: detail = {action, from, to, slide}.
 *  (g) mobile — on mobile UA (mirrors the host platform's isMobile()
 *      semantics: UA regex + iPad maxTouchPoints, never viewport width),
 *      browse mode renders slides as a vertical card stream (12px margin,
 *      8px gap, 8px radius) with native scrolling; a per-card page badge
 *      fades in while scrolling (16|20×12, ink 20% bg, fixed 8px digits)
 *      and fades out 2s after scrolling stops. Presenting mode
 *      (miaoda:deck:presenting) rotates the stage 90° (CSS pseudo-
 *      landscape, no orientation.lock): tap left/right third to navigate,
 *      tap center to toggle the controls overlay (back button +
 *      filmstrip; auto-hides after 3s). The back button exits presenting
 *      and posts miaoda:deck:presenting-dismissed to the parent.
 *      Desktop behaviour is unchanged.
 *
 * Slides are HIDDEN, not unmounted. Non-active slides stay in the DOM with
 * `visibility: hidden` + `opacity: 0`, so their state (videos, iframes,
 * form inputs, React trees) is preserved across navigation.
 *
 * Lifecycle event — the component dispatches a `slidechange` CustomEvent on
 * itself whenever the active slide changes (including the initial mount).
 * The event bubbles and composes out of shadow DOM, so you can listen on
 * the <deck-stage> element or on document:
 *
 *   document.querySelector('deck-stage').addEventListener('slidechange', (e) => {
 *     e.detail.index         // new 0-based index
 *     e.detail.previousIndex // previous index, or -1 on init
 *     e.detail.total         // total slide count
 *     e.detail.slide         // the new active slide element
 *     e.detail.previousSlide // the prior slide element, or null on init
 *     e.detail.reason        // 'init' | 'keyboard' | 'click' | 'tap' | 'api'
 *   });
 *
 * Persistence: none at the deck level. The host app keeps the current slide
 * in its own URL (?slide=) and re-delivers it via location.hash on load, so a
 * bare load with no hash always starts at slide 1.
 *
 * Usage:
 *   <style>deck-stage:not(:defined){visibility:hidden}</style>
 *   <deck-stage width="1920" height="1080">
 *     <section data-label="Title">...</section>
 *     <section data-label="Agenda">...</section>
 *   </deck-stage>
 *   <script src="deck-stage.js"></script>
 *
 * The :not(:defined) rule prevents a flash of the first slide at its
 * authored styles before this script runs and attaches the shadow root.
 *
 * Slides are the direct element children of <deck-stage>. Each slide is
 * automatically tagged with:
 *   - data-screen-label="NN Label"   (1-indexed, for comment flow)
 *   - data-miaoda-validate="no_overflowing_text,no_overlapping_text,slide_sized_text"
 *
 * Speaker notes stay in sync because the component posts miaoda:deck:slide-changed
 * to the parent — just include the #speaker-notes script tag if asked for notes.
 *
 * Authoring guidance:
 *   - Write slide bodies as static HTML inside <deck-stage>, with sizing via
 *     CSS custom properties in a <style> block rather than JS constants.
 *     Static slide markup is what lets the user click a heading in edit mode
 *     and retype it directly; a slide rendered through <script type="text/babel">,
 *     React, or a loop over a JS array has to round-trip every tweak through a
 *     chat message instead. Reach for script-generated slides only when the
 *     content genuinely needs interactive behaviour static HTML can't express.
 *   - Do NOT set position/inset/width/height on the slide <section> elements —
 *     the component absolutely positions every slotted child for you.
 *   - Entrance animations: make the visible end-state the base style and
 *     animate *from* hidden. The thumbnail rail, print, and reduced-motion
 *     render each slide's static base state and never fire the animation, so
 *     a hidden base (opacity:0 on the rule itself) renders blank there while
 *     the live slide looks fine. Keep the hidden state only in the keyframes
 *     `from`, and gate the animation on [data-deck-active] and the motion
 *     query, e.g.:
 *       .fade-up { opacity: 1 }
 *       @media (prefers-reduced-motion:no-preference){ [data-deck-active] .fade-up{ animation: fadeUp .5s both } }
 *       @keyframes fadeUp { from{ opacity:0; transform:translateY(24px) } to{ opacity:1; transform:none } }
 *     Avoid infinite decorative loops on slide content.
 */
/* END USAGE */

(() => {
  if (typeof window === 'undefined') return;
  const DESIGN_W_DEFAULT = 1920;
  const DESIGN_H_DEFAULT = 1080;
  // Safe margin around the scaled slide in edit/preview (non-presenting) mode,
  // giving the canvas a floating "card" look with a drop-shadow.
  const DECK_MARGIN = 20;
  // TEMP: presenter-notes dock is hidden for now. Flip back to false to restore
  // the editable speaker-notes panel (all notes logic below stays intact).
  const NOTES_DISABLED = true;
  const VALIDATE_ATTR = 'no_overflowing_text,no_overlapping_text,slide_sized_text';
  const FINE_POINTER_MQ = matchMedia('(hover: hover) and (pointer: fine)');
  const NARROW_MQ = matchMedia('(max-width: 640px)');
  // Slide-authored controls that should keep a tap instead of it navigating.
  const INTERACTIVE_SEL = 'a[href], button, input, select, textarea, summary, label, video[controls], audio[controls], [role="button"], [onclick], [tabindex]:not([tabindex^="-"]), [contenteditable]:not([contenteditable="false" i])';

  // ── 移动端（复刻主仓 @apaas-ai/global-states isMobile() 的 UA 语义；
  //    跨域产物 iframe 无法 import 该包）。刻意不用视口断点：桌面工作台的
  //    预览 iframe 本身就窄，视口宽度会把桌面预览误判成移动端。
  //    mode=desktop 逃生阀只作用于壳层自身 location、不透传到本 iframe，不实现。
  const MOBILE_UA_RE = /(phone|pad|pod|iPhone|iPod|ios|iPad|Android|Mobile|BlackBerry|IEMobile|MQQBrowser|JUC|Fennec|wOSBrowser|BrowserNG|WebOS|Symbian|Windows Phone)/i;
  const isMobileUA = () =>
    MOBILE_UA_RE.test(navigator.userAgent) ||
    (/iPad|Tab|Tablet/i.test(navigator.userAgent) && !!navigator.maxTouchPoints && navigator.maxTouchPoints > 1);
  // 移动端视觉常量（Figma node 2965:32138 / 2965:32047 / 2965:34386 已确认）
  const MOB_MARGIN = 12;        // 竖排卡片页边距
  const MOB_GAP = 8;            // 卡片间距
  const MOB_RADIUS = 8;         // 卡片圆角
  const MOB_BADGE_IDLE_MS = 2000;  // 滚动停止 → 徽标淡出（UX 确认 2s）
  const MOB_UI_HIDE_MS = 3000;     // 播放控件层自动收起

  const pad2 = (n) => String(n).padStart(2, '0');

  // Label precedence: data-label → data-screen-label (number stripped) → first heading → "Slide".
  const getSlideLabel = (el) => {
    const explicit = el.getAttribute('data-label');
    if (explicit) return explicit;

    const existing = el.getAttribute('data-screen-label');
    if (existing) return existing.replace(/^\s*\d+\s*/, '').trim() || existing;

    const h = el.querySelector('h1, h2, h3, [data-title]');
    const t = h && (h.textContent || '').trim().slice(0, 40);
    if (t) return t;

    return 'Slide';
  };

  const stylesheet = `
    :host {
      position: fixed;
      inset: 0;
      display: block;
      background: #F8F9FA;
      font-family: -apple-system, BlinkMacSystemFont, "Helvetica Neue", Helvetica, Arial, sans-serif;
      overflow: hidden;
      -webkit-tap-highlight-color: transparent;
    }
    /* connectedCallback holds this until document.fonts.ready (capped 2s) so
     * the first visible paint has the deck's real typography + final rail
     * layout. opacity (not visibility) so the active slide can't un-hide
     * itself via the ::slotted([data-deck-active]) visibility:visible rule.
     * Only the stage/rail hide — the black :host background stays, so the
     * iframe doesn't flash the page's default white. */
    :host([data-fonts-pending]) .stage,
    :host([data-fonts-pending]) .rail,
    :host([data-fonts-pending]) .notes { opacity: 0; pointer-events: none; }

    .stage {
      position: absolute;
      inset: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      background: transparent;
    }
    :host(:not([data-windowed])) .stage { background: #000; }

    .canvas {
      position: relative;
      transform-origin: center center;
      flex-shrink: 0;
      background: #fff;
      will-change: transform;
    }
    /* Floating card drop-shadow — only when the stage is windowed (20px
       margin), i.e. edit/preview. Full-bleed presenting / noscale / _snthumb
       carry no shadow (_fit toggles data-windowed). */
    :host([data-windowed]) .canvas {
      border: 0.882px solid #DEE0E3;
      box-shadow: 0 0 7.052px 0 rgba(31, 35, 41, 0.06), 0 0 15.867px 0 rgba(31, 35, 41, 0.04), 0 0 22.919px 7.052px rgba(31, 35, 41, 0.04);
    }

    /* Slides live in light DOM (via <slot>) so authored CSS still applies.
       We absolutely position each slotted child to stack them. */
    ::slotted(*) {
      position: absolute !important;
      inset: 0 !important;
      width: 100% !important;
      height: 100% !important;
      box-sizing: border-box !important;
      overflow: hidden;
      opacity: 0;
      pointer-events: none;
      visibility: hidden;
    }
    ::slotted([data-deck-active]) {
      opacity: 1;
      pointer-events: auto;
      visibility: visible;
    }

    /* ── Thumbnail rail ──────────────────────────────────────────────────
       Fixed column on the left; each thumbnail is a static deep-clone of
       the light-DOM slide scaled into a 16:9 (or design-aspect) frame. The
       stage re-fits around it (see _fit); hidden during present / noscale
       / print so capture geometry and fullscreen output are unchanged. */
    .rail {
      position: fixed;
      left: 0;
      top: 0;
      bottom: 0;
      width: var(--deck-rail-w, 192px);
      background: #fff;
      border-right: 0.5px solid #DEE0E3;
      overflow-y: auto;
      overflow-x: hidden;
      transform: translate3d(0, 0, 0);
      padding: 20px 15px 20px 10px;
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      gap: 18px;
      z-index: 2147482500;
      scrollbar-width: thin;
      scrollbar-color: rgba(0,0,0,0.18) transparent;
    }
    .rail::-webkit-scrollbar { width: 8px; }
    .rail::-webkit-scrollbar-track { background: transparent; margin: 2px; }
    .rail::-webkit-scrollbar-thumb {
      background: rgba(0,0,0,0.18);
      border-radius: 4px;
      border: 2px solid transparent;
      background-clip: content-box;
    }
    .rail::-webkit-scrollbar-thumb:hover {
      background: rgba(0,0,0,0.28);
      border: 2px solid transparent;
      background-clip: content-box;
    }
    :host([no-rail]) .rail,
    :host([noscale]) .rail { display: none; }
    .rail[data-presenting] { display: none; }
    @media (max-width: 640px) {
      .rail, .rail-resize { display: none; }
    }
    /* User-driven show/hide (the TweaksPanel toggle) slides instead of
       popping. Transitions are gated on :host([data-rail-anim]) — set only
       for the 200ms around the toggle — so window-resize and rail-width
       drag (which also call _fit) don't lag behind the cursor. */
    .rail[data-user-hidden] { transform: translate3d(-100%, 0, 0); }
    :host([data-rail-anim]) .rail { transition: transform 200ms cubic-bezier(.3,.7,.4,1); }
    :host([data-rail-anim]) .stage { transition: left 200ms cubic-bezier(.3,.7,.4,1); }
    :host([data-rail-anim]) .canvas { transition: transform 200ms cubic-bezier(.3,.7,.4,1); }
    :host([data-rail-anim]) .notes { transition: left 200ms cubic-bezier(.3,.7,.4,1); }

    .thumb {
      position: relative;
      display: flex;
      align-items: flex-start;
      gap: 9px;
      cursor: pointer;
      user-select: none;
    }
    .thumb[data-dragging] { cursor: grabbing; }
    .thumb .num {
      width: 14px;
      flex-shrink: 0;
      font-size: 12px;
      line-height: 1;
      font-weight: 400;
      text-align: center;
      color: #646A73;
      padding-top: 8px;
      font-variant-numeric: tabular-nums;
    }
    .thumb .frame {
      position: relative;
      flex: 1;
      min-width: 0;
      aspect-ratio: var(--deck-aspect);
      background: #fff;
      border-radius: 6px;
      /* Default hairline as a box-shadow, not a border: the frame is the
         absolutely-positioned origin for the cloned slide, so a real border
         would shift the clone; box-shadow doesn't affect layout, so
         _scaleThumbs' frame.offsetWidth read stays correct. */
      box-shadow: 0 0 0 1px #DEE0E3;
      outline: 2px solid transparent;
      outline-offset: 2px;
      overflow: hidden;
      transition: outline-color 120ms ease, box-shadow 120ms ease;
    }
    .thumb:hover .frame { outline-color: #1f23290d; }
    .thumb { outline: none; }
    .thumb:focus-visible .frame { outline-color: #1f23290d; }
    .thumb[data-current] .num { color: #1F2329; font-weight: 500; }
    .thumb[data-current] .frame { outline-color: #1F2329; }
    /* Elevated "picked up" look on the *stationary* source while dragging
       (outline hidden, card lifted); the ::before drop-line marks the landing
       position. The floating ghost under the cursor is a separate custom
       drag-image built in _makeDragGhost — the browser's native snapshot of
       .thumb clipped this frame's outside-offset ring and carried the number
       gutter, so we no longer let it draw the ghost. */
    .thumb[data-dragging] .frame {
      outline-color: transparent;
      box-shadow: 0 8px 24px rgba(31,35,41,0.22), 0 0 0 1px rgba(31,35,41,0.08);
    }
    .thumb::before {
      content: '';
      position: absolute;
      left: 24px;
      right: 0;
      height: 3px;
      border-radius: 2px;
      background: #336DF4;
      opacity: 0;
      pointer-events: none;
    }
    .thumb[data-drop="before"]::before { top: -8px; opacity: 1; }
    .thumb[data-drop="after"]::before { bottom: -8px; opacity: 1; }
    .thumb .skip-icon {
      display: none;
      position: absolute;
      inset: 0;
      align-items: center;
      justify-content: center;
      background: rgba(255,255,255,0.6);
      z-index: 1;
    }
    .thumb[data-skip] .skip-icon { display: flex; }

    .ctxmenu {
      position: fixed;
      min-width: 160px;
      padding: 4px;
      background: #fff;
      color: #1F2329;
      border: 0.5px solid #D2D5D8;
      border-radius: 6px;
      box-shadow: 0 4px 8px -8px rgba(0, 0, 0, 0.06), 0 6px 12px 0 rgba(0, 0, 0, 0.04), 0 8px 24px 8px rgba(0, 0, 0, 0.04);
      z-index: 2147483100;
      display: none;
      font-size: 14px;
      line-height: 22px;
    }
    .ctxmenu[data-open] { display: block; }
    .ctxmenu button {
      display: block;
      width: 100%;
      appearance: none;
      border: 0;
      background: transparent;
      color: #1F2329;
      font: inherit;
      text-align: left;
      padding: 6px 8px;
      border-radius: 4px;
      cursor: pointer;
    }
    .ctxmenu button:hover:not(:disabled) { background: #F2F3F5; }
    .ctxmenu button:disabled { opacity: 0.35; cursor: default; }
    .ctxmenu hr {
      border: 0;
      border-top: 0.5px solid #D2D5D8;
      margin: 4px -4px;
    }

    .rail-resize {
      position: fixed;
      left: calc(var(--deck-rail-w, 192px) - 3px);
      top: 0;
      bottom: 0;
      width: 6px;
      cursor: col-resize;
      z-index: 2147482600;
      touch-action: none;
    }
    .rail-resize:hover,
    .rail-resize[data-dragging] { background: rgba(255,255,255,0.12); }
    :host([no-rail]) .rail-resize,
    :host([noscale]) .rail-resize,
    .rail[data-presenting] + .rail-resize,
    .rail[data-user-hidden] + .rail-resize { display: none; }

    /* ── 移动端竖排浏览 ─────────────────────────────────────── */
    :host([data-mobile-list]) .stage {
      display: block;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
      background: #F8F9FA;
    }
    :host([data-mobile-list]) .canvas {
      transform: none !important;
      background: transparent;
      will-change: auto;
    }
    :host([data-mobile-list][data-windowed]) .canvas { border: none; box-shadow: none; }
    :host([data-mobile-list]) .rail, :host([data-mobile-list]) .rail-resize { display: none; }
    :host([data-mobile-list]) ::slotted(*) {
      opacity: 1;
      visibility: visible;
      pointer-events: auto;
      transform-origin: 0 0;
      box-sizing: border-box;
    }
    /* Skipped slides get no nth-child placement rule from _fitMobileList —
       without this they'd fall back to the base inset:0 stacking rule and,
       now force-visible, paint over the whole list canvas and swallow taps.
       (The equivalent rule further down lives inside @media print only.) */
    :host([data-mobile-list]) ::slotted([data-deck-skip]) { display: none !important; }

    /* ── 移动端播放：CSS 旋转伪横屏（不依赖 orientation.lock）────
       只在设备物理竖屏时旋转；用户顺势把手机转成横屏（系统自动旋转）
       后，视口本身已是横屏，再叠 90° 会让画面侧躺——此时退化为常规
       full-bleed 适配，_fitMobilePresent/_mobPresentZone 同步按
       orientation 分支换轴。 */
    :host([data-mobile-present]) .stage { background: #000; }
    @media (orientation: portrait) {
      :host([data-mobile-present]) .stage {
        inset: auto;
        left: 50%;
        top: 50%;
        width: 100vh;
        height: 100vw;
        transform: translate(-50%, -50%) rotate(90deg);
      }
    }

    /* ── 移动端页码徽标（规格：Figma 2965:32047）────────────── */
    .mob-badges { display: none; }
    :host([data-mobile-list]) .mob-badges {
      display: block;
      position: absolute;
      inset: 0;
      pointer-events: none;
      z-index: 10;
    }
    .mob-badge {
      position: absolute;
      height: 12px;
      box-sizing: border-box;
      padding: 0 2px;
      border-radius: 3px;
      background: rgba(31, 35, 41, 0.2);
      color: #fff;
      font-family: 'PingFang SC', -apple-system, system-ui, sans-serif;
      font-size: 8px;
      line-height: 12px;
      text-align: center;
      opacity: 0;
      transition: opacity 0.1s linear;
    }
    :host([data-mob-scrolling]) .mob-badge { opacity: 1; }

    /* ── 移动端播放控件层（规格：Figma 2965:34386）──────────── */
    .mob-ui { display: none; }
    :host([data-mobile-present][data-mob-ui]) .mob-ui {
      display: block;
      position: absolute;
      inset: 0;
      pointer-events: none;
      z-index: 20;
    }
    .mob-back {
      position: absolute;
      left: max(16px, env(safe-area-inset-left));
      top: 12px;
      pointer-events: auto;
      display: flex;
      align-items: center;
      gap: 6px;
      border: none;
      border-radius: 999px;
      padding: 6px 14px;
      background: rgba(0, 0, 0, 0.4);
      color: #fff;
      font-family: 'PingFang SC', -apple-system, system-ui, sans-serif;
      font-size: 15px;
      font-weight: 500;
      cursor: pointer;
    }
    .mob-strip {
      position: absolute;
      left: 0;
      right: 0;
      bottom: 0;
      pointer-events: auto;
      display: flex;
      gap: 8px;
      padding: 10px 16px calc(10px + env(safe-area-inset-bottom));
      /* 透明浮在 slide 上（验收对齐设计稿）——缩略图自带白底+描边已足够区分 */
      background: transparent;
      overflow-x: auto;
    }
    .mob-strip .mthumb { flex: 0 0 132px; cursor: pointer; }
    .mob-strip .mframe {
      width: 132px;
      height: 72px;
      border-radius: 8px;
      border: 0.5px solid #DEE0E3;
      overflow: hidden;
      position: relative;
      background: #fff;
    }
    .mob-strip .mthumb[data-current] .mframe {
      outline: 1px solid #336DF4;
      outline-offset: 2px;
      border-radius: 10px;
    }
    .mob-strip .mnum {
      margin-top: 2px;
      text-align: center;
      font-family: 'PingFang SC', -apple-system, system-ui, sans-serif;
      font-size: 12px;
      line-height: 20px;
      font-weight: 500;
      /* 条带透明、底衬通常是播放态黑底/slide 内容——页码用白色（对齐设计稿） */
      color: rgba(255, 255, 255, 0.9);
    }
    .mob-strip .mthumb[data-current] .mnum { color: #336DF4; }

    /* ── Speaker-notes dock ──────────────────────────────────────────────
       Bottom panel spanning the non-rail width in edit mode: a resize grip
       (drag to set height, persisted to localStorage) + an editable textarea
       bound to the current slide's note. Hidden while presenting / preview /
       narrow / noscale / no-rail / print — same editing-chrome gating as the
       rail. Its left edge is set to the live rail width by _fit. */
    .notes {
      position: fixed;
      left: var(--deck-rail-w, 192px);
      right: 20px;
      bottom: 0;
      height: var(--deck-notes-h, 160px);
      background: #fff;
      border: 0.5px solid #D2D5D8;
      border-bottom: none;
      border-radius: 12px 12px 0 0;
      box-shadow: 0 2px 4px -4px rgba(31, 35, 41, 0.02), 0 4px 8px 0 rgba(31, 35, 41, 0.02), 0 4px 16px 4px rgba(31, 35, 41, 0.03);
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      z-index: 2147482400;
    }
    :host([no-rail]) .notes,
    :host([noscale]) .notes { display: none; }
    .notes[data-hidden] { display: none; }
    @media (max-width: 640px) { .notes { display: none; } }
    .notes-handle {
      flex-shrink: 0;
      height: 20px;
      display: flex;
      align-items: center;
      justify-content: center;
      cursor: row-resize;
      color: #8F959E;
      font-size: 18px;
      line-height: 20px;
      letter-spacing: 2px;
      user-select: none;
      touch-action: none;
    }
    .notes-handle:hover,
    .notes-handle[data-dragging] { color: #646A73; }
    .notes-body {
      flex: 1;
      min-height: 0;
      width: 100%;
      box-sizing: border-box;
      border: 0;
      outline: none;
      resize: none;
      background: transparent;
      color: #1F2329;
      font: inherit;
      font-size: 14px;
      line-height: 1.6;
      padding: 4px 24px 12px;
      overflow-y: auto;
    }
    .notes-body::placeholder { color: #8F959E; }

    /* ── Print: one page per slide, no chrome ────────────────────────────
       The screen layout stacks every slide at inset:0 inside a scaled
       canvas; for print we want them in document flow at the authored
       design size so the browser paginates one slide per sheet. The
       @page size is set from the width/height attributes via the inline
       <style id="deck-stage-print-page"> that connectedCallback injects
       into <head> (the @page at-rule has no effect inside shadow DOM). */
    @media print {
      :host {
        position: static;
        inset: auto;
        background: none;
        overflow: visible;
        color: inherit;
      }
      .stage { position: static; display: block; }
      .canvas {
        transform: none !important;
        width: auto !important;
        height: auto !important;
        background: none;
        will-change: auto;
        /* !important: :host([data-windowed]) .canvas (0,2,0) outranks a bare
           .canvas rule inside this media query. */
        box-shadow: none !important;
      }
      ::slotted(*) {
        position: relative !important;
        inset: auto !important;
        width: var(--deck-design-w) !important;
        height: var(--deck-design-h) !important;
        box-sizing: border-box !important;
        opacity: 1 !important;
        visibility: visible !important;
        pointer-events: auto;
        break-after: page;
        page-break-after: always;
        break-inside: avoid;
        overflow: hidden;
      }
      /* :last-child alone isn't enough once data-deck-skip hides the
         trailing slide(s) — the last *visible* slide still carries
         break-after:page and prints a blank sheet. _markLastVisible()
         maintains data-deck-last-visible on the last non-skipped slide. */
      ::slotted(*:last-child),
      ::slotted([data-deck-last-visible]) {
        break-after: auto;
        page-break-after: auto;
      }
      ::slotted([data-deck-skip]) { display: none !important; }
      .rail, .rail-resize, .ctxmenu, .notes { display: none !important; }
    }
  `;

  class DeckStage extends HTMLElement {
    static get observedAttributes() { return ['width', 'height', 'noscale', 'no-rail']; }

    constructor() {
      super();
      this._root = this.attachShadow({ mode: 'open' });
      this._index = 0;
      this._slides = [];
      this._notes = [];
      this._menuIndex = -1;
      this._notesPersistTimer = null;

      this._onKey = this._onKey.bind(this);
      this._onResize = this._onResize.bind(this);
      this._onSlotChange = this._onSlotChange.bind(this);
      this._onTap = this._onTap.bind(this);
      this._onMessage = this._onMessage.bind(this);
      this._onHashChange = this._onHashChange.bind(this);
      // Capture-phase close so a click anywhere dismisses the menu, but
      // ignore clicks that land inside the menu itself — otherwise the
      // capture handler runs before the menu's own (bubble) handler and
      // clears _menuIndex out from under it.
      this._onDocClick = (e) => {
        if (this._menu && e.composedPath && e.composedPath().includes(this._menu)) return;
        this._closeMenu();
      };
    }

    get designWidth() {
      return parseInt(this.getAttribute('width'), 10) || DESIGN_W_DEFAULT;
    }
    get designHeight() {
      return parseInt(this.getAttribute('height'), 10) || DESIGN_H_DEFAULT;
    }

    connectedCallback() {
      // Capture modes that render a clean full-bleed single slide, both routed
      // through the same no-rail path: the host's pre-screenshot ?thumbnail=1
      // (product thumbnail — slide 1) and the presenter-view popup's
      // ?_snthumb=...#N (prev/cur/next thumbnails). Both drop the rail (wrong
      // scale, offsets the stage into a gutter) and render full-bleed — no
      // windowed margin/shadow, see _fit. A bare load with no #N hash starts at
      // slide 1, so ?thumbnail=1 alone yields a clean first-slide frame.
      this._snthumb = /[?&](_snthumb|thumbnail)=/.test(location.search);
      this._mobile = isMobileUA();
      this._syncMobileMode();
      if (this._snthumb) this.setAttribute('no-rail', '');
      this._render();
      this._loadNotes();
      this._syncPrintPageRule();
      window.addEventListener('keydown', this._onKey);
      window.addEventListener('resize', this._onResize);
      window.addEventListener('message', this._onMessage);
      window.addEventListener('hashchange', this._onHashChange);
      window.addEventListener('click', this._onDocClick, true);
      this.addEventListener('click', this._onTap);
      // Print lays every slide out as its own page, so [data-deck-active]-
      // gated entrance styles need the attribute on every slide (not just
      // the current one) or their content prints at the hidden base style.
      // The transient freeze style lands BEFORE the attributes so any
      // attribute-keyed transition fires at 0s (changing transition-
      // duration after a transition has started doesn't affect it).
      this._onBeforePrint = () => {
        if (this._freezeStyle) this._freezeStyle.remove();
        this._freezeStyle = document.createElement('style');
        this._freezeStyle.textContent = '*,*::before,*::after{transition-duration:0s !important}';
        document.head.appendChild(this._freezeStyle);
        this._slides.forEach((s) => s.setAttribute('data-deck-active', ''));
      };
      this._onAfterPrint = () => {
        this._applyIndex({ broadcast: false });
        if (this._freezeStyle) { this._freezeStyle.remove(); this._freezeStyle = null; }
      };
      window.addEventListener('beforeprint', this._onBeforePrint);
      window.addEventListener('afterprint', this._onAfterPrint);
      // Initial collection + layout happens via slotchange, which fires on mount.
      this._enableRail();
      // Hold the stage hidden until webfonts are ready so the first visible
      // paint has the deck's real typography — the :not(:defined) guard in
      // the page HTML only covers custom-element upgrade, not font load.
      // Capped so a 404'd font URL can't blank the deck indefinitely.
      this.setAttribute('data-fonts-pending', '');
      const reveal = () => this.removeAttribute('data-fonts-pending');
      // rAF first: fonts.ready is a pre-resolved promise until layout has
      // resolved the slotted text's font-family and pushed a FontFace into
      // 'loading'. Reading it here in connectedCallback (parse-time) would
      // settle the race in a microtask before any font fetch starts.
      requestAnimationFrame(() => {
        Promise.race([
          document.fonts ? document.fonts.ready : Promise.resolve(),
          new Promise((r) => setTimeout(r, 2000)),
        ]).then(reveal, reveal);
      });
      // Announce to the host that this product IS a deck scene.
      try { window.parent.postMessage({ type: 'miaoda:deck:available' }, '*'); } catch (e) {}
    }

    _enableRail() {
      // Idempotent — host may post miaoda:deck:rail-enabled multiple times.
      // no-rail guard keeps the observers/stylesheet walk off the cheap path
      // for presenter-popup thumbnail iframes (up to 9 per view).
      if (this._railEnabled || this.hasAttribute('no-rail')) return;
      this._railEnabled = true;
      // Per-viewer preference — restored alongside rail width. Default on;
      // only a stored '0' (from the TweaksPanel toggle) hides it.
      this._railVisible = true;
      try {
        if (localStorage.getItem('deck-stage.railVisible') === '0') this._railVisible = false;
      } catch (e) {}
      // Live thumbnail updates: watch the light-DOM slides for content
      // edits and re-clone just the affected thumb(s), debounced. Ignore
      // the data-deck-* / data-screen-label / data-miaoda-validate attributes
      // this component itself writes so nav and skip don't trigger
      // spurious refreshes.
      const OWN_ATTRS = /^data-(deck-|screen-label$|om-validate$)/;
      this._liveDirty = new Set();
      this._liveObserver = new MutationObserver((records) => {
        for (const r of records) {
          if (r.type === 'attributes' && OWN_ATTRS.test(r.attributeName || '')) continue;
          let n = r.target;
          while (n && n.parentElement !== this) n = n.parentElement;
          if (n && this._slideSet && this._slideSet.has(n)) this._liveDirty.add(n);
        }
        if (this._liveDirty.size && !this._liveTimer) {
          this._liveTimer = setTimeout(() => {
            this._liveTimer = null;
            this._liveDirty.forEach((s) => this._refreshThumb(s));
            this._liveDirty.clear();
          }, 200);
        }
      });
      this._liveObserver.observe(this, {
        subtree: true, childList: true, characterData: true, attributes: true,
      });
      // Lazy thumbnail materialization — clone the slide only when its
      // frame scrolls into (or near) the rail viewport. rootMargin gives
      // ~4 thumbs of pre-load so fast scrolling doesn't flash blanks.
      this._railObserver = new IntersectionObserver((entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting && e.target.__deckThumb) {
            this._materialize(e.target.__deckThumb);
          }
        });
      }, { root: this._rail, rootMargin: '400px 0px' });
      // Tweaks typically change CSS vars / attrs OUTSIDE <deck-stage>
      // (on <html>, <body>, a wrapper div, or a <style> tag), which
      // _liveObserver can't see. Re-snapshot author CSS (constructable
      // sheet is shared by reference, so one replaceSync updates every
      // thumb shadow root) and re-sync each thumb host's attrs + custom
      // properties. In-slide DOM mutations are _liveObserver's job.
      // Debounced so slider drags don't thrash.
      this._onTweakChange = () => {
        clearTimeout(this._tweakTimer);
        this._tweakTimer = setTimeout(() => {
          this._snapshotAuthorCss();
          // One getComputedStyle for the whole batch — each
          // getPropertyValue read below reuses the same computed style
          // as long as nothing invalidates layout between thumbs.
          const cs = getComputedStyle(this);
          (this._thumbs || []).forEach((t) => {
            if (t.host) this._syncThumbHostAttrs(t.host, cs);
          });
        }, 120);
      };
      window.addEventListener('tweakchange', this._onTweakChange);
      this._snapshotAuthorCss();
      // Build the rail now that it's enabled — slotchange already fired,
      // so _renderRail's early-return skipped the initial build.
      this._syncRailHidden();
      this._renderRail();
      this._fit();
      // The notes dock is editing chrome too — show it and load the current
      // slide's note now that the rail feature is enabled.
      this._syncNotesHidden();
      this._syncNotesBody();
    }

    /** Snapshot document stylesheets into a constructable sheet that each
     *  thumbnail's nested shadow root adopts — so author CSS styles the
     *  cloned slide content without touching this component's chrome.
     *  Re-callable: the constructable sheet is reused via replaceSync so
     *  every already-adopted shadow root picks up the fresh CSS.
     *
     *  Cross-origin sheets (a CDN styles.css) throw SecurityError on
     *  .cssRules — the rules style the light DOM but CSSOM won't expose them,
     *  so the snapshot comes back empty and thumbnail clones render unstyled.
     *  Re-fetch those sheets' source and re-apply when it lands; the shared
     *  adopted sheet means one replaceSync restyles every clone (no re-clone).
     *  The fetch is CORS-gated — a CDN without Access-Control-Allow-Origin
     *  fails it and we degrade to an unstyled clone, no worse than before. */
    _snapshotAuthorCss() {
      // Generation guard: a stale fetch (from a snapshot a newer call has
      // superseded) must not clobber the current adopted CSS.
      const gen = (this._cssGen = (this._cssGen || 0) + 1);
      if (!this._xoCssCache) this._xoCssCache = new Map(); // href → fetched text ('' = failed)
      const pending = [];
      this._applyAuthorCss(this._collectAuthorCss(pending));
      if (!pending.length) return;
      Promise.all(pending.map((href) =>
        fetch(href).then((r) => (r.ok ? r.text() : '')).catch(() => '')
          .then((text) => { this._xoCssCache.set(href, text); })
      )).then(() => {
        if (this._cssGen !== gen) return;
        this._applyAuthorCss(this._collectAuthorCss(null));
        if (this._adoptedSheet) {
          // Shared sheet already restyled every clone — just re-mirror vars.
          const cs = getComputedStyle(this);
          (this._thumbs || []).forEach((t) => { if (t.host) this._syncThumbHostAttrs(t.host, cs); });
        } else {
          // Fallback (no constructable sheets): each clone baked its own
          // <style> at materialize, so re-clone to pick up the fetched CSS.
          (this._thumbs || []).forEach((t) => { if (t.host) this._refreshThumb(t.slide); });
        }
      });
    }

    /** Concatenate every document stylesheet's rules in document order. A
     *  cross-origin sheet whose .cssRules throws is unreadable via CSSOM:
     *  substitute its re-fetched source if cached, else push its href to
     *  pendingOut (when provided) and leave a placeholder until the fetch lands. */
    _collectAuthorCss(pendingOut) {
      return Array.from(document.styleSheets).map((sh) => {
        try {
          return Array.from(sh.cssRules).map((r) => r.cssText).join('\n');
        } catch (e) {
          const href = sh.href;
          if (!href) return '';
          if (this._xoCssCache.has(href)) return this._xoCssCache.get(href);
          if (pendingOut) pendingOut.push(href);
          return '';
        }
      }).join('\n');
    }

    /** Rewrite raw author CSS so its document-scoped selectors match inside a
     *  thumbnail's shadow root, then (re)load it into the shared adopted sheet.
     *  :root in an adopted sheet inside a shadow root matches nothing (only the
     *  document root qualifies), so author rules like
     *  `:root[data-voice="modern"] .serif` never reach the clones. Rewrite
     *  :root → :host and mirror <html>'s data-*, class and lang onto each thumb
     *  host (see _syncThumbHostAttrs) so the same selectors match inside it.
     *  The <deck-stage> type selector gets the same treatment: from a clone's
     *  point of view inside an isolated thumb shadow root there is no real
     *  <deck-stage> ancestor, so a rule like `deck-stage section { background }`
     *  (the common per-slide default-background idiom) would never match and
     *  the clone renders with a transparent background — through which the
     *  page-level html/body background (rewritten to :host above) leaks. The
     *  thumb's own host IS the deck-stage from the clone's perspective, so
     *  alias deck-stage → :host to restore the match. */
    _applyAuthorCss(rawCss) {
      const authorCss = rawCss
        // The shadow host is featureless outside the functional :host(...)
        // form, so any compound on :root — [attr], .class, #id, :pseudo —
        // must become :host(<compound>) not :host<compound>. Same for the
        // html type selector (Tailwind class-strategy dark mode emits
        // html.dark; Pico uses html[data-theme]), which has nothing to
        // match inside the thumb's shadow tree.
        .replace(/:root((?:\[[^\]]*\]|[.#][-\w]+|:[-\w]+(?:\([^)]*\))?)+)/g, ':host($1)')
        .replace(/:root\b/g, ':host')
        .replace(/(^|[\s,>~+(}])html((?:\[[^\]]*\]|[.#][-\w]+|:[-\w]+(?:\([^)]*\))?)+)(?![-\w])/g, '$1:host($2)')
        .replace(/(^|[\s,>~+(}])html(?![-\w])/g, '$1:host')
        // Same rewrite for the deck-stage type selector (see the doc comment).
        // The negative-lookahead (?![-\w]) keeps `.deck-stage-title` and the
        // `--deck-stage-*` custom-property names untouched.
        .replace(/(^|[\s,>~+(}])deck-stage((?:\[[^\]]*\]|[.#][-\w]+|:[-\w]+(?:\([^)]*\))?)+)(?![-\w])/g, '$1:host($2)')
        .replace(/(^|[\s,>~+(}])deck-stage(?![-\w])/g, '$1:host');
      // Every custom property the author references. _syncThumbHostAttrs
      // mirrors each one's *computed* value at <deck-stage> onto the
      // thumb host so the live value wins over the :host default above
      // regardless of which ancestor the tweak wrote to (<html>, <body>,
      // a wrapper div, or the deck-stage element itself all inherit
      // down to getComputedStyle(this)).
      this._authorVars = new Set(authorCss.match(/--[\w-]+/g) || []);
      try {
        if (!this._adoptedSheet) this._adoptedSheet = new CSSStyleSheet();
        this._adoptedSheet.replaceSync(authorCss);
      } catch (e) {
        this._adoptedSheet = null;
        this._authorCss = authorCss;
      }
    }

    _syncThumbHostAttrs(host, cs) {
      const de = document.documentElement;
      // setAttribute overwrites but can't delete — an attr removed from
      // <html> (toggleAttribute off, classList emptied) would linger on
      // the host and :host([data-*]) / :host(.foo) rules would keep
      // matching. Remove stale mirrored attrs first; iterate backward
      // because removeAttribute mutates the live NamedNodeMap.
      for (let i = host.attributes.length - 1; i >= 0; i--) {
        const n = host.attributes[i].name;
        if ((n.startsWith('data-') || n === 'class' || n === 'lang')
            && !de.hasAttribute(n)) {
          host.removeAttribute(n);
        }
      }
      for (const a of de.attributes) {
        if (a.name.startsWith('data-') || a.name === 'class' || a.name === 'lang') {
          host.setAttribute(a.name, a.value);
        }
      }
      // The :root→:host rewrite in _snapshotAuthorCss pins each custom
      // property to its stylesheet default on the thumb host, shadowing
      // the live value that would otherwise inherit. Tweaks can write the
      // live value on any ancestor — <html>, <body>, a wrapper div, the
      // deck-stage element — so read it as the *computed* value at
      // <deck-stage> (which sees the whole inheritance chain) rather than
      // trying to guess which element the author wrote to. Inline on the
      // host beats the :host{} rule. remove-stale covers vars dropped
      // from the stylesheet between snapshots.
      const vars = this._authorVars || new Set();
      for (let i = host.style.length - 1; i >= 0; i--) {
        const p = host.style[i];
        if (p.startsWith('--') && !vars.has(p)) host.style.removeProperty(p);
      }
      const live = cs || getComputedStyle(this);
      vars.forEach((p) => {
        const v = live.getPropertyValue(p);
        if (v) host.style.setProperty(p, v.trim());
        else host.style.removeProperty(p);
      });
    }

    disconnectedCallback() {
      window.removeEventListener('keydown', this._onKey);
      window.removeEventListener('resize', this._onResize);
      window.removeEventListener('message', this._onMessage);
      window.removeEventListener('hashchange', this._onHashChange);
      window.removeEventListener('click', this._onDocClick, true);
      window.removeEventListener('beforeprint', this._onBeforePrint);
      window.removeEventListener('afterprint', this._onAfterPrint);
      if (this._freezeStyle) { this._freezeStyle.remove(); this._freezeStyle = null; }
      if (this._dragGhost) { this._dragGhost.remove(); this._dragGhost = null; }
      this.removeEventListener('click', this._onTap);
      if (this._liveTimer) clearTimeout(this._liveTimer);
      if (this._tweakTimer) clearTimeout(this._tweakTimer);
      if (this._railAnimTimer) clearTimeout(this._railAnimTimer);
      if (this._notesPersistTimer) clearTimeout(this._notesPersistTimer);
      clearTimeout(this._mobUiTimer);
      clearTimeout(this._mobScrollIdleTimer);
      if (this._mobScrollRaf) { cancelAnimationFrame(this._mobScrollRaf); this._mobScrollRaf = null; }
      if (this._scaleRaf) cancelAnimationFrame(this._scaleRaf);
      if (this._liveObserver) this._liveObserver.disconnect();
      if (this._railObserver) this._railObserver.disconnect();
      if (this._onTweakChange) window.removeEventListener('tweakchange', this._onTweakChange);
    }

    attributeChangedCallback() {
      if (this._canvas) {
        this._canvas.style.width = this.designWidth + 'px';
        this._canvas.style.height = this.designHeight + 'px';
        this._canvas.style.setProperty('--deck-design-w', this.designWidth + 'px');
        this._canvas.style.setProperty('--deck-design-h', this.designHeight + 'px');
        if (this._rail) {
          this._rail.style.setProperty('--deck-aspect', this.designWidth + '/' + this.designHeight);
        }
        this._syncMobileMode();
        this._fit();
        this._scaleThumbs();
        this._syncPrintPageRule();
      }
    }

    _render() {
      const style = document.createElement('style');
      style.textContent = stylesheet;

      const stage = document.createElement('div');
      stage.className = 'stage';

      const canvas = document.createElement('div');
      canvas.className = 'canvas';
      canvas.style.width = this.designWidth + 'px';
      canvas.style.height = this.designHeight + 'px';
      canvas.style.setProperty('--deck-design-w', this.designWidth + 'px');
      canvas.style.setProperty('--deck-design-h', this.designHeight + 'px');

      const slot = document.createElement('slot');
      slot.addEventListener('slotchange', this._onSlotChange);
      canvas.appendChild(slot);
      stage.appendChild(canvas);

      this._onMobScroll = this._onMobScroll.bind(this);
      stage.addEventListener('scroll', this._onMobScroll, { passive: true });

      // 移动竖排的逐页定位规则（::slotted(:nth-child(i))），_fitMobileList 重建。
      this._mobStyle = document.createElement('style');

      this._mobBadges = document.createElement('div');
      this._mobBadges.className = 'mob-badges export-hidden';
      this._mobBadges.setAttribute('data-miaoda-chrome', '');
      canvas.appendChild(this._mobBadges);

      // Thumbnail rail + context menu. Thumbnails are populated in
      // _renderRail() after _collectSlides().
      const rail = document.createElement('div');
      rail.className = 'rail export-hidden';
      rail.setAttribute('data-miaoda-chrome', '');
      rail.style.setProperty('--deck-aspect', this.designWidth + '/' + this.designHeight);
      // Edge auto-scroll while dragging a thumb near the rail's top/bottom
      // so off-screen drop targets are reachable. Native dragover fires
      // continuously while the pointer is stationary, so a per-event nudge
      // (ramped by edge proximity) is enough — no rAF loop needed.
      rail.addEventListener('dragover', (e) => {
        if (this._dragFrom == null) return;
        // preventDefault makes the whole rail — including the gaps between
        // thumbs, where the drop-line actually renders — a valid drop target.
        // Without it a release in a gap is rejected and the slide snaps back
        // even though the indicator was showing.
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        const r = rail.getBoundingClientRect();
        const EDGE = 40;
        const dt = e.clientY - r.top;
        const db = r.bottom - e.clientY;
        if (dt < EDGE) rail.scrollTop -= Math.ceil((EDGE - dt) / 3);
        else if (db < EDGE) rail.scrollTop += Math.ceil((EDGE - db) / 3);
        // Over a thumb its own dragover set the indicator (O(1)); over a gap
        // no thumb handler fires, so resolve the nearest slot here.
        const overThumb = e.target && e.target.closest && e.target.closest('.thumb');
        if (!overThumb) {
          const tgt = this._railDropTarget(e.clientY);
          if (tgt) this._setDrop(tgt.i, tgt.pos);
        }
      });
      rail.addEventListener('drop', (e) => {
        // A drop over a thumb is handled by the thumb's own handler, which
        // nulls _dragFrom before the event bubbles here — so this branch only
        // runs for releases that land in a gap / the rail padding. It commits
        // to whatever slot the indicator is currently showing.
        if (this._dragFrom == null) return;
        e.preventDefault();
        const on = this._dropOn;
        const from = this._dragFrom;
        this._dragFrom = null;
        let to = on ? (this._dropWhere === 'after' ? on.i + 1 : on.i) : from;
        if (from < to) to--;
        this._clearDrop();
        if (on && to !== from) this._moveSlide(from, to);
      });

      const menu = document.createElement('div');
      menu.className = 'ctxmenu export-hidden';
      menu.setAttribute('data-miaoda-chrome', '');
      menu.innerHTML = `
        <button type="button" data-act="skip">隐藏此页</button>
        <button type="button" data-act="up">上移</button>
        <button type="button" data-act="down">下移</button>
        <hr>
        <button type="button" data-act="delete">删除</button>
      `;
      menu.addEventListener('click', (e) => {
        const act = e.target && e.target.getAttribute && e.target.getAttribute('data-act');
        if (!act) return;
        const i = this._menuIndex;
        this._closeMenu();
        if (act === 'skip') this._toggleSkip(i);
        else if (act === 'up') this._moveSlide(i, i - 1);
        else if (act === 'down') this._moveSlide(i, i + 1);
        else if (act === 'delete') this._deleteSlide(i);
      });
      menu.addEventListener('contextmenu', (e) => e.preventDefault());

      // Rail resize handle — drag to set --deck-rail-w, persisted to
      // localStorage so the width survives reloads.
      const resize = document.createElement('div');
      resize.className = 'rail-resize export-hidden';
      resize.setAttribute('data-miaoda-chrome', '');
      resize.addEventListener('pointerdown', (e) => {
        e.preventDefault();
        resize.setPointerCapture(e.pointerId);
        resize.setAttribute('data-dragging', '');
        const move = (ev) => this._setRailWidth(ev.clientX);
        const up = () => {
          resize.removeEventListener('pointermove', move);
          resize.removeEventListener('pointerup', up);
          resize.removeEventListener('pointercancel', up);
          resize.removeAttribute('data-dragging');
          try { localStorage.setItem('deck-stage.railWidth', String(this._railPx)); } catch (err) {}
        };
        resize.addEventListener('pointermove', move);
        resize.addEventListener('pointerup', up);
        resize.addEventListener('pointercancel', up);
      });

      // Speaker-notes dock — a resize grip + an editable textarea bound to the
      // current slide's note. Chrome (export-hidden / data-miaoda-chrome) like
      // the rail/overlay so it's excluded from PPTX/screenshot capture.
      const notes = document.createElement('div');
      notes.className = 'notes export-hidden';
      notes.setAttribute('data-miaoda-chrome', '');
      const notesHandle = document.createElement('div');
      notesHandle.className = 'notes-handle';
      notesHandle.innerHTML = '<svg width="18" height="18" viewBox="0 0 18 18" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M13.3008 11.4434C13.3008 10.822 13.8045 10.3184 14.4258 10.3184C15.047 10.3185 15.5508 10.8221 15.5508 11.4434C15.5508 12.0646 15.047 12.5682 14.4258 12.5684C13.8045 12.5684 13.3008 12.0647 13.3008 11.4434ZM13.3008 6.55664C13.3008 5.93532 13.8045 5.43164 14.4258 5.43164C15.047 5.43179 15.5508 5.93541 15.5508 6.55664C15.5508 7.17787 15.047 7.68149 14.4258 7.68164C13.8045 7.68164 13.3008 7.17796 13.3008 6.55664ZM7.875 11.4434C7.875 10.822 8.37868 10.3184 9 10.3184C9.62132 10.3184 10.125 10.822 10.125 11.4434C10.125 12.0647 9.62132 12.5684 9 12.5684C8.37868 12.5684 7.875 12.0647 7.875 11.4434ZM7.875 6.55664C7.875 5.93532 8.37868 5.43164 9 5.43164C9.62132 5.43164 10.125 5.93532 10.125 6.55664C10.125 7.17796 9.62132 7.68164 9 7.68164C8.37868 7.68164 7.875 7.17796 7.875 6.55664ZM2.44922 11.4434C2.44922 10.822 2.9529 10.3184 3.57422 10.3184C4.19554 10.3184 4.69922 10.822 4.69922 11.4434C4.69922 12.0647 4.19554 12.5684 3.57422 12.5684C2.9529 12.5684 2.44922 12.0647 2.44922 11.4434ZM2.44922 6.55664C2.44922 5.93532 2.9529 5.43164 3.57422 5.43164C4.19554 5.43164 4.69922 5.93532 4.69922 6.55664C4.69922 7.17796 4.19554 7.68164 3.57422 7.68164C2.9529 7.68164 2.44922 7.17796 2.44922 6.55664Z" fill="currentColor"/></svg>';
      notesHandle.setAttribute('title', '拖动调整高度');
      const notesBody = document.createElement('textarea');
      notesBody.className = 'notes-body';
      notesBody.setAttribute('placeholder', '点击添加演示者备注');
      notesBody.setAttribute('aria-label', '演示者备注');
      notesBody.setAttribute('spellcheck', 'false');
      notes.append(notesHandle, notesBody);

      // Drag the grip to resize the dock height (mirror the rail-resize
      // pattern); persist to localStorage so it survives reloads.
      notesHandle.addEventListener('pointerdown', (e) => {
        e.preventDefault();
        notesHandle.setPointerCapture(e.pointerId);
        notesHandle.setAttribute('data-dragging', '');
        const move = (ev) => this._setNotesHeight(window.innerHeight - ev.clientY);
        const up = () => {
          notesHandle.removeEventListener('pointermove', move);
          notesHandle.removeEventListener('pointerup', up);
          notesHandle.removeEventListener('pointercancel', up);
          notesHandle.removeAttribute('data-dragging');
          try { localStorage.setItem('deck-stage.notesHeight', String(this._notesPx)); } catch (err) {}
        };
        notesHandle.addEventListener('pointermove', move);
        notesHandle.addEventListener('pointerup', up);
        notesHandle.addEventListener('pointercancel', up);
      });

      // Edit → write the current slide's note + debounced persist.
      notesBody.addEventListener('input', () => {
        if (!Array.isArray(this._notes)) this._notes = [];
        this._notes[this._index] = notesBody.value;
        this._scheduleNotesPersist();
      });
      // At window scope the keydown target is retargeted to the host
      // <deck-stage> (not this textarea), so _onKey's INPUT/TEXTAREA guard
      // never matches — stop propagation here so typing (space/arrows/…) edits
      // the note instead of navigating slides (same trick as the thumb keydown).
      notesBody.addEventListener('keydown', (e) => e.stopPropagation());

      this._root.append(style, this._mobStyle, rail, resize, stage, notes, menu);
      this._canvas = canvas;
      this._stage = stage;
      this._slot = slot;
      this._rail = rail;
      this._resize = resize;
      this._menu = menu;
      this._notesPanel = notes;
      this._notesHandle = notesHandle;
      this._notesBody = notesBody;

      // Restore persisted rail width.
      let rw = 192;
      try {
        const s = localStorage.getItem('deck-stage.railWidth');
        if (s) rw = parseInt(s, 10) || rw;
      } catch (err) {}
      this._setRailWidth(rw);
      this._syncRailHidden();

      // Restore persisted notes-dock height.
      let nh = 160;
      try {
        const s = localStorage.getItem('deck-stage.notesHeight');
        if (s) nh = parseInt(s, 10) || nh;
      } catch (err) {}
      this._setNotesHeight(nh);
      this._syncNotesHidden();
    }

    _setRailWidth(px) {
      const w = Math.max(120, Math.min(360, Math.round(px)));
      this._railPx = w;
      this.style.setProperty('--deck-rail-w', w + 'px');
      this._fit();
      // _scaleThumbs forces a sync layout (frame.offsetWidth) then writes
      // N transforms. During a resize drag this runs per-pointermove;
      // coalesce to one per frame.
      if (!this._scaleRaf) {
        this._scaleRaf = requestAnimationFrame(() => {
          this._scaleRaf = null;
          this._scaleThumbs();
        });
      }
    }

    // ── Speaker-notes dock ────────────────────────────────────────────────

    _setNotesHeight(px) {
      const max = Math.max(120, Math.round(window.innerHeight * 0.5));
      const h = Math.max(58, Math.min(max, Math.round(px)));
      this._notesPx = h;
      this.style.setProperty('--deck-notes-h', h + 'px');
      this._fit();
    }

    _notesVisible() {
      if (NOTES_DISABLED) return false;
      return !!this._hasNotesTag && !!this._railEnabled
        && !this._presenting && !this._previewMode
        && !this.hasAttribute('noscale') && !this.hasAttribute('no-rail')
        && !NARROW_MQ.matches;
    }

    /** Height _fit subtracts from the stage region. Clamped so a tall dock on
     *  a short window still leaves ~160px of canvas (no negative/NaN scale). */
    _notesHeight() {
      if (!this._notesVisible()) return 0;
      return Math.min(this._notesPx || 0, Math.max(0, window.innerHeight - 160));
    }

    _syncNotesHidden() {
      if (!this._notesPanel) return;
      // Static conditions (no-rail/noscale/narrow/print) live in CSS; toggle
      // only the runtime-dynamic hide (presenting / preview / rail disabled).
      const hide = NOTES_DISABLED || !this._hasNotesTag || !this._railEnabled || this._presenting || this._previewMode;
      this._notesPanel.toggleAttribute('data-hidden', hide);
      this._notesPanel.inert = !this._notesVisible(); // keep textarea out of tab order when hidden
    }

    /** Load the current slide's note into the body (no-op if unchanged so it
     *  doesn't disturb an in-progress edit). */
    _syncNotesBody() {
      if (!this._notesBody) return;
      const val = (Array.isArray(this._notes) && this._notes[this._index]) || '';
      if (this._notesBody.value !== val) this._notesBody.value = val;
    }

    /** this._notes with holes → '' and length preserved (safe to JSON). */
    _notesForEmit() {
      const src = Array.isArray(this._notes) ? this._notes : [];
      return Array.from(src, (v) => (typeof v === 'string' ? v : (v == null ? '' : String(v))));
    }

    _scheduleNotesPersist() {
      clearTimeout(this._notesPersistTimer);
      this._notesPersistTimer = setTimeout(() => this._emitNotesChange(), 300);
    }

    /** Persist a pure text edit: mirror the in-DOM #speaker-notes tag (if any)
     *  for in-iframe consumers, and post the full array to the parent so the
     *  host writes it to disk — the tag lives OUTSIDE <deck-stage>, so the
     *  deck-changed outerHTML path can't carry notes. */
    _emitNotesChange() {
      const notes = this._notesForEmit();
      this._notes = notes;
      const tag = document.getElementById('speaker-notes');
      if (tag) tag.textContent = JSON.stringify(notes);
      this.dispatchEvent(new CustomEvent('noteschange', {
        detail: { notes, index: this._index }, bubbles: true, composed: true,
      }));
      try { window.parent.postMessage({ type: 'miaoda:deck:notes-changed', index: this._index, notes }, '*'); } catch (e) {}
    }

    /** @page must live in the document stylesheet — it's a no-op inside
     *  shadow DOM. Inject/update a single <head> style tag so the print
     *  sheet matches the design size and Save-as-PDF yields one slide per
     *  page with no margins. */
    _syncPrintPageRule() {
      const id = 'deck-stage-print-page';
      let tag = document.getElementById(id);
      if (!tag) {
        tag = document.createElement('style');
        tag.id = id;
        document.head.appendChild(tag);
      }
      tag.textContent =
        '@page { size: ' + this.designWidth + 'px ' + this.designHeight + 'px; margin: 0; } ' +
        '@media print { html, body { margin: 0 !important; padding: 0 !important; background: none !important; overflow: visible !important; height: auto !important; } ' +
        '* { -webkit-print-color-adjust: exact; print-color-adjust: exact; } ' +
        // Jump authored animations/transitions to their end state so print
        // never captures mid-entrance — pairs with the beforeprint handler
        // in connectedCallback that sets data-deck-active on every slide.
        '*, *::before, *::after { animation-delay: -99s !important; animation-duration: .001s !important; ' +
        'animation-iteration-count: 1 !important; animation-fill-mode: both !important; ' +
        'animation-play-state: running !important; transition-duration: 0s !important; } }';
    }

    _onSlotChange() {
      // Rail mutations (delete/move) already reconcile synchronously and
      // emit slidechange with reason 'api'; skip the async slotchange that
      // would otherwise re-broadcast with reason 'init'.
      if (this._squelchSlotChange) { this._squelchSlotChange = false; return; }
      this._collectSlides();
      this._restoreIndex();
      this._applyIndex({ broadcast: true, reason: 'init' });
      this._fit();
    }

    _collectSlides() {
      const assigned = this._slot.assignedElements({ flatten: true });
      this._slides = assigned.filter((el) => {
        // Skip template/style/script nodes even if someone slots them.
        const tag = el.tagName;
        return tag !== 'TEMPLATE' && tag !== 'SCRIPT' && tag !== 'STYLE';
      });
      this._slideSet = new Set(this._slides);

      this._slides.forEach((slide, i) => {
        const n = i + 1;
        slide.setAttribute('data-screen-label', `${pad2(n)} ${getSlideLabel(slide)}`);

        // Validation attribute for comment flow / auto-checks.
        if (!slide.hasAttribute('data-miaoda-validate')) {
          slide.setAttribute('data-miaoda-validate', VALIDATE_ATTR);
        }

        slide.setAttribute('data-deck-slide', String(i));
      });

      if (this._index >= this._slides.length) this._index = Math.max(0, this._slides.length - 1);
      this._markLastVisible();
      this._renderRail();
    }

    /** Tag the last non-skipped slide so print CSS can drop its
     *  break-after (see the @media print comment above — :last-child
     *  alone matches a hidden skipped slide). */
    _markLastVisible() {
      let last = null;
      this._slides.forEach((s) => {
        s.removeAttribute('data-deck-last-visible');
        if (!s.hasAttribute('data-deck-skip')) last = s;
      });
      if (last) last.setAttribute('data-deck-last-visible', '');
    }

    _loadNotes() {
      const tag = document.getElementById('speaker-notes');
      this._hasNotesTag = !!tag;
      if (!tag) { this._notes = []; return; }
      try {
        const parsed = JSON.parse(tag.textContent || '[]');
        if (Array.isArray(parsed)) this._notes = parsed;
      } catch (e) {
        console.warn('[deck-stage] Failed to parse #speaker-notes JSON:', e);
        this._notes = [];
      }
    }

    /** Parse a `#<int>` fragment (1-indexed) into a 0-based slide index, or
     *  -1 when the hash is absent/malformed or points outside the deck.
     *  Shared by _restoreIndex (load time) and _onHashChange (runtime). */
    _slideFromHash() {
      const h = (location.hash || '').match(/^#(\d+)$/);
      if (!h) return -1;
      const n = parseInt(h[1], 10) - 1;
      return n >= 0 && n < this._slides.length ? n : -1;
    }

    _restoreIndex() {
      // The host's ?slide= param is delivered as a #<int> hash (1-indexed) on
      // the iframe src. No hash → slide 1; the deck itself keeps no position
      // state across loads.
      const n = this._slideFromHash();
      if (n >= 0) this._index = n;
    }

    _onHashChange() {
      // Runtime counterpart to _restoreIndex, which only reads the fragment
      // once (at connectedCallback). A same-document fragment change — a deep
      // link edited in the address bar, or an automation/screenshot tool that
      // navigates to #N without a full reload — fires hashchange but never
      // re-runs connectedCallback, so honor it here too. The deck's own
      // navigation writes the fragment via history.replaceState, which does
      // NOT fire hashchange, so this can't feed back into a loop; the index
      // guard also makes a redundant fragment a no-op.
      const n = this._slideFromHash();
      if (n >= 0 && n !== this._index) this._go(n, 'api');
    }

    _applyIndex({ broadcast = true, reason = 'init' } = {}) {
      if (!this._slides.length) return;
      const prev = this._prevIndex == null ? -1 : this._prevIndex;
      const curr = this._index;
      // Keep the iframe's own hash in sync so an in-iframe location.reload()
      // (reload banner path in viewer-handle.ts) lands on the current slide,
      // not the stale deep-link hash from initial load.
      try { history.replaceState(null, '', '#' + (curr + 1)); } catch (e) {}
      this._slides.forEach((s, i) => {
        if (i === curr) s.setAttribute('data-deck-active', '');
        else s.removeAttribute('data-deck-active');
      });
      // Follow-scroll on every navigation (init deep-link, keyboard, click,
      // tap, external goTo) — the only time we *don't* want the rail to
      // track current is after a rail-internal mutation, where _renderRail
      // has already restored the user's scroll position and yanking back to
      // current would undo it.
      this._syncRail(reason !== 'mutation');
      this._syncNotesBody();

      if (broadcast) {
        // (1) Parent-window postMessage for speaker-notes renderers.
        try { window.parent.postMessage({ type: 'miaoda:deck:slide-changed', index: curr, total: this._slides.length, skipped: this._skippedIndices() }, '*'); } catch (e) {}

        // (2) In-page CustomEvent on the <deck-stage> element itself.
        //     Bubbles and composes out of shadow DOM so slide code can listen:
        //       document.querySelector('deck-stage').addEventListener('slidechange', e => {
        //         e.detail.index, e.detail.previousIndex, e.detail.total, e.detail.slide, e.detail.reason
        //       });
        const detail = {
          index: curr,
          previousIndex: prev,
          total: this._slides.length,
          slide: this._slides[curr] || null,
          previousSlide: prev >= 0 ? (this._slides[prev] || null) : null,
          reason: reason, // 'init' | 'keyboard' | 'click' | 'tap' | 'api'
        };
        this.dispatchEvent(new CustomEvent('slidechange', {
          detail,
          bubbles: true,
          composed: true,
        }));
      }

      if (this.hasAttribute('data-mob-ui')) this._syncMobStrip();

      this._prevIndex = curr;
    }

    // 移动端两种呈现互斥：竖排浏览（list）/ 旋转横屏播放（present）。
    // noscale / _snthumb（导出与演讲者缩略图捕获）必须保持桌面几何。
    _syncMobileMode() {
      const eligible = this._mobile && !this.hasAttribute('noscale') && !this._snthumb;
      this.toggleAttribute('data-mobile-list', eligible && !this._presenting);
      this.toggleAttribute('data-mobile-present', eligible && !!this._presenting);
    }

    _fitMobileList() {
      const dw = this.designWidth, dh = this.designHeight;
      const vw = window.innerWidth;
      this._mobLastVw = vw; // _onResize 高度变化跳过的基线
      const stage = this._canvas.parentElement;
      // 清掉桌面 _fit 写过的 inset 内联值
      if (stage) { stage.style.left = '0'; stage.style.right = '0'; stage.style.top = '0'; stage.style.bottom = '0'; }
      this.removeAttribute('data-windowed');
      const cardW = vw - 2 * MOB_MARGIN;
      const s = cardW / dw;
      const cardH = dh * s;
      const kids = Array.prototype.slice.call(this.children);
      const rules = [];
      const cards = [];
      let y = MOB_MARGIN;
      this._slides.forEach((slide, i) => {
        if (slide.hasAttribute('data-deck-skip')) return;
        const nth = kids.indexOf(slide) + 1;
        if (nth <= 0) return;
        rules.push(
          ':host([data-mobile-list]) ::slotted(:nth-child(' + nth + ')) {' +
          ' inset: auto !important; top: 0 !important; left: 0 !important;' +
          ' width: ' + dw + 'px !important; height: ' + dh + 'px !important;' +
          ' transform: translate(' + MOB_MARGIN + 'px, ' + y + 'px) scale(' + s + ') !important;' +
          // 圆角/描边随 scale 反向放大，落地后视觉为 8px / 0.5px
          ' border-radius: ' + (MOB_RADIUS / s) + 'px; overflow: hidden !important;' +
          ' border: ' + (0.5 / s) + 'px solid #DEE0E3; }'
        );
        cards.push({ slide, i, y, h: cardH });
        y += cardH + MOB_GAP;
      });
      // 竖排规则只作用于屏幕媒体——print 的 ::slotted 分页规则特异性更低，
      // 否则每页会带着列表位移/缩放进入打印流。
      this._mobStyle.textContent = '@media screen {\n' + rules.join('\n') + '\n}';
      this._canvas.style.transform = 'none';
      this._canvas.style.width = vw + 'px';
      this._canvas.style.height = Math.max(y - MOB_GAP + MOB_MARGIN, 0) + 'px';
      this._mobCards = cards;
      this._mobScale = s;

      // 徽标：每卡片左下角内 6px；宽度按位数 16/16/20。布局是确定性的，
      // 直接按 _mobCards 几何摆放，无需 IntersectionObserver。
      this._mobBadges.textContent = '';
      cards.forEach((c, ordinal) => {
        const b = document.createElement('div');
        b.className = 'mob-badge';
        const n = ordinal + 1;
        b.textContent = String(n);
        b.style.width = (String(n).length >= 3 ? 20 : 16) + 'px';
        b.style.left = (MOB_MARGIN + 6) + 'px';
        b.style.top = (c.y + c.h - 6 - 12) + 'px';
        this._mobBadges.appendChild(b);
      });

      this._mobScrollTo(this._index, false);
    }

    _onMobScroll() {
      if (!this.hasAttribute('data-mobile-list') || !this._mobCards || !this._mobCards.length) return;

      // 徽标显隐：程序化滚动也算"滚动"，两种来源都给可见性反馈。
      this.setAttribute('data-mob-scrolling', '');
      clearTimeout(this._mobScrollIdleTimer);
      this._mobScrollIdleTimer = setTimeout(
        () => this.removeAttribute('data-mob-scrolling'), MOB_BADGE_IDLE_MS);

      if (this._mobScrollRaf) return;
      this._mobScrollRaf = requestAnimationFrame(() => {
        this._mobScrollRaf = null;
        const stage = this._canvas.parentElement;
        const top = stage.scrollTop;
        const moved = this._mobLastTop == null ? Infinity : Math.abs(top - this._mobLastTop);
        this._mobLastTop = top; // 抑制期间也持续跟踪位置
        if (this._mobProgUntil && performance.now() < this._mobProgUntil) {
          // 程序化滚动事件流仍在到达：续期窗口，动画多长都盖得住；
          // 事件停止 120ms 后窗口自然过期，之后才恢复用户滚动反推。
          this._mobProgUntil = Math.max(this._mobProgUntil, performance.now() + 120);
          return;
        }
        if (moved < 1) return; // 无真实位移（合成事件/终点回调）不反推
        // 纯视口中心公式在卡片高 < 半视口时到不了首/末页（卡片高 ~= 0.5625 * 宽,
        // 竖屏视口高通常远大于宽的一半），两端改用 clamp，中段维持视口中心语义。
        const max = stage.scrollHeight - stage.clientHeight;
        let cur;
        if (max <= 0 || stage.scrollTop <= 1) {
          cur = this._mobCards[0];
        } else if (stage.scrollTop >= max - 1) {
          cur = this._mobCards[this._mobCards.length - 1];
        } else {
          const center = stage.scrollTop + stage.clientHeight / 2;
          cur = this._mobCards[0];
          for (const c of this._mobCards) {
            if (center >= c.y) cur = c; else break;
          }
        }
        if (cur.i !== this._index) {
          this._index = cur.i;
          // 广播走现有内核：hash 同步 + slide-changed + slidechange 事件。
          // data-deck-active 的切换在竖排 CSS 下无视觉副作用。
          this._applyIndex({ broadcast: true, reason: 'api' });
        }
      });
    }

    _mobScrollTo(i, smooth) {
      const card = (this._mobCards || []).find((c) => c.i === i);
      if (!card) return;
      const stage = this._canvas.parentElement;
      const top = Math.max(0, card.y - Math.max(0, (stage.clientHeight - card.h) / 2));
      // 程序化滚动期间抑制 _onMobScroll 的反推——goto/deep-link 已直接设定
      // _index，多张边缘卡片的居中目标会被 clamp 到同一个 scrollTop，
      // 反推无法区分它们对应哪一页，故导航直接落 index、反推只处理真实
      // 用户位移。
      this._mobProgUntil = performance.now() + (smooth ? 800 : 200);
      // 目标位置立为基线：即便这次 scrollTo 因目标已等于当前位置（如居中
      // 目标 clamp 到 0）而不产生真实 scroll 事件，_mobLastTop 也不会停留
      // 在 null——避免之后任意一次无位移事件被当成"首次滚动"误判有位移。
      this._mobLastTop = top;
      stage.scrollTo({ top, behavior: smooth ? 'smooth' : 'auto' });
    }

    _fitMobilePresent() {
      const stage = this._canvas.parentElement;
      if (stage) { stage.style.left = ''; stage.style.right = ''; stage.style.top = ''; stage.style.bottom = ''; }
      this.removeAttribute('data-windowed');
      this._canvas.style.width = this.designWidth + 'px';
      this._canvas.style.height = this.designHeight + 'px';
      // 物理竖屏：CSS 旋转 90°，stage 逻辑尺寸 = (innerHeight × innerWidth)；
      // 物理横屏（用户顺势转了手机）：不旋转，按真实视口常规适配。
      // CSS 侧由 @media (orientation) 同步分支。
      const portrait = window.innerHeight >= window.innerWidth;
      const lw = portrait ? window.innerHeight : window.innerWidth;
      const lh = portrait ? window.innerWidth : window.innerHeight;
      const s = Math.min(lw / this.designWidth, lh / this.designHeight);
      this._canvas.style.transform = 'scale(' + s + ')';
    }

    _railWidth() {
      // State-based, no offsetWidth: the first _fit() can run before the
      // rail has had layout on some load paths, and a 0 there paints the
      // slide full-width for one frame before the post-slotchange _fit()
      // corrects it.
      if (!this._railEnabled || !this._railVisible || this.hasAttribute('no-rail')
          || this.hasAttribute('noscale') || this._presenting || this._previewMode
          || NARROW_MQ.matches) return 0;
      return this._railPx || 0;
    }

    _fit() {
      if (!this._canvas) return;
      if (this.hasAttribute('data-mobile-list')) return this._fitMobileList();
      if (this.hasAttribute('data-mobile-present')) return this._fitMobilePresent();
      const stage = this._canvas.parentElement;
      // PPTX export sets noscale so the DOM capture sees authored-size
      // geometry — the scaled canvas is in shadow DOM, so the exporter's
      // resetTransformSelector can't reach .canvas.style.transform directly.
      if (this.hasAttribute('noscale')) {
        this._canvas.style.transform = 'none';
        this.removeAttribute('data-windowed');
        if (stage) { stage.style.left = '0'; stage.style.right = '0'; stage.style.top = '0'; stage.style.bottom = '0'; }
        return;
      }
      this._canvas.style.width = this.designWidth + 'px';
      this._canvas.style.height = this.designHeight + 'px';
      const rw = this._railWidth();
      const nh = this._notesHeight();
      // Windowed (20px margin + card shadow) in edit/preview; full-bleed while
      // presenting or inside presenter-thumbnail iframes (_snthumb).
      const windowed = !this._presenting && !this._snthumb;
      const m = windowed ? DECK_MARGIN : 0;
      this.toggleAttribute('data-windowed', windowed);
      // Explicit four-side inset shrinks the flex box to the safe region; the
      // design-size canvas centres in it, then scale() shrinks it around the
      // centre — so it stays inside the inset with equal margins on all sides.
      if (stage) {
        stage.style.left = (rw + m) + 'px';
        stage.style.right = m + 'px';
        stage.style.top = m + 'px';
        stage.style.bottom = (m + nh) + 'px';
      }
      // The notes dock spans the non-rail width; its left edge tracks the rail.
      if (this._notesPanel) this._notesPanel.style.left = (rw + DECK_MARGIN) + 'px';
      const vw = window.innerWidth - rw - 2 * m;
      const vh = window.innerHeight - nh - 2 * m;
      const s = Math.min(vw / this.designWidth, vh / this.designHeight);
      this._canvas.style.transform = `scale(${s})`;
    }

    _onResize() {
      // 竖排列表几何只依赖视口宽度。移动浏览器地址栏收起/展开只改高度，
      // 若照常全量 refit（重建规则/徽标 + 无条件回中当前页），用户滚动中
      // 会被可见地"拽"一下——纯高度变化直接跳过。宽度变化（旋转/分屏）
      // 照常走完整 refit。
      if (this.hasAttribute('data-mobile-list')) {
        if (this._mobLastVw === window.innerWidth) return;
        this._mobLastVw = window.innerWidth;
      }
      this._fit();
      // Crossing the narrow-viewport breakpoint reveals the rail — rerun the
      // thumbnail scale the same way _setRailWidth does.
      if (!this._scaleRaf) {
        this._scaleRaf = requestAnimationFrame(() => {
          this._scaleRaf = null;
          this._scaleThumbs();
        });
      }
    }

    _onMessage(e) {
      const d = e.data;
      if (d && d.type === 'miaoda:deck:presenting' && typeof d.on === 'boolean') {
        this._presenting = d.on;
        this._syncMobileMode();
        if (!d.on) this._toggleMobUi(false);
        this._syncRailHidden();
        this._syncNotesHidden();
        this._closeMenu();
        this._fit();
        this._scaleThumbs();
      }
      // Host's Preview segment (ViewerMode='none'): the rail's drag-reorder /
      // right-click skip-delete affordances are editing chrome, so hide it
      // while the user is just looking at the deck. Same hard-hide path as
      // presenting; independent of the user's _railVisible preference so
      // returning to Edit restores whatever they had.
      if (d && d.type === 'miaoda:deck:preview-mode' && typeof d.on === 'boolean') {
        if (d.on === this._previewMode) return;
        this._previewMode = d.on;
        this._syncRailHidden();
        this._syncNotesHidden();
        this._closeMenu();
        this._fit();
        this._scaleThumbs();
      }
      // Per-viewer show/hide, driven by the TweaksPanel's auto-injected
      // "Thumbnail rail" toggle (or any author script). Independent of
      // whether the Tweaks panel itself is open — closing the panel
      // doesn't change rail visibility. Persists alongside rail width.
      if (d && d.type === 'miaoda:deck:rail-visible' && typeof d.on === 'boolean') {
        if (d.on === this._railVisible) return;
        this._railVisible = d.on;
        try { localStorage.setItem('deck-stage.railVisible', d.on ? '1' : '0'); } catch (e) {}
        // Notify the parent so its toolbar state stays in sync.
        try { window.parent.postMessage({ type: 'miaoda:deck:rail-visible-changed', on: d.on }, '*'); } catch (e) {}
        // Arm the transition, commit it, then flip state — otherwise the
        // browser coalesces both writes and nothing animates on show.
        this.setAttribute('data-rail-anim', '');
        void (this._rail && this._rail.offsetHeight);
        this._syncRailHidden();
        this._fit();
        this._scaleThumbs();
        clearTimeout(this._railAnimTimer);
        this._railAnimTimer = setTimeout(() => this.removeAttribute('data-rail-anim'), 220);
      }
      if (d && d.type === 'miaoda:deck:rail-enabled') this._enableRail();
      if (d && d.type === 'miaoda:deck:goto' && typeof d.index === 'number') this._go(d.index | 0, 'api');
    }

    _syncRailHidden() {
      if (!this._rail) return;
      // data-presenting is the hard hide (display:none) for flag-off,
      // presentation mode, and the host's Preview segment — instant, no
      // transition. data-user-hidden is the soft hide (translateX(-100%))
      // for the viewer's rail toggle, so show/hide slides under
      // :host([data-rail-anim]).
      const hard = !this._railEnabled || this._presenting || this._previewMode;
      if (hard) this._rail.setAttribute('data-presenting', '');
      else this._rail.removeAttribute('data-presenting');
      if (!this._railVisible) this._rail.setAttribute('data-user-hidden', '');
      else this._rail.removeAttribute('data-user-hidden');
      // translateX hide leaves thumbs (tabIndex=0) in the tab order —
      // inert keeps them unfocusable while the rail is off-screen.
      this._rail.inert = hard || !this._railVisible;
    }

    _onTap(e) {
      // Touch-only for normal browse/edit — on desktop there, keyboard + the
      // overlay toolbar cover nav. But in presenting mode the overlay is hidden
      // and click-to-advance is the expected gesture (Keynote/PPT), so allow it
      // on fine pointers too.
      if (FINE_POINTER_MQ.matches && !this._presenting) return;
      // Only taps that land on the stage (slide content or letterbox); the
      // overlay / rail / menus are siblings with their own click handlers.
      const path = e.composedPath();
      if (!this._stage || !path.includes(this._stage)) return;
      // Let interactive slide content keep the tap. composedPath (not
      // e.target.closest) so we see through open shadow roots — a <button>
      // inside a slide-authored custom element retargets e.target to the
      // host but still appears in the composed path.
      if (e.defaultPrevented) return;
      for (const n of path) {
        if (n === this._stage) break;
        if (n.matches && n.matches(INTERACTIVE_SEL)) return;
      }
      // 竖排浏览是滚动列表，tap 不翻页（spec 决策 #5）
      if (this.hasAttribute('data-mobile-list')) return;
      e.preventDefault();
      if (this.hasAttribute('data-mobile-present')) {
        const zone = this._mobPresentZone(e);
        if (zone === 'menu') this._toggleMobUi();
        else {
          this._advance(zone === 'prev' ? -1 : 1, 'tap');
          if (this.hasAttribute('data-mob-ui')) this._toggleMobUi(true);
        }
        return;
      }
      const rw = this._railWidth();
      const mid = rw + (window.innerWidth - rw) / 2;
      this._advance(e.clientX < mid ? -1 : 1, 'tap');
    }

    // rotate(90deg) 后视觉横屏的 x 轴对应视口 y 轴。方向以真机/仿真实测为准：
    // 若实测发现 prev/next 颠倒，翻转此处的比较符（保持单点修改）。
    _mobPresentZone(e) {
      const rect = this._stage.getBoundingClientRect();
      // 物理竖屏 = stage 被 CSS 旋转 90°，视觉横屏 x 轴对应视口 y 轴；
      // 物理横屏 = 无旋转，直接用视口 x 轴。与 _fitMobilePresent 同分支。
      const portrait = window.innerHeight >= window.innerWidth;
      const x = portrait
        ? (e.clientY - rect.top) / Math.max(1, rect.height)
        : (e.clientX - rect.left) / Math.max(1, rect.width);
      return x < 1 / 3 ? 'prev' : x > 2 / 3 ? 'next' : 'menu';
    }

    _toggleMobUi(force) {
      const wasOpen = this.hasAttribute('data-mob-ui');
      const on = force != null ? !!force : !wasOpen;
      this.toggleAttribute('data-mob-ui', on);
      clearTimeout(this._mobUiTimer);
      if (!on) return;
      // 从隐藏态唤出才全量重建（吸收直编/重排/skip 变化——strip 是静态
      // 快照、不接 _liveObserver 刷新管线）；已展开时的调用（缩略图点击/
      // 翻页重置计时）只同步高亮，避免高频路径 O(页数) 重建。
      if (!wasOpen || !this._mobUi) {
        if (this._mobUi) { this._mobUi.remove(); this._mobUi = null; }
        this._buildMobUi();
      }
      this._syncMobStrip();
      this._mobUiTimer = setTimeout(() => this._toggleMobUi(false), MOB_UI_HIDE_MS);
    }

    _buildMobUi() {
      // 移动 viewer 语境 host 可能从不发 rail-enabled，author-CSS 快照
      // （_enableRail 才建）不存在时缩略图克隆会裸渲染——此处兜底建一次。
      // _snapshotAuthorCss 可重入且带 generation guard。
      if (!this._adoptedSheet && this._authorCss == null) this._snapshotAuthorCss();
      const ui = document.createElement('div');
      ui.className = 'mob-ui export-hidden';
      ui.setAttribute('data-miaoda-chrome', '');
      const back = document.createElement('button');
      back.className = 'mob-back';
      back.type = 'button';
      back.textContent = '‹ 演示模式';
      back.addEventListener('click', (e) => {
        e.stopPropagation();
        this._presenting = false;
        this._syncMobileMode();
        this._syncRailHidden();
        this._syncNotesHidden();
        this._fit();
        this._toggleMobUi(false);
        try { window.parent.postMessage({ type: 'miaoda:deck:presenting-dismissed' }, '*'); } catch (err) {}
      });
      const strip = document.createElement('div');
      strip.className = 'mob-strip';
      this._mobStripEntries = this._slides
        .map((slide, i) => ({ slide, i }))
        .filter((e2) => !e2.slide.hasAttribute('data-deck-skip'))
        .map((e2, ordinal) => {
          const t = document.createElement('div');
          t.className = 'mthumb';
          const frame = document.createElement('div');
          frame.className = 'mframe';
          const num = document.createElement('div');
          num.className = 'mnum';
          num.textContent = String(ordinal + 1);
          t.append(frame, num);
          t.addEventListener('click', (ev) => {
            ev.stopPropagation();
            this._go(e2.i, 'click');
            this._toggleMobUi(true); // 重置自动收起计时，保持控件层可见
          });
          strip.appendChild(t);
          // 复用 _materialize 的克隆管线（media 熄火、canvas 快照、custom
          // element 中性化）：它会自建 host/shadow 并直接挂进 entry.frame，
          // 所以这里不需要再手工搭 host——只需在其后把统一走 _thumbScale
          // （rail 宽度）算出的缩放，改写成本条带 132px 专属的缩放。
          const entry = { slide: e2.slide, frame, host: null, clone: null };
          this._materialize(entry);
          if (entry.clone) entry.clone.style.transform = 'scale(' + (132 / this.designWidth) + ')';
          return { el: t, i: e2.i };
        });
      // 控件层内任何点击不冒泡成翻页 tap
      ui.addEventListener('click', (e) => e.stopPropagation());
      ui.append(back, strip);
      this._stage.appendChild(ui);
      this._mobUi = ui;
    }

    _syncMobStrip() {
      (this._mobStripEntries || []).forEach((en) => {
        en.el.toggleAttribute('data-current', en.i === this._index);
        if (en.i === this._index) en.el.scrollIntoView({ block: 'nearest', inline: 'center' });
      });
    }

    _onKey(e) {
      // Ignore when the user is typing.
      const t = e.target;
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
      if (e.key === 'Escape' && this._menu && this._menu.hasAttribute('data-open')) {
        this._closeMenu();
        e.preventDefault();
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;

      const key = e.key;
      let handled = true;

      if (key === 'ArrowRight' || key === 'ArrowDown' || key === 'PageDown' || key === ' ' || key === 'Spacebar') {
        this._advance(1, 'keyboard');
      } else if (key === 'ArrowLeft' || key === 'ArrowUp' || key === 'PageUp') {
        this._advance(-1, 'keyboard');
      } else if (key === 'Home') {
        this._go(0, 'keyboard');
      } else if (key === 'End') {
        this._go(this._slides.length - 1, 'keyboard');
      } else if (key === 'r' || key === 'R') {
        this._go(0, 'keyboard');
      } else if (/^[0-9]$/.test(key)) {
        // 1..9 jump to that slide; 0 jumps to 10.
        const n = key === '0' ? 9 : parseInt(key, 10) - 1;
        if (n < this._slides.length) this._go(n, 'keyboard');
      } else {
        handled = false;
      }

      if (handled) {
        e.preventDefault();
      }
    }

    _go(i, reason = 'api') {
      if (!this._slides.length) return;
      const clamped = Math.max(0, Math.min(this._slides.length - 1, i));
      // 竖排浏览：导航直接落 index（与桌面语义一致），滚动只是视觉跟随；
      // 不再依赖 _onMobScroll 从滚动位置反推——多张边缘卡片的居中滚动目标
      // 会被 clamp 到同一个 scrollTop，反推无法区分它们是哪一页。
      if (this.hasAttribute('data-mobile-list')) {
        const changed = clamped !== this._index;
        this._index = clamped;
        // 同页 goto 不重复广播（与桌面分支语义一致），但仍需重新居中滚动。
        if (changed) this._applyIndex({ broadcast: true, reason });
        this._mobScrollTo(clamped, true);
        return;
      }
      if (clamped === this._index) return;
      this._index = clamped;
      this._applyIndex({ broadcast: true, reason });
    }

    /** Step forward/back skipping any slide marked data-deck-skip. Clamps at
     *  the ends (no-op past the first/last non-skipped slide). */
    _advance(dir, reason) {
      if (!this._slides.length) return;
      let i = this._index + dir;
      while (i >= 0 && i < this._slides.length && this._slides[i].hasAttribute('data-deck-skip')) {
        i += dir;
      }
      if (i < 0 || i >= this._slides.length) return;
      this._go(i, reason);
    }

    // ── Thumbnail rail ────────────────────────────────────────────────────
    //
    // Thumbs are keyed by slide element and reused across _renderRail()
    // calls, so a reorder/delete is an O(changed) DOM shuffle instead of an
    // O(N) teardown-and-re-clone. Each thumb starts as a lightweight shell
    // (num + empty frame); the clone is materialized lazily by an
    // IntersectionObserver when the frame scrolls into (or near) view, so
    // only visible-ish slides pay the clone + image-decode cost.

    _renderRail() {
      if (!this._rail || !this._railEnabled) { this._thumbs = []; return; }
      // FLIP: record each *materialized* thumb's top before the reconcile.
      // Off-screen (non-materialized) thumbs don't need the animation and
      // skipping their getBoundingClientRect saves a forced layout per
      // off-screen thumb on large decks.
      const prevTops = new Map();
      (this._thumbs || []).forEach(({ thumb, slide, host }) => {
        if (host) prevTops.set(slide, thumb.getBoundingClientRect().top);
      });
      const st = this._rail.scrollTop;

      // Reconcile: reuse thumbs that already exist for a slide, create
      // shells for new slides, drop thumbs for removed slides.
      const bySlide = new Map();
      (this._thumbs || []).forEach((t) => bySlide.set(t.slide, t));
      const next = [];
      this._slides.forEach((slide) => {
        let t = bySlide.get(slide);
        if (t) bySlide.delete(slide);
        else t = this._makeThumb(slide);
        next.push(t);
      });
      // Orphans — slides removed since last render.
      bySlide.forEach((t) => {
        if (this._railObserver) this._railObserver.unobserve(t.frame);
        t.thumb.remove();
      });
      // Put thumbs into document order to match _slides. insertBefore on
      // an already-correctly-placed node is a no-op, so this is cheap
      // when nothing moved.
      next.forEach((t, i) => {
        const want = t.thumb;
        const at = this._rail.children[i];
        if (at !== want) this._rail.insertBefore(want, at || null);
        t.i = i;
        t.num.textContent = String(i + 1);
        if (t.slide.hasAttribute('data-deck-skip')) t.thumb.setAttribute('data-skip', '');
        else t.thumb.removeAttribute('data-skip');
      });
      this._thumbs = next;

      this._rail.scrollTop = st;
      if (prevTops.size) {
        const moved = [];
        this._thumbs.forEach(({ thumb, slide }) => {
          const old = prevTops.get(slide);
          if (old == null) return;
          const dy = old - thumb.getBoundingClientRect().top;
          if (Math.abs(dy) < 1) return;
          thumb.style.transition = 'none';
          thumb.style.transform = `translateY(${dy}px)`;
          moved.push(thumb);
        });
        if (moved.length) {
          // Commit the inverted positions before flipping the transition
          // on — otherwise the browser coalesces both style writes and
          // nothing animates.
          void this._rail.offsetHeight;
          moved.forEach((t) => {
            t.style.transition = 'transform 180ms cubic-bezier(.2,.7,.3,1)';
            t.style.transform = '';
          });
          setTimeout(() => moved.forEach((t) => { t.style.transition = ''; }), 220);
        }
      }
      requestAnimationFrame(() => this._scaleThumbs());
      this._syncRail(false);
    }

    /** Create a lightweight thumb shell for one slide. The clone is
     *  materialized later by the IntersectionObserver. Event handlers
     *  look up the thumb's *current* index (via _thumbs.indexOf) so the
     *  same element can be reused across reorders. */
    _makeThumb(slide) {
      const thumb = document.createElement('div');
      thumb.className = 'thumb';
      thumb.tabIndex = 0;
      const num = document.createElement('div');
      num.className = 'num';
      const frame = document.createElement('div');
      frame.className = 'frame';
      const skipIcon = document.createElement('div');
      skipIcon.className = 'skip-icon';
      skipIcon.innerHTML = '<svg width="20" height="20" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M1.8447 1.82978C2.16723 1.50736 2.68943 1.51112 3.0119 1.8336L18.3385 17.1606C18.6638 17.4859 18.6637 18.0136 18.3384 18.3389C18.0131 18.6641 17.4855 18.6643 17.1602 18.339L1.83317 3.01234C1.51069 2.68986 1.50692 2.16767 1.82935 1.84514C1.83447 1.84002 1.83958 1.8349 1.8447 1.82978ZM3.33085 6.27753L6.68668 9.63337C6.63272 10.1199 6.68658 10.6123 6.84443 11.0757C7.00228 11.5391 7.26025 11.962 7.60001 12.3144C7.93976 12.6668 8.35296 12.94 8.81024 13.1147C9.26753 13.2894 9.75767 13.3612 10.2458 13.325L10.3667 13.3134L13.3529 16.2996C12.2833 16.8225 11.1604 17.0838 9.98626 17.0838C6.65746 17.0838 3.75395 14.9833 1.27539 10.7819C0.991215 10.3003 0.987158 9.70269 1.26629 9.21806C1.91897 8.08486 2.60738 7.10484 3.33085 6.27753ZM13.3333 10.0005C13.3334 9.54197 13.2389 9.08841 13.0556 8.66813C12.8724 8.24784 12.6045 7.86987 12.2686 7.55784C11.9327 7.24581 11.536 7.00644 11.1033 6.85468C10.6707 6.70293 10.2114 6.64205 9.75418 6.67587L9.63293 6.68712L6.6246 3.67878C7.67308 3.17422 8.82227 2.91381 9.98585 2.91712C13.3783 2.91712 16.2919 5.01573 18.7276 9.21295C19.0101 9.6999 19.0068 10.3018 18.7193 10.7858C18.0552 11.9035 17.3618 12.8724 16.6388 13.693L13.3133 10.3675C13.3267 10.2467 13.3333 10.1246 13.3333 10.0005Z" fill="#646A73"/></svg>';
      frame.append(skipIcon);
      thumb.append(num, frame);

      const entry = { thumb, num, frame, slide, clone: null, host: null, i: -1 };
      // entry.i is refreshed on every _renderRail reconcile pass, so
      // handlers read the thumb's current position without an O(N) scan.
      const idx = () => entry.i;

      thumb.addEventListener('click', () => this._go(idx(), 'click'));
      // ↑/↓ step through the rail when a thumb has focus. _go clamps at the
      // ends and _applyIndex→_syncRail scrolls the new current thumb into
      // view; we move focus to it (preventScroll — _syncRail already
      // scrolled) so a held key walks the whole list. stopPropagation keeps
      // this out of the window-level _onKey nav handler.
      thumb.addEventListener('keydown', (e) => {
        if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
        if (e.metaKey || e.ctrlKey || e.altKey) return;
        e.preventDefault();
        e.stopPropagation();
        this._go(idx() + (e.key === 'ArrowDown' ? 1 : -1), 'keyboard');
        const cur = this._thumbs && this._thumbs[this._index];
        if (cur) cur.thumb.focus({ preventScroll: true });
      });
      thumb.addEventListener('contextmenu', (e) => {
        e.preventDefault();
        this._openMenu(idx(), e.clientX, e.clientY);
      });
      thumb.draggable = true;
      thumb.addEventListener('dragstart', (e) => {
        this._dragFrom = idx();
        thumb.setAttribute('data-dragging', '');
        e.dataTransfer.effectAllowed = 'move';
        try { e.dataTransfer.setData('text/plain', String(this._dragFrom)); } catch (err) {}
        // Hand the browser a clean elevated card instead of its raw .thumb
        // bitmap; anchor it under the cursor at the grab point (no jump).
        try {
          const g = this._makeDragGhost(entry);
          if (g) {
            const r = frame.getBoundingClientRect();
            const gx = g.pad + Math.max(0, Math.min(g.w, e.clientX - r.left));
            const gy = g.pad + Math.max(0, Math.min(g.h, e.clientY - r.top));
            e.dataTransfer.setDragImage(g.el, gx, gy);
          }
        } catch (err) {}
      });
      thumb.addEventListener('dragend', () => {
        thumb.removeAttribute('data-dragging');
        this._clearDrop();
        this._dragFrom = null;
        if (this._dragGhost) { this._dragGhost.remove(); this._dragGhost = null; }
      });
      thumb.addEventListener('dragover', (e) => {
        if (this._dragFrom == null) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        const r = thumb.getBoundingClientRect();
        this._setDrop(idx(), e.clientY < r.top + r.height / 2 ? 'before' : 'after');
      });
      thumb.addEventListener('drop', (e) => {
        if (this._dragFrom == null) return;
        e.preventDefault();
        const i = idx();
        const r = thumb.getBoundingClientRect();
        let to = e.clientY >= r.top + r.height / 2 ? i + 1 : i;
        if (this._dragFrom < to) to--;
        const from = this._dragFrom;
        this._clearDrop();
        this._dragFrom = null;
        if (to !== from) this._moveSlide(from, to);
      });

      if (this._railObserver) this._railObserver.observe(frame);
      frame.__deckThumb = entry;
      return entry;
    }

    /** Build the floating drag-image for a rail thumb.
     *  The browser's default drag-image is a raw bitmap of the whole .thumb:
     *  it includes the number gutter and clips the current-slide's dark ring
     *  (which sits 2px outside the frame via outline-offset) at the bitmap
     *  edge. Instead we hand setDragImage a purpose-built card — just the
     *  slide frame, rounded, with a soft drop-shadow, transparent around it.
     *  The card lives inside a transparent padded wrapper because the bitmap
     *  crops to the element's border-box, so the shadow needs room inside the
     *  box or it clips too. Returns { el, pad, w, h } for the anchor math, or
     *  null if the frame has no size yet. */
    _makeDragGhost(entry) {
      if (this._dragGhost) { this._dragGhost.remove(); this._dragGhost = null; }
      this._materialize(entry);
      const w = entry.frame.offsetWidth, h = entry.frame.offsetHeight;
      if (!w || !h) return null;
      const pad = 36; // ≥ the shadow's blur + offset so it stays in the bitmap
      const wrap = document.createElement('div');
      wrap.style.cssText =
        'position:fixed;top:0;left:-100000px;pointer-events:none;' +
        'background:transparent;box-sizing:content-box;padding:' + pad + 'px;';
      const card = document.createElement('div');
      // Drop-shadow only — no hairline ring (the 0 0 0 1px shadow reads as a
      // border, which we don't want on the floating card).
      card.style.cssText =
        'position:relative;width:' + w + 'px;height:' + h + 'px;background:#fff;' +
        'border-radius:6px;overflow:hidden;' +
        'box-shadow:0 6px 10px 0 rgba(31, 35, 41, 0.20);cursor:grabbing;';
      const host = document.createElement('div');
      host.style.cssText = 'position:absolute;inset:0;';
      this._syncThumbHostAttrs(host);
      const sr = host.attachShadow({ mode: 'open' });
      if (this._adoptedSheet) sr.adoptedStyleSheets = [this._adoptedSheet];
      else {
        const st = document.createElement('style');
        st.textContent = this._authorCss || '';
        sr.appendChild(st);
      }
      if (entry.clone) sr.appendChild(entry.clone.cloneNode(true));
      card.appendChild(host);
      wrap.appendChild(card);
      (this.shadowRoot || document.body).appendChild(wrap);
      this._dragGhost = wrap;
      return { el: wrap, pad, w, h };
    }

    /** Lazily build the clone for a thumb that has scrolled into view. */
    _materialize(entry) {
      if (entry.host) return;
      const dw = this.designWidth, dh = this.designHeight;
      let clone = entry.slide.cloneNode(true);
      clone.removeAttribute('id');
      clone.removeAttribute('data-deck-active');
      clone.querySelectorAll('[id]').forEach((el) => el.removeAttribute('id'));
      // Neuter heavy media; replace <video> with its poster so the box
      // keeps a visual. <iframe>/<audio> become empty placeholders.
      clone.querySelectorAll('iframe, audio, object, embed').forEach((el) => {
        el.removeAttribute('src');
        el.removeAttribute('srcdoc');
        el.removeAttribute('data');
        el.innerHTML = '';
      });
      clone.querySelectorAll('video').forEach((el) => {
        if (!el.poster) { el.removeAttribute('src'); el.innerHTML = ''; return; }
        const img = document.createElement('img');
        img.src = el.poster;
        img.alt = '';
        img.style.cssText = el.style.cssText + ';object-fit:cover;width:100%;height:100%;';
        img.className = el.className;
        el.replaceWith(img);
      });
      // Canvas: cloneNode does not copy drawn pixels — snapshot each
      // original canvas into an <img> so ECharts / Chart.js / custom
      // drawings appear in the thumbnail instead of a blank rectangle.
      const origCanvases = entry.slide.querySelectorAll('canvas');
      const cloneCanvases = clone.querySelectorAll('canvas');
      cloneCanvases.forEach((el, ci) => {
        const orig = origCanvases[ci];
        if (!orig || !orig.width || !orig.height) return;
        try {
          const img = document.createElement('img');
          img.src = orig.toDataURL();
          img.alt = '';
          img.style.cssText = (el.getAttribute('style') || '') +
            ';width:' + orig.offsetWidth + 'px;height:' + orig.offsetHeight + 'px;';
          img.className = el.className;
          el.replaceWith(img);
        } catch (e) {}
      });
      // Images: defer decode and let the browser pick the smallest
      // srcset candidate for the ~140px thumb. Same-URL clones reuse the
      // slide's decoded bitmap (URL-keyed cache), so the remaining cost
      // is paint/composite — lazy+async keeps that off the main thread.
      clone.querySelectorAll('img').forEach((el) => {
        el.loading = 'lazy';
        el.decoding = 'async';
        if (el.srcset) el.sizes = (this._railPx || 192) + 'px';
      });
      // Custom elements inside the slide would have their
      // connectedCallback fire when the clone is appended. Replace them
      // with inert boxes so a component-heavy deck doesn't run N copies
      // of each component's mount logic in the rail. Children are
      // preserved so layout-wrapper elements (<my-column><h2>…</h2>)
      // still show their authored content; the querySelectorAll NodeList
      // is static, so nested custom elements in the moved subtree are
      // still visited on later iterations.
      const neuter = (el) => {
        const box = document.createElement('div');
        box.style.cssText = (el.getAttribute('style') || '') +
          ';background:rgba(0,0,0,0.06);border:1px dashed rgba(0,0,0,0.15);';
        box.className = el.className;
        // Preserve theming/i18n hooks so [data-*] / :lang() / [dir]
        // descendant selectors still match the neutered root.
        for (const a of el.attributes) {
          const n = a.name;
          if (n.startsWith('data-') || n.startsWith('aria-') ||
              n === 'lang' || n === 'dir' || n === 'role' || n === 'title') {
            box.setAttribute(n, a.value);
          }
        }
        while (el.firstChild) box.appendChild(el.firstChild);
        return box;
      };
      // querySelectorAll('*') returns descendants only — a custom-element
      // slide root (<my-slide>…</my-slide>) would slip through and upgrade
      // on append. Swap the root first.
      if (clone.tagName.includes('-')) clone = neuter(clone);
      clone.querySelectorAll('*').forEach((el) => {
        if (el.tagName.includes('-')) el.replaceWith(neuter(el));
      });
      clone.style.cssText += ';position:absolute;top:0;left:0;transform-origin:0 0;' +
        'pointer-events:none;width:' + dw + 'px;height:' + dh + 'px;' +
        'box-sizing:border-box;overflow:hidden;visibility:visible;opacity:1;';
      const host = document.createElement('div');
      host.style.cssText = 'position:absolute;inset:0;';
      this._syncThumbHostAttrs(host);
      const sr = host.attachShadow({ mode: 'open' });
      if (this._adoptedSheet) sr.adoptedStyleSheets = [this._adoptedSheet];
      else {
        const st = document.createElement('style');
        st.textContent = this._authorCss || '';
        sr.appendChild(st);
      }
      sr.appendChild(clone);
      entry.frame.appendChild(host);
      entry.host = host;
      entry.clone = clone;
      if (this._thumbScale) clone.style.transform = 'scale(' + this._thumbScale + ')';
      // Once materialized the IO callback is a no-op early-return —
      // unobserve so scroll doesn't keep firing it.
      if (this._railObserver) this._railObserver.unobserve(entry.frame);
    }

    /** Re-clone a single thumb (live-update path). No-op if the thumb
     *  hasn't been materialized yet — it'll pick up current content when
     *  it scrolls into view. */
    _refreshThumb(slide) {
      const entry = (this._thumbs || []).find((t) => t.slide === slide);
      if (!entry || !entry.host) return;
      entry.host.remove();
      entry.host = entry.clone = null;
      this._materialize(entry);
    }

    _scaleThumbs() {
      if (!this._thumbs || !this._thumbs.length) return;
      // Every frame is the same width; if it reads 0 the rail is
      // display:none (noscale / no-rail / presenting / print) — leave the
      // clones as-is and re-run when the rail is revealed.
      const fw = this._thumbs[0].frame.offsetWidth;
      if (!fw) return;
      this._thumbScale = fw / this.designWidth;
      this._thumbs.forEach(({ clone }) => {
        if (clone) clone.style.transform = 'scale(' + this._thumbScale + ')';
      });
    }

    /** Nearest reorder slot for a Y coordinate anywhere in the rail — used when
     *  the pointer is over a gap, where no thumb dragover fires. Returns
     *  { i, pos } matching _setDrop's contract, or null. Mirrors the thumb
     *  handler's midpoint test so the gap indicator lines up with the thumbs. */
    _railDropTarget(clientY) {
      const ts = this._thumbs;
      if (!ts || !ts.length) return null;
      for (let k = 0; k < ts.length; k++) {
        const r = ts[k].thumb.getBoundingClientRect();
        if (clientY < r.top + r.height / 2) return { i: ts[k].i, pos: 'before' };
        const next = ts[k + 1];
        if (!next) return { i: ts[k].i, pos: 'after' };
        const nr = next.thumb.getBoundingClientRect();
        if (clientY < nr.top + nr.height / 2) return { i: ts[k].i, pos: 'after' };
      }
      const last = ts[ts.length - 1];
      return { i: last.i, pos: 'after' };
    }

    _setDrop(i, where) {
      // dragover fires at pointer-event rate; touch only the previous
      // and new target rather than sweeping all N thumbs.
      const t = this._thumbs && this._thumbs[i];
      if (this._dropOn && this._dropOn !== t) {
        this._dropOn.thumb.removeAttribute('data-drop');
      }
      if (t) t.thumb.setAttribute('data-drop', where);
      this._dropOn = t || null;
      // Remembered so a drop that lands in a gap (rail handler) can commit to
      // the slot the indicator is showing without re-deriving before/after.
      this._dropWhere = t ? where : null;
    }

    _clearDrop() {
      if (this._dropOn) this._dropOn.thumb.removeAttribute('data-drop');
      this._dropOn = null;
      this._dropWhere = null;
    }

    _syncRail(follow) {
      if (!this._thumbs) return;
      this._thumbs.forEach(({ thumb }, i) => {
        if (i === this._index) {
          thumb.setAttribute('data-current', '');
          if (follow && typeof thumb.scrollIntoView === 'function') {
            thumb.scrollIntoView({ block: 'nearest' });
          }
        } else {
          thumb.removeAttribute('data-current');
        }
      });
    }

    _openMenu(i, x, y) {
      if (!this._menu) return;
      this._menuIndex = i;
      const slide = this._slides[i];
      const skip = slide && slide.hasAttribute('data-deck-skip');
      this._menu.querySelector('[data-act="skip"]').textContent = skip ? '取消隐藏' : '隐藏此页';
      this._menu.querySelector('[data-act="up"]').disabled = i <= 0;
      this._menu.querySelector('[data-act="down"]').disabled = i >= this._slides.length - 1;
      this._menu.querySelector('[data-act="delete"]').disabled = this._slides.length <= 1;
      // Place, then clamp to viewport after it's measurable.
      this._menu.style.left = x + 'px';
      this._menu.style.top = y + 'px';
      this._menu.setAttribute('data-open', '');
      const r = this._menu.getBoundingClientRect();
      const nx = Math.min(x, window.innerWidth - r.width - 4);
      const ny = Math.min(y, window.innerHeight - r.height - 4);
      this._menu.style.left = Math.max(4, nx) + 'px';
      this._menu.style.top = Math.max(4, ny) + 'px';
    }

    _closeMenu() {
      if (this._menu) this._menu.removeAttribute('data-open');
      this._menuIndex = -1;
    }

    _emitDeckChange(detail) {
      // A structural edit carries the (possibly reindexed) notes array so the
      // host writes deck-stage + speaker-notes in ONE file write. Cancel any
      // pending text-edit notes-changed so the two never race two writes at
      // the same file (etag conflict would drop the second).
      clearTimeout(this._notesPersistTimer);
      this.dispatchEvent(new CustomEvent('deckchange', {
        detail, bubbles: true, composed: true,
      }));
      // Forward to parent so the host can persist mutations (the CustomEvent
      // cannot cross iframe boundaries). detail.slide is an Element and can't
      // survive structured-clone, so only the scalars cross.
      try {
        window.parent.postMessage({
          type: 'miaoda:deck:deck-changed',
          action: detail.action, from: detail.from, to: detail.to,
          html: this.outerHTML,
          notes: this._notesForEmit(),
        }, '*');
      } catch (e) {}
    }

    _deleteSlide(i) {
      const slide = this._slides[i];
      if (!slide || this._slides.length <= 1) return;
      const wasCurrent = i === this._index;
      if (i < this._index || (wasCurrent && i === this._slides.length - 1)) this._index--;
      this._squelchSlotChange = true;
      slide.remove();
      // Keep the per-slide-indexed notes array aligned with the deck.
      if (Array.isArray(this._notes)) this._notes.splice(i, 1);
      this._emitDeckChange({ action: 'delete', from: i, slide });
      this._collectSlides();
      this._applyIndex({ broadcast: true, reason: 'mutation' });
    }

    _toggleSkip(i) {
      const slide = this._slides[i];
      if (!slide) return;
      const on = !slide.hasAttribute('data-deck-skip');
      if (on) slide.setAttribute('data-deck-skip', '');
      else slide.removeAttribute('data-deck-skip');
      if (this._thumbs && this._thumbs[i]) {
        if (on) this._thumbs[i].thumb.setAttribute('data-skip', '');
        else this._thumbs[i].thumb.removeAttribute('data-skip');
      }
      this._markLastVisible();
      this._emitDeckChange({ action: on ? 'skip' : 'unskip', from: i, slide });
      // Re-broadcast so the presenter popup's prev/next thumbnails re-pick
      // the nearest non-skipped slide without waiting for a nav event.
      try { window.parent.postMessage({ type: 'miaoda:deck:slide-changed', index: this._index, total: this._slides.length, skipped: this._skippedIndices() }, '*'); } catch (e) {}
    }

    _skippedIndices() {
      const out = [];
      for (let i = 0; i < this._slides.length; i++) {
        if (this._slides[i].hasAttribute('data-deck-skip')) out.push(i);
      }
      return out;
    }

    _moveSlide(i, j) {
      if (j < 0 || j >= this._slides.length || j === i) return;
      const slide = this._slides[i];
      const ref = j < i ? this._slides[j] : this._slides[j].nextSibling;
      // Track the active slide across the reorder so the same content
      // stays on screen.
      const cur = this._index;
      if (cur === i) this._index = j;
      else if (i < cur && j >= cur) this._index = cur - 1;
      else if (i > cur && j <= cur) this._index = cur + 1;
      this._squelchSlotChange = true;
      this.insertBefore(slide, ref);
      // Move the note entry in lockstep so notes stay aligned to slides.
      if (Array.isArray(this._notes)) {
        const [n] = this._notes.splice(i, 1);
        this._notes.splice(j, 0, n === undefined ? '' : n);
      }
      this._emitDeckChange({ action: 'move', from: i, to: j, slide });
      this._collectSlides();
      this._applyIndex({ broadcast: true, reason: 'mutation' });
    }

    // Public API ------------------------------------------------------------

    /** Current slide index (0-based). */
    get index() { return this._index; }
    /** Total slide count. */
    get length() { return this._slides.length; }
    /** Programmatically navigate. */
    goTo(i) { this._go(i, 'api'); }
    next() { this._advance(1, 'api'); }
    prev() { this._advance(-1, 'api'); }
    reset() { this._go(0, 'api'); }
  }

  if (!customElements.get('deck-stage')) {
    customElements.define('deck-stage', DeckStage);
  }
})();
