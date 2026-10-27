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
.vk-datatable__bar-left{display:flex;align-items:center;gap:.75rem;flex-wrap:wrap}
.vk-datatable__bar-right{display:flex;align-items:center;gap:.5rem;margin-left:auto}
.vk-datatable__searchbox{position:relative;display:inline-flex;align-items:center;flex:1;min-width:12rem;max-width:22rem}
.vk-datatable__searchicon{position:absolute;left:.6rem;color:var(--ui-muted);pointer-events:none}
.vk-datatable__search{width:100%;padding-left:2rem}
.vk-sort__ind{color:var(--ui-muted)}
.vk-sort__ind--active{color:var(--ui-primary)}

/* Icon + spinner */
.vk-icon{display:inline-block;vertical-align:-.125em;width:1em;height:1em;flex:none}
.vk-icon--spin{animation:vk-spin .8s linear infinite}
.vk-spinner{display:inline-block;width:1em;height:1em;border:2px solid currentColor;border-right-color:transparent;border-radius:50%;animation:vk-spin .6s linear infinite}
@keyframes vk-spin{to{transform:rotate(360deg)}}

/* Button */
.vk-btn{--_fg:var(--ui-fg);--_bg:var(--ui-subtle);--_bd:var(--ui-border);display:inline-flex;align-items:center;justify-content:center;gap:.4rem;
  font:inherit;font-weight:600;font-size:.9rem;line-height:1.2;padding:.5rem .9rem;border:1px solid var(--_bd);border-radius:var(--ui-radius);
  background:var(--_bg);color:var(--_fg);cursor:pointer;text-decoration:none;white-space:nowrap;-webkit-appearance:none;appearance:none;transition:filter .12s,background .12s,border-color .12s}
.vk-btn:hover:not(:disabled):not([aria-disabled="true"]){filter:brightness(.97)}
.vk-btn:focus-visible{outline:2px solid var(--ui-ring);outline-offset:2px}
.vk-btn:disabled,.vk-btn[aria-disabled="true"]{opacity:.55;cursor:default}
.vk-btn--sm{padding:.3rem .6rem;font-size:.8rem}
.vk-btn--lg{padding:.65rem 1.2rem;font-size:1rem}
.vk-btn--icon{padding:.45rem;aspect-ratio:1}
.vk-btn--primary{--_bg:var(--ui-primary);--_fg:var(--ui-primary-fg);--_bd:var(--ui-primary)}
.vk-btn--success{--_bg:var(--ui-success);--_fg:#fff;--_bd:var(--ui-success)}
.vk-btn--danger{--_bg:var(--ui-danger);--_fg:#fff;--_bd:var(--ui-danger)}
.vk-btn--warning{--_bg:var(--ui-warning);--_fg:#fff;--_bd:var(--ui-warning)}
.vk-btn--info{--_bg:var(--ui-info);--_fg:#fff;--_bd:var(--ui-info)}
.vk-btn--ghost{--_bg:transparent;--_bd:transparent}
.vk-btn--ghost:hover:not(:disabled){background:var(--ui-subtle)}
.vk-btn__icon{display:inline-flex;align-items:center;font-size:1.05em}
.vk-btn[aria-busy="true"]{pointer-events:none}
.vk-btn[aria-busy="true"] .vk-btn__icon,.vk-form[aria-busy="true"] .vk-btn--submit .vk-btn__icon{display:none}
.vk-btn[aria-busy="true"]::before,.vk-form[aria-busy="true"] .vk-btn--submit::before{content:"";width:1em;height:1em;margin-right:.1rem;border:2px solid currentColor;border-right-color:transparent;border-radius:50%;animation:vk-spin .6s linear infinite}
.vk-form[aria-busy="true"] .vk-btn--submit{opacity:.85}

/* Actions */
.vk-actions{display:flex;gap:.5rem;align-items:center;justify-content:flex-end;flex-wrap:wrap}

/* Card */
.vk-card{background:var(--ui-bg);border:1px solid var(--ui-border);border-radius:var(--ui-radius);box-shadow:0 1px 2px rgba(15,23,42,.04)}
.vk-card__header{display:flex;align-items:center;gap:1rem;padding:.9rem 1.15rem;border-bottom:1px solid var(--ui-border)}
.vk-card__title{font-weight:600;font-size:1rem;display:flex;align-items:center;gap:.5rem}
.vk-card__header-actions{margin-left:auto;display:flex;align-items:center;gap:.5rem}
.vk-card__body{padding:1.15rem}
.vk-card__body--flush{padding:0}

/* Tile */
.vk-tile-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:1rem}
.vk-tile{display:flex;align-items:center;gap:.9rem;padding:1rem 1.15rem;background:var(--ui-bg);border:1px solid var(--ui-border);border-radius:var(--ui-radius)}
.vk-tile__icon{display:inline-flex;align-items:center;justify-content:center;width:2.6rem;height:2.6rem;border-radius:50%;font-size:1.3rem;flex:none;
  background:color-mix(in srgb,var(--ui-primary) 12%,transparent);color:var(--ui-primary)}
.vk-tile__icon--success{background:color-mix(in srgb,var(--ui-success) 14%,transparent);color:var(--ui-success)}
.vk-tile__icon--danger{background:color-mix(in srgb,var(--ui-danger) 14%,transparent);color:var(--ui-danger)}
.vk-tile__icon--warning{background:color-mix(in srgb,var(--ui-warning) 14%,transparent);color:var(--ui-warning)}
.vk-tile__icon--info{background:color-mix(in srgb,var(--ui-info) 14%,transparent);color:var(--ui-info)}
.vk-tile__value{font-size:1.5rem;font-weight:700;line-height:1.1}
.vk-tile__label{color:var(--ui-muted);font-size:.85rem;margin-top:.1rem}

/* Badge */
.vk-badge{display:inline-flex;align-items:center;gap:.3rem;padding:.15rem .5rem;font-size:.75rem;font-weight:600;line-height:1.4;border-radius:999px;background:var(--ui-subtle);color:var(--ui-fg)}
.vk-badge--primary{background:var(--ui-primary);color:var(--ui-primary-fg)}
.vk-badge--success{background:var(--ui-success);color:#fff}
.vk-badge--danger{background:var(--ui-danger);color:#fff}
.vk-badge--warning{background:var(--ui-warning);color:#fff}
.vk-badge--info{background:var(--ui-info);color:#fff}
.vk-badge--soft.vk-badge--primary{color:var(--ui-primary);background:color-mix(in srgb,var(--ui-primary) 14%,transparent)}
.vk-badge--soft.vk-badge--success{color:var(--ui-success);background:color-mix(in srgb,var(--ui-success) 14%,transparent)}
.vk-badge--soft.vk-badge--danger{color:var(--ui-danger);background:color-mix(in srgb,var(--ui-danger) 14%,transparent)}
.vk-badge--soft.vk-badge--warning{color:var(--ui-warning);background:color-mix(in srgb,var(--ui-warning) 14%,transparent)}
.vk-badge--soft.vk-badge--info{color:var(--ui-info);background:color-mix(in srgb,var(--ui-info) 14%,transparent)}

/* Tabs */
.vk-tabs{display:flex;gap:.25rem;border-bottom:1px solid var(--ui-border);margin-bottom:1rem;overflow-x:auto;scrollbar-width:none;-ms-overflow-style:none}
.vk-tabs::-webkit-scrollbar{width:0;height:0;display:none}
.vk-tab{display:inline-flex;align-items:center;gap:.4rem;padding:.6rem .9rem;border:0;border-bottom:2px solid transparent;background:transparent;color:var(--ui-muted);
  font:inherit;font-weight:600;font-size:.9rem;cursor:pointer;text-decoration:none;white-space:nowrap;margin-bottom:-1px}
.vk-tab:hover{color:var(--ui-fg)}
.vk-tab--active{color:var(--ui-primary);border-bottom-color:var(--ui-primary)}
.vk-tab:focus-visible{outline:2px solid var(--ui-ring);outline-offset:-2px}
.vk-tab__count{display:inline-flex;align-items:center;justify-content:center;min-width:1.4em;padding:0 .35em;font-size:.72rem;border-radius:999px;
  background:color-mix(in srgb,var(--ui-primary) 14%,transparent);color:var(--ui-primary)}

/* Form + fields */
.vk-form{display:flex;flex-direction:column;gap:1rem}
.vk-field{display:flex;flex-direction:column;gap:.35rem}
.vk-field__label{font-weight:600;font-size:.85rem}
.vk-field__req{color:var(--ui-danger);margin-left:.15rem}
.vk-input,.vk-select,.vk-textarea{width:100%;font:inherit;font-size:.9rem;padding:.5rem .65rem;border:1px solid var(--ui-border);border-radius:var(--ui-radius);background:var(--ui-bg);color:var(--ui-fg)}
.vk-input,.vk-textarea{-webkit-appearance:none;appearance:none}
.vk-textarea{resize:vertical;min-height:4.5rem}
.vk-input::placeholder,.vk-textarea::placeholder{color:var(--ui-muted)}
.vk-input:focus,.vk-select:focus,.vk-textarea:focus{outline:2px solid var(--ui-ring);outline-offset:-1px;border-color:var(--ui-ring)}
.vk-input:disabled,.vk-select:disabled,.vk-textarea:disabled{opacity:.6;cursor:not-allowed}
.vk-input--invalid,.vk-select--invalid,.vk-textarea--invalid{border-color:var(--ui-danger)}
.vk-input--invalid:focus,.vk-select--invalid:focus,.vk-textarea--invalid:focus{outline-color:var(--ui-danger);border-color:var(--ui-danger)}
.vk-field__help{font-size:.8rem;color:var(--ui-muted)}
.vk-field__error{font-size:.8rem;color:var(--ui-danger)}
.vk-check{display:inline-flex;align-items:center;gap:.5rem;cursor:pointer;font-size:.9rem}
.vk-check__box{width:1rem;height:1rem;accent-color:var(--ui-primary);flex:none}
.vk-field__marker{font-weight:400;color:var(--ui-muted)}
.vk-field--invalid{border-left:4px solid var(--ui-danger);padding-left:.75rem}
.vk-field__error{font-weight:600}
.vk-field__reveal{align-self:flex-start;font:inherit;font-size:.8rem;padding:.2rem .5rem;border:1px solid var(--ui-border);border-radius:var(--ui-radius);background:var(--ui-bg);color:var(--ui-fg);cursor:pointer}
.vk-field__reveal[aria-pressed="true"]{background:color-mix(in srgb,var(--ui-primary) 12%,transparent)}
.vk-field__ok{color:var(--ui-success);font-weight:700;font-size:.9rem}
.vk-field__update{align-self:flex-start}
.vk-fieldset{border:0;margin:0;padding:0;display:flex;flex-direction:column;gap:.5rem;min-width:0}
.vk-fieldset__legend{font-weight:600;font-size:.9rem;padding:0;margin-bottom:.25rem}
.vk-choices{display:flex;flex-direction:column;gap:.4rem}
.vk-summary{border:3px solid var(--ui-danger);border-radius:var(--ui-radius);padding:.9rem 1rem}
.vk-summary:focus{outline:3px solid var(--ui-ring);outline-offset:2px}
.vk-summary__title{margin:0 0 .5rem;font-size:1.05rem}
.vk-summary__list{margin:0;padding-left:1.1rem;display:flex;flex-direction:column;gap:.25rem}
.vk-summary__list a{color:var(--ui-danger);font-weight:600}
.vk-sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}

/* Table additions */
.vk-table--flush{border:0}
.vk-col--select,.vk-col--index{width:1px;white-space:nowrap}
.vk-col--index{color:var(--ui-muted)}
.vk-table tbody tr.vk-row--selected{background:color-mix(in srgb,var(--ui-primary) 8%,transparent)}
.vk-empty{display:flex;flex-direction:column;align-items:center;gap:.5rem;color:var(--ui-muted)}
.vk-empty__icon{font-size:2rem;opacity:.65}
.vk-empty__msg{font-size:.9rem}

/* Modal additions */
.vk-modal--xl{max-width:64rem}
.vk-modal__header{justify-content:flex-start;gap:.75rem}
.vk-modal__icon{display:inline-flex;align-items:center;justify-content:center;width:2.2rem;height:2.2rem;border-radius:var(--ui-radius);font-size:1.2rem;flex:none;
  background:color-mix(in srgb,var(--ui-primary) 12%,transparent);color:var(--ui-primary)}
.vk-modal__heading{flex:1;min-width:0}
.vk-modal__subtitle{margin:.15rem 0 0;color:var(--ui-muted);font-size:.85rem}
.vk-modal__close .vk-icon{font-size:1.15rem}

@media (prefers-reduced-motion:reduce){.vk-spinner,.vk-icon--spin,.vk-btn[aria-busy="true"]::before,.vk-form[aria-busy="true"] .vk-btn--submit::before{animation-duration:1.4s}}
@media (max-width:960px){.vk-tile-grid{grid-template-columns:repeat(2,1fr)}}
@media (max-width:640px){
  .vk-table thead th{font-size:.72rem}
  .vk-datatable__bar,.vk-datatable__foot{gap:.5rem}
  .vk-datatable__bar{flex-direction:column;align-items:stretch}
  .vk-datatable__bar-right{margin-left:0}
  .vk-tile-grid{grid-template-columns:1fr}
}
.vk-img{background-color:#e2e8f0;background-image:linear-gradient(90deg,transparent,rgb(255 255 255/.6),transparent);background-size:200% 100%;background-repeat:no-repeat;animation:vk-img-shimmer 1.3s ease infinite}
.vk-img--plain{background:none;animation:none}
@keyframes vk-img-shimmer{0%{background-position:200% 0}100%{background-position:-200% 0}}
.vk-img-wrap{position:relative;overflow:hidden}
.vk-img-scrim{position:absolute;inset:0;pointer-events:none;background:linear-gradient(to top,rgb(0 0 0/.6),rgb(0 0 0/.08),transparent)}
.vk-img-overlay{position:absolute;inset:0}
@media (prefers-reduced-motion:reduce){.vk-img{animation:none}}`

// StyleTag is a <style> element holding Styles, to place in a page's head so
// the components are themed:
//
//	func layout(page ui.Node) ui.Node {
//		return ui.El("html", nil, ui.El("head", nil, ui.StyleTag()), ui.El("body", nil, page))
//	}
func StyleTag() Node {
	return El("style", []Attr{{Name: "data-vk-ui", Value: true}}, RawHTML(Styles))
}
