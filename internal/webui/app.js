"use strict";

const $ = (id) => document.getElementById(id);
const state = { printer: null, status: null, diagnostics: null, locale: "en", messages: {} };

// A language is a presentation preference only. It never changes session
// authority, document identity, durable states, or provider routing.
function browserLocale() {
  for (const candidate of navigator.languages || [navigator.language || "en"]) {
    const primary = String(candidate).toLowerCase().split(/[-_]/)[0];
    if (["en", "de", "th"].includes(primary)) return primary;
  }
  return "en";
}
function tr(key, values = {}) {
  const template = state.messages[key];
  if (typeof template !== "string") return key;
  return template.replace(/\{([a-z][a-z0-9_]*)\}/g, (_match, name) =>
    Object.prototype.hasOwnProperty.call(values, name) ? String(values[name]) : "");
}
function renderStatic() {
  document.documentElement.lang = state.locale;
  document.querySelectorAll("[data-i18n]").forEach((node) => {
    node.textContent = tr(node.dataset.i18n);
  });
  document.querySelectorAll("[data-i18n-aria-label]").forEach((node) => {
    node.setAttribute("aria-label", tr(node.dataset.i18nAriaLabel));
  });
  $("language").value = state.locale;
  $("language").setAttribute("aria-label", tr("locale.label"));
}
async function changeLocale(lang) {
  if (!["en", "de", "th"].includes(lang)) throw new Error("unsupported locale");
  const response = await fetch("/locales/" + lang, {
    credentials: "same-origin", cache: "no-store"
  });
  if (!response.ok) throw new Error("locale catalog unavailable");
  const catalog = await response.json();
  if (!catalog || typeof catalog["login.title"] !== "string" ||
      typeof catalog["diagnostics.next_action"] !== "string") {
    throw new Error("invalid locale catalog");
  }
  state.messages = catalog;
  state.locale = lang;
  renderStatic();
  if (state.printer && state.status) renderOverview();
  if (state.diagnostics) renderChecks(state.diagnostics);
  if (state.status && !$("inbox").hidden) await loadJobs();
}


async function api(path, options = {}) {
  const response = await fetch(path, { credentials: "same-origin", ...options });
  if (response.status === 401) {
    showLogin();
    throw new Error(tr("login.required"));
  }
  if (!response.ok) {
    let code = "request_failed";
    try {
      const body = await response.json();
      if (/^[a-z][a-z0-9_]{0,72}$/.test(body?.error?.code || "")) {
        code = body.error.code;
      }
    } catch (_) {}
    throw new Error(tr("errors.generic", { code }));
  }
  if (response.status === 204) return null;
  return response.json();
}

function showLogin() {
  $("login").hidden = false;
  $("app").hidden = true;
  $("logout").hidden = true;
}

function showApp() {
  $("login").hidden = true;
  $("app").hidden = false;
  $("logout").hidden = false;
}

function setText(id, value) {
  $(id).textContent = value ?? "—";
}

function prettyTime(value) {
  if (!value) return tr("overview.never");
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? String(value) :
    date.toLocaleString(state.locale);
}

async function loadOverview() {
  const [status, printer] = await Promise.all([
    api("/api/v1/status"),
    api("/api/v1/printer")
  ]);
  state.status = status;
  state.printer = printer;
  showApp();
  renderOverview();
}

function renderOverview() {
  const { status, printer } = state;
  const overall = $("overall-status");
  overall.textContent = status.status === "ready" ?
    tr("overview.ready") : tr("overview.degraded");
  overall.className = status.status === "ready" ? "status-ready" : "status-warn";
  setText("status-detail", tr("overview.accepting"));
  setText("job-count", String(status.inbox_jobs ?? 0));
  setText("last-print", status.last_successful_print_at ?
    tr("overview.last_print", { time: prettyTime(status.last_successful_print_at) }) :
    tr("overview.never"));
  setText("printer-uri", printer.public_uri);
  setText("windows-state", tr(printer.profiles?.windows_ipp ?
    "overview.enabled" : "overview.not_enabled"));
  setText("airprint-state", tr(printer.profiles?.airprint ?
    "overview.enabled" : "overview.not_enabled"));
  setText("airprint-help", tr(printer.profiles?.airprint ?
    "overview.airprint_enabled" : "overview.airprint_disabled"));

  renderPrinter(printer);
}

function renderPrinter(printer) {
  const root = $("printer-details");
  root.replaceChildren();
  const entries = [
    [tr("printer.name"), printer.identity?.display_name],
    [tr("printer.location"), printer.identity?.location || "—"],
    [tr("printer.public_uri"), printer.public_uri],
    [tr("printer.uuid"), printer.identity?.printer_uuid],
    [tr("printer.windows_ipp"), tr(printer.profiles?.windows_ipp ? "overview.enabled" : "overview.disabled")],
    ["AirPrint", tr(printer.profiles?.airprint ? "overview.enabled" : "overview.disabled")]
  ];
  for (const [name, value] of entries) {
    const dt = document.createElement("dt");
    dt.textContent = name;
    const dd = document.createElement("dd");
    dd.textContent = value ?? "—";
    root.append(dt, dd);
  }
}

async function loadJobs() {
  const page = await api("/api/v1/jobs?limit=100");
  const root = $("jobs");
  root.replaceChildren();
  if (!page.items?.length) {
    const empty = document.createElement("p");
    empty.className = "muted";
    empty.textContent = tr("inbox.empty");
    root.append(empty);
    return;
  }
  for (const job of page.items) {
    const row = document.createElement("div");
    row.className = "job";

    const primary = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = job.source_title || job.download_name || prettyTime(job.accepted_at);
    const id = document.createElement("code");
    id.textContent = prettyTime(job.accepted_at) + " · " + job.job_id;
    primary.append(title, document.createElement("br"), id);

    const format = document.createElement("span");
    format.textContent = job.media_type;
    const size = document.createElement("span");
    size.textContent = formatBytes(job.artifact_bytes);
    const link = document.createElement("a");
    link.href = "/api/v1/jobs/" + encodeURIComponent(job.job_id) + "/artifact";
    link.download = job.download_name;
    link.textContent = tr("inbox.download");
    link.className = "quiet";
    row.append(primary, format, size, link);
    root.append(row);
  }
}

function formatBytes(value) {
  const bytes = Number(value);
  if (!Number.isFinite(bytes)) return "—";
  const number = (v) => new Intl.NumberFormat(state.locale, {
    minimumFractionDigits: 1, maximumFractionDigits: 1
  }).format(v);
  if (bytes < 1024) return new Intl.NumberFormat(state.locale).format(bytes) + " B";
  if (bytes < 1024 * 1024) return number(bytes / 1024) + " KiB";
  return number(bytes / (1024 * 1024)) + " MiB";
}

async function runSelfTest() {
  const receipt = await api("/api/v1/self-test", { method: "POST" });
  state.diagnostics = receipt;
  renderChecks(receipt);
  return receipt;
}

function renderChecks(receipt) {
  const root = $("checks");
  root.replaceChildren();
  for (const check of receipt.checks || []) {
    const article = document.createElement("article");
    article.className = "panel check " + check.status;
    const heading = document.createElement("h2");
    heading.textContent = check.code + " — " +
      tr("diagnostics." + check.status);
    const message = document.createElement("p");
    const key = "diagnostics." + check.code + "." + check.status;
    message.textContent = state.messages[key] ? tr(key) :
      tr("diagnostics.code_only", { code: check.code });
    article.append(heading, message);
    if (check.detail) {
      const detail = document.createElement("p");
      detail.className = "muted";
      detail.textContent = tr("diagnostics.technical_detail") + ": " + check.detail;
      article.append(detail);
    }
    if (check.remediation) {
      const remediation = document.createElement("p");
      const fixKey = "diagnostics." + check.code + ".fix";
      remediation.textContent = state.messages[fixKey] ?
        tr("diagnostics.next_action", { action: tr(fixKey) }) :
        tr("diagnostics.code_only", { code: check.code });
      article.append(remediation);
    }
    root.append(article);
  }
}

async function selectView(name) {
  document.querySelectorAll(".view").forEach((node) => { node.hidden = node.id !== name; });
  document.querySelectorAll(".tabs button").forEach((node) => {
    const selected = node.dataset.view === name;
    node.classList.toggle("active", selected);
    if (selected) {
      node.setAttribute("aria-current", "page");
    } else {
      node.removeAttribute("aria-current");
    }
  });
  if (name === "inbox") await loadJobs();
  if (name === "diagnostics") await runSelfTest();
}

$("login-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  $("login-error").textContent = "";
  const credential = $("credential");
  try {
    const response = await fetch("/auth/session", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: credential.value })
    });
    credential.value = "";
    if (!response.ok) throw new Error(tr("login.invalid"));
    await loadOverview();
  } catch (error) {
    $("login-error").textContent = error.message;
  }
});

$("logout").addEventListener("click", async () => {
  await fetch("/auth/logout", { method: "POST", credentials: "same-origin" });
  showLogin();
});

document.querySelectorAll(".tabs button").forEach((button) => {
  button.addEventListener("click", () => selectView(button.dataset.view));
});
$("refresh-jobs").addEventListener("click", loadJobs);
$("run-diagnostics").addEventListener("click", runSelfTest);
$("self-test").addEventListener("click", async () => {
  await selectView("diagnostics");
});
$("copy-uri").addEventListener("click", async () => {
  if (!state.printer?.public_uri) return;
  await navigator.clipboard.writeText(state.printer.public_uri);
  $("copy-uri").textContent = tr("overview.copied");
  setTimeout(() => { $("copy-uri").textContent = tr("overview.copy"); }, 1200);
});

$("language").addEventListener("change", async () => {
  const before = state.locale;
  try {
    await changeLocale($("language").value);
  } catch (_) {
    $("language").value = before;
  }
});

async function bootstrap() {
  try {
    await changeLocale(browserLocale());
  } catch (_) {
    // English markup remains available if the source-only catalog endpoint
    // cannot be served. Do not claim translated UI on partial failure.
  }
  try {
    const response = await fetch("/auth/session", {
      credentials: "same-origin",
      cache: "no-store"
    });
    if (!response.ok) throw new Error("session status unavailable");
    const session = await response.json();
    if (session.authenticated) {
      await loadOverview();
    } else {
      showLogin();
    }
  } catch (_) {
    showLogin();
  }
}

bootstrap();
