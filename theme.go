package ui

// Styles is the stylesheet the components here render against: the --ui-*
// design tokens and the rules for every component's classes (.vk-modal,
// .vk-alert, .vk-toast, .vk-table, .vk-datatable). It is plain CSS — no build
// step, no Tailwind pass — so an app ships it by putting StyleTag in its
// layout head, or by serving it as a static file. Tokens have light defaults
// and dark values under prefers-color-scheme (overridable with a data-theme
// attribute on the html element), so the components follow the page's theme.
const Styles = `:root{
  --ui-bg:#ffffff; --ui-fg:#0f172a; --ui-muted:#64748b; --ui-subtle:#f1f5f9;
  --ui-border:#e2e8f0; --ui-ring:#94a3b8; --ui-overlay:rgba(15,23,42,.45);
  --ui-primary:#2563eb; --ui-primary-fg:#ffffff;
  --ui-info:#0ea5e9; --ui-success:#16a34a; --ui-warning:#d97706; --ui-danger:#dc2626;
  --ui-info-bg:#f0f9ff; --ui-success-bg:#f0fdf4; --ui-warning-bg:#fffbeb; --ui-danger-bg:#fef2f2;
  --ui-radius:.5rem; --ui-shadow:0 10px 30px -10px rgba(15,23,42,.35);
}
@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){
  --ui-bg:#0f172a; --ui-fg:#e2e8f0; --ui-muted:#94a3b8; --ui-subtle:#1e293b;
  --ui-border:#334155; --ui-ring:#64748b; --ui-overlay:rgba(0,0,0,.6);
  --ui-primary:#3b82f6; --ui-primary-fg:#ffffff;
  --ui-info-bg:#082f49; --ui-success-bg:#052e16; --ui-warning-bg:#451a03; --ui-danger-bg:#450a0a;
}}
:root[data-theme="dark"]{
  --ui-bg:#0f172a; --ui-fg:#e2e8f0; --ui-muted:#94a3b8; --ui-subtle:#1e293b;
  --ui-border:#334155; --ui-ring:#64748b; --ui-overlay:rgba(0,0,0,.6);
  --ui-primary:#3b82f6; --ui-primary-fg:#ffffff;
  --ui-info-bg:#082f49; --ui-success-bg:#052e16; --ui-warning-bg:#451a03; --ui-danger-bg:#450a0a;
}

/* Modal */
.vk-modal-root{position:fixed;inset:0;z-index:50;display:flex;align-items:center;justify-content:center;padding:1rem}
.vk-modal-backdrop{position:absolute;inset:0;background:var(--ui-overlay)}
.vk-modal{position:relative;z-index:1;display:flex;flex-direction:column;width:100%;max-width:32rem;max-height:calc(100vh - 2rem);
  overflow:auto;background:var(--ui-bg);color:var(--ui-fg);border:1px solid var(--ui-border);border-radius:var(--ui-radius);box-shadow:var(--ui-shadow)}
.vk-modal--sm{max-width:24rem} .vk-modal--lg{max-width:48rem}
.vk-modal__header{display:flex;align-items:center;justify-content:space-between;gap:1rem;padding:1rem 1.25rem;border-bottom:1px solid var(--ui-border)}
.vk-modal__title{margin:0;font-size:1.05rem;font-weight:600}
.vk-modal__body{padding:1.25rem;overflow:auto}
.vk-modal__footer{display:flex;gap:.5rem;justify-content:flex-end;padding:1rem 1.25rem;border-top:1px solid var(--ui-border)}
.vk-modal__close{appearance:none;border:0;background:transparent;color:var(--ui-muted);font-size:1.25rem;line-height:1;cursor:pointer;padding:.25rem}
.vk-modal__close:hover{color:var(--ui-fg)}

/* Alert */
.vk-alert{display:flex;gap:.75rem;align-items:flex-start;padding:.75rem 1rem;border:1px solid var(--ui-border);
  border-left-width:4px;border-radius:var(--ui-radius);background:var(--ui-subtle);color:var(--ui-fg)}
.vk-alert__body{flex:1;min-width:0}
.vk-alert__title{font-weight:600;margin:0 0 .15rem}
.vk-alert__close{appearance:none;border:0;background:transparent;color:inherit;opacity:.6;cursor:pointer;font-size:1.1rem;line-height:1}
.vk-alert__close:hover{opacity:1}
.vk-alert--info{border-left-color:var(--ui-info);background:var(--ui-info-bg)}
.vk-alert--success{border-left-color:var(--ui-success);background:var(--ui-success-bg)}
.vk-alert--warning{border-left-color:var(--ui-warning);background:var(--ui-warning-bg)}
.vk-alert--error{border-left-color:var(--ui-danger);background:var(--ui-danger-bg)}

/* Toasts */
.vk-toasts{position:fixed;z-index:60;display:flex;flex-direction:column;gap:.5rem;padding:1rem;max-width:min(24rem,100vw);pointer-events:none}
.vk-toasts--top-right{top:0;right:0} .vk-toasts--top-left{top:0;left:0}
.vk-toasts--bottom-right{bottom:0;right:0;flex-direction:column-reverse}
.vk-toasts--bottom-left{bottom:0;left:0;flex-direction:column-reverse}
.vk-toast{pointer-events:auto;display:flex;gap:.75rem;align-items:flex-start;padding:.75rem 1rem;border:1px solid var(--ui-border);
  border-left-width:4px;border-radius:var(--ui-radius);background:var(--ui-bg);color:var(--ui-fg);box-shadow:var(--ui-shadow)}
.vk-toast__body{flex:1;min-width:0}
.vk-toast__title{font-weight:600;margin:0 0 .15rem}
.vk-toast__close{appearance:none;border:0;background:transparent;color:inherit;opacity:.6;cursor:pointer;font-size:1.1rem;line-height:1}
.vk-toast__close:hover{opacity:1}
.vk-toast--info{border-left-color:var(--ui-info)} .vk-toast--success{border-left-color:var(--ui-success)}
.vk-toast--warning{border-left-color:var(--ui-warning)} .vk-toast--error{border-left-color:var(--ui-danger)}
.vk-toast[style*="--vk-toast-life"]{animation:vk-toast-out .4s var(--vk-toast-life) forwards}
@keyframes vk-toast-out{to{opacity:0;transform:translateY(-.25rem);visibility:hidden}}
@media (prefers-reduced-motion:reduce){.vk-toast{animation:none}}

/* Table + DataTable */
.vk-table-wrap{width:100%;overflow-x:auto}
.vk-table{width:100%;border-collapse:collapse;color:var(--ui-fg);font-size:.925rem}
.vk-table th,.vk-table td{padding:.6rem .75rem;text-align:left;border-bottom:1px solid var(--ui-border);vertical-align:middle}
.vk-table thead th{color:var(--ui-muted);font-weight:600;font-size:.8rem;text-transform:uppercase;letter-spacing:.03em}
.vk-table tbody tr:hover{background:var(--ui-subtle)}
.vk-table td.vk-col--right,.vk-table th.vk-col--right{text-align:right}
.vk-table td.vk-col--center,.vk-table th.vk-col--center{text-align:center}
.vk-table tr.vk-row--link{cursor:pointer}
.vk-table__empty{padding:2rem;text-align:center;color:var(--ui-muted)}
.vk-sort{appearance:none;border:0;background:transparent;color:inherit;font:inherit;font-weight:600;cursor:pointer;display:inline-flex;gap:.3rem;align-items:center;text-transform:inherit;letter-spacing:inherit}
.vk-sort__ind{color:var(--ui-primary)}
.vk-datatable{display:flex;flex-direction:column;gap:.75rem}
.vk-datatable__bar{display:flex;gap:.75rem;align-items:center;flex-wrap:wrap;justify-content:space-between}
.vk-datatable__search{flex:1;min-width:12rem;max-width:20rem;padding:.45rem .6rem;border:1px solid var(--ui-border);border-radius:var(--ui-radius);background:var(--ui-bg);color:var(--ui-fg)}
.vk-datatable__foot{display:flex;gap:.75rem;align-items:center;justify-content:space-between;flex-wrap:wrap;color:var(--ui-muted);font-size:.875rem}
.vk-pager{display:flex;gap:.25rem;align-items:center}
.vk-pager button,.vk-pager a{appearance:none;cursor:pointer;min-width:2rem;padding:.35rem .55rem;border:1px solid var(--ui-border);border-radius:var(--ui-radius);background:var(--ui-bg);color:var(--ui-fg);text-decoration:none;text-align:center;font:inherit}
.vk-pager button:hover:not(:disabled),.vk-pager a:hover{border-color:var(--ui-ring)}
.vk-pager button:disabled{opacity:.5;cursor:default}
.vk-pager [aria-current="page"]{background:var(--ui-primary);color:var(--ui-primary-fg);border-color:var(--ui-primary)}
@media (max-width:640px){
  .vk-table thead th{font-size:.72rem}
  .vk-datatable__bar,.vk-datatable__foot{gap:.5rem}
}`

// StyleTag is a <style> element holding Styles, to place in a page's head so
// the components are themed:
//
//	func layout(page ui.Node) ui.Node {
//		return ui.El("html", nil, ui.El("head", nil, ui.StyleTag()), ui.El("body", nil, page))
//	}
func StyleTag() Node { return El("style", []Attr{{"data-vk-ui", true}}, RawHTML(Styles)) }
