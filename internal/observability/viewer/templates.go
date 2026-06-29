package viewer

const overviewTitle = "CodeAgentLens Debug Viewer"

const pageCSS = `
:root {
  color-scheme: dark;
  --bg: #090b10;
  --bg-2: #10151f;
  --panel: #151b26;
  --panel-2: #111722;
  --ink: #edf2ff;
  --muted: #9aa7b8;
  --line: #2a3546;
  --accent: #ff9f43;
  --accent-2: #5eb8ff;
  --healthy: #50d890;
  --warn: #f7c948;
  --code: #06080d;
}
* {
  box-sizing: border-box;
}
body {
  margin: 0;
  min-height: 100vh;
  color: var(--ink);
  background:
    radial-gradient(circle at 22% 8%, rgba(255, 159, 67, .18), transparent 28rem),
    radial-gradient(circle at 84% 4%, rgba(94, 184, 255, .18), transparent 30rem),
    linear-gradient(145deg, var(--bg) 0%, var(--bg-2) 62%, #0d111a 100%);
  font-family: "Aptos", "Segoe UI", sans-serif;
}
a {
  color: var(--accent-2);
  text-decoration: none;
}
a:hover {
  text-decoration: underline;
}
.app-shell {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  min-height: 100vh;
}
.sidebar {
  position: sticky;
  top: 0;
  height: 100vh;
  padding: 24px 18px;
  border-right: 1px solid var(--line);
  background: rgba(11, 15, 23, .9);
  backdrop-filter: blur(16px);
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 28px;
}
.brand-mark {
  width: 14px;
  height: 28px;
  border-radius: 999px;
  background: linear-gradient(180deg, var(--accent), var(--accent-2));
  box-shadow: 0 0 24px rgba(255, 159, 67, .45);
}
.primary-nav,
.quick-nav {
  display: grid;
  gap: 10px;
}
.quick-nav {
  margin-top: 28px;
  color: var(--muted);
}
.quick-nav a {
  display: inline-block;
  margin: 0 8px 8px 0;
}
.nav-link {
  padding: 11px 12px;
  border: 1px solid transparent;
  border-radius: 12px;
  color: var(--muted);
}
.nav-link.active,
.nav-link:hover {
  color: var(--ink);
  border-color: var(--line);
  background: rgba(94, 184, 255, .12);
  text-decoration: none;
}
.shell {
  width: min(1220px, calc(100% - 32px));
  margin: 0 auto;
  padding: 22px 0 56px;
}
.topbar {
  min-height: 38px;
  display: flex;
  align-items: center;
}
.breadcrumbs {
  color: var(--muted);
  font-size: 14px;
}
.hero,
.panel,
.request-card,
.prompt-block,
.status-tile {
  border: 1px solid var(--line);
  background: linear-gradient(180deg, rgba(21, 27, 38, .94), rgba(14, 19, 29, .94));
  box-shadow: 0 18px 60px rgba(0, 0, 0, .24);
}
.hero {
  padding: 32px;
  border-radius: 22px;
  margin-bottom: 20px;
}
.hero h1 {
  margin: 0 0 10px;
  font-size: clamp(30px, 5vw, 58px);
  line-height: 1;
  letter-spacing: -.04em;
}
.hero p {
  max-width: 820px;
  margin: 8px 0 0;
  color: var(--muted);
  font-size: 16px;
}
.eyebrow {
  margin: 0 0 12px !important;
  color: var(--accent);
  font-size: 12px !important;
  font-weight: 700;
  letter-spacing: .18em;
  text-transform: uppercase;
}
.panel {
  padding: 24px;
  border-radius: 18px;
  margin-top: 18px;
}
.panel h2,
.request-card h2,
.request-card h3,
.prompt-block h2,
.prompt-block h3 {
  margin-top: 0;
}
.section-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
}
.status-grid,
.dashboard-grid,
.action-grid,
.grid {
  display: grid;
  gap: 14px;
}
.status-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin: 18px 0;
}
.dashboard-grid {
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
}
.action-grid,
.grid {
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
}
.status-tile {
  display: grid;
  gap: 10px;
  padding: 18px;
  border-radius: 16px;
  color: var(--ink);
}
.status-tile span {
  color: var(--muted);
}
.status-tile strong {
  font-size: 24px;
}
.tool-card {
  display: grid;
  gap: 8px;
  min-height: 142px;
  padding: 18px;
  border: 1px solid var(--line);
  border-radius: 16px;
  color: inherit;
  background: linear-gradient(150deg, rgba(255, 159, 67, .12), rgba(94, 184, 255, .08));
  text-decoration: none;
}
.tool-card strong {
  font-size: 20px;
}
.tool-card span,
.muted {
  color: var(--muted);
}
.empty-state {
  padding: 20px;
  border: 1px dashed var(--line);
  border-radius: 14px;
  color: var(--muted);
}
.table-wrap {
  overflow-x: auto;
}
.trace-list {
  overflow: visible;
}
.obs-table {
  width: 100%;
  border-collapse: collapse;
}
.obs-table th,
.obs-table td {
  padding: 12px;
  border-bottom: 1px solid var(--line);
  text-align: left;
  vertical-align: top;
}
.obs-table th {
  color: var(--muted);
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: .08em;
}
.prompt-trace-table {
  table-layout: fixed;
}
.prompt-trace-table th:nth-child(3),
.prompt-trace-table td:nth-child(3) {
  min-width: 180px;
}
.prompt-trace-table th:nth-child(5),
.prompt-trace-table td:nth-child(5) {
  min-width: 220px;
}
.trace-row {
  transition: background .16s ease, box-shadow .16s ease;
}
.trace-row:hover,
.trace-row:focus-within {
  background: rgba(94, 184, 255, .07);
  box-shadow: inset 3px 0 0 var(--accent-2);
}
.trace-cell {
  position: relative;
}
.trace-primary {
  display: inline-flex;
  max-width: 100%;
}
.trace-primary code {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hover-card {
  position: absolute;
  z-index: 20;
  left: 18px;
  top: calc(100% - 4px);
  width: min(680px, 78vw);
  max-height: min(560px, 72vh);
  overflow: auto;
  padding: 16px;
  border: 1px solid rgba(94, 184, 255, .34);
  border-radius: 16px;
  opacity: 0;
  visibility: hidden;
  transform: translateY(8px) scale(.98);
  pointer-events: none;
  background:
    linear-gradient(180deg, rgba(17, 23, 34, .98), rgba(9, 12, 18, .98)),
    radial-gradient(circle at 20% 0%, rgba(255, 159, 67, .12), transparent 18rem);
  box-shadow: 0 22px 70px rgba(0, 0, 0, .52);
  transition: opacity .14s ease, transform .14s ease, visibility .14s ease;
}
.trace-cell:hover .hover-card,
.trace-cell:focus-within .hover-card {
  opacity: 1;
  visibility: visible;
  transform: translateY(0) scale(1);
}
.hover-card-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.hover-card-head strong {
  overflow-wrap: anywhere;
}
.hover-card-head span {
  color: var(--muted);
  white-space: nowrap;
}
.role-badge {
  display: inline-flex;
  align-items: center;
  margin: 0 6px 6px 0;
  padding: 3px 8px;
  border: 1px solid rgba(94, 184, 255, .35);
  border-radius: 999px;
  color: #cfeaff;
  background: rgba(94, 184, 255, .12);
  font-size: 12px;
  font-weight: 700;
}
.row-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.explorer-layout {
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}
.filter-rail {
  position: sticky;
  top: 22px;
  padding: 18px;
  border: 1px solid var(--line);
  border-radius: 18px;
  background: rgba(12, 17, 26, .92);
}
.filter-form {
  display: grid;
  gap: 12px;
}
.filter-form label {
  display: grid;
  gap: 6px;
  color: var(--muted);
}
.filter-form input,
.filter-form select {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
  color: var(--ink);
  background: var(--code);
}
.filter-form button {
  padding: 11px 12px;
  border: 0;
  border-radius: 10px;
  color: #1b1207;
  background: var(--accent);
  font-weight: 700;
}
.reset-link {
  color: var(--muted);
}
.prompt-summary header {
  margin-bottom: 12px;
}
.role-row {
  margin: 10px 0;
}
.snippet-block {
  margin-top: 10px;
  padding: 12px;
  border: 1px solid rgba(42, 53, 70, .75);
  border-radius: 14px;
  background: rgba(5, 8, 13, .35);
}
.snippet-block div {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.compact-snippet {
  margin-top: 8px;
  padding: 10px;
}
.compact-snippet pre {
  max-height: 160px;
  margin: 8px 0 0;
  padding: 12px;
  font-size: 12px;
}
.pagination {
  display: flex;
  gap: 12px;
  margin-top: 18px;
}
.pagination a {
  padding: 9px 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
}
.tool-frame-panel {
  min-height: 70vh;
}
.frame-policy {
  color: var(--warn);
}
.tool-frame {
  width: 100%;
  height: 68vh;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: #05070b;
}
code,
pre {
  font-family: "Cascadia Code", Consolas, "Courier New", monospace;
}
code {
  color: var(--accent);
  overflow-wrap: anywhere;
}
pre {
  overflow: auto;
  white-space: pre-wrap;
  padding: 16px;
  border-radius: 14px;
  color: #edf7f5;
  background: var(--code);
}
.request-card,
.prompt-block {
  padding: 18px;
  border-radius: 16px;
  margin: 14px 0;
}
.loading-indicator {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: grid;
  place-items: center;
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  background: rgba(5, 8, 13, .52);
  backdrop-filter: blur(2px);
  transition: opacity .12s ease, visibility .12s ease;
}
.loading-indicator span {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border: 1px solid var(--line);
  border-radius: 999px;
  color: var(--ink);
  background: rgba(17, 23, 34, .96);
  box-shadow: 0 16px 48px rgba(0, 0, 0, .42);
}
.loading-indicator span::before {
  content: "";
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 18px rgba(255, 159, 67, .8);
  animation: pulse-dot 1s ease-in-out infinite;
}
body.is-loading .loading-indicator {
  opacity: 1;
  visibility: visible;
}
@keyframes pulse-dot {
  0%, 100% { transform: scale(.8); opacity: .55; }
  50% { transform: scale(1.15); opacity: 1; }
}
@media (max-width: 940px) {
  .app-shell {
    grid-template-columns: 1fr;
  }
  .sidebar {
    position: static;
    height: auto;
    border-right: 0;
    border-bottom: 1px solid var(--line);
  }
  .status-grid,
  .dashboard-grid,
  .explorer-layout {
    grid-template-columns: 1fr;
  }
  .filter-rail {
    position: static;
  }
  .prompt-trace-table {
    table-layout: auto;
  }
  .hover-card {
    position: static;
    width: auto;
    max-height: none;
    margin-top: 10px;
    opacity: 1;
    visibility: visible;
    transform: none;
  }
}
@media (max-width: 640px) {
  .shell {
    width: min(100% - 20px, 1180px);
    padding-top: 18px;
  }
  .hero,
  .panel {
    padding: 18px;
    border-radius: 18px;
  }
}
`
