"use strict";

const $ = (id) => document.getElementById(id);
// This last-resort, trusted plain-text copy is checked against the Go embedded
// English catalog in five-profile browser CI. Do not issue network requests
// or display raw i18n keys when language catalogs are offline.
const ENGLISH = Object.freeze({
  "portal.tagline": "Virtual Printer",
  "portal.sign_out": "Sign out",
  "login.title": "Connect to FolioRelay",
  "login.description": "Enter the management credential created when the TrueNAS app was installed.",
  "login.credential": "Management credential",
  "login.sign_in": "Sign in",
  "login.invalid": "The management credential was not accepted.",
  "login.required": "Authentication required",
  "login.unavailable": "Session status unavailable",
  "nav.aria": "FolioRelay views",
  "nav.overview": "Overview",
  "nav.inbox": "Inbox",
  "nav.printer": "Printer",
  "nav.diagnostics": "Diagnostics",
  "overview.system_status": "System status",
  "overview.checking": "Checking…",
  "overview.ready": "Ready",
  "overview.accepting": "FolioRelay is accepting and retaining print jobs.",
  "overview.self_test": "Run self-test",
  "overview.native_apple": "Native iPhone/iPad printing",
  "overview.native_windows": "Inbox Microsoft IPP path",
  "overview.accepted_jobs": "Accepted print jobs",
  "overview.endpoint": "Printer endpoint",
  "overview.copy": "Copy",
  "overview.copied": "Copied",
  "overview.never": "No successful prints yet.",
  "overview.last_print": "Last accepted print: {time}",
  "overview.enabled": "Enabled",
  "overview.disabled": "Disabled",
  "overview.not_enabled": "Not enabled",
  "overview.airprint_enabled": "AirPrint is configured. Installed-system discovery is verified separately by deployment diagnostics and real-device acceptance.",
  "overview.airprint_disabled": "AirPrint is not enabled in this runtime profile.",
  "inbox.heading": "Accepted artifacts",
  "inbox.refresh": "Refresh",
  "inbox.empty": "No accepted print jobs yet.",
  "inbox.download": "Download",
  "printer.native": "Native printing",
  "printer.name": "Name",
  "printer.location": "Location",
  "printer.public_uri": "Public URI",
  "printer.uuid": "Printer UUID",
  "printer.windows_ipp": "Windows IPP",
  "printer.instructions_windows": "Add the printer using the IPP endpoint above. No vendor driver is required.",
  "printer.airprint_loading": "AirPrint status is loading.",
  "diagnostics.shared_oracle": "Shared human / automation / agent oracle",
  "diagnostics.next_action": "Next action: {action}",
  "diagnostics.pass": "Pass",
  "diagnostics.fail": "Fail",
  "diagnostics.warn": "Warning",
  "diagnostics.unknown": "Unknown",
  "diagnostics.printer.identity.pass": "Canonical printer identity is valid.",
  "diagnostics.printer.identity.fail": "Canonical printer identity is invalid.",
  "diagnostics.state.inbox_projection.pass": "Durable Inbox projection is reconstructable.",
  "diagnostics.state.inbox_projection.fail": "Durable Inbox projection cannot be reconstructed.",
  "diagnostics.storage.artifact_store.pass": "Artifact store is available.",
  "diagnostics.storage.artifact_store.fail": "Artifact store is unavailable.",
  "diagnostics.profiles.windows_ipp.pass": "Windows inbox IPP profile is enabled.",
  "diagnostics.profiles.airprint.pass": "AirPrint profile is enabled by qualified configuration.",
  "diagnostics.profiles.airprint.warn": "AirPrint profile is not yet enabled.",
  "diagnostics.code_only": "Diagnostic code: {code}",
  "errors.generic": "The operation could not be completed. Reference: {code}",
  "locale.label": "Language",
  "diagnostics.technical_detail": "Technical detail",
  "diagnostics.printer.identity.fix": "Repair the durable printer identity before admitting print jobs.",
  "diagnostics.state.inbox_projection.fix": "Preserve the journal and inspect the first invalid durable Inbox record.",
  "diagnostics.storage.artifact_store.fix": "Restore the configured FolioRelay data dataset/mount before accepting jobs.",
  "diagnostics.profiles.airprint.fix": "Enable only after the selected substrate passes the AirPrint MVP profile gate.",
  "overview.degraded": "Action required"
});
const state = {
  printer: null, status: null, diagnostics: null, jobPage: null,
  locale: "en", messages: ENGLISH, localeRequest: 0
};
const keys = Object.keys(ENGLISH).sort();
function placeholders(message) {
  return [...new Set([...message.matchAll(/\{([a-z][a-z0-9_]*)\}/g)].map(x => x[1]))].sort();
}
function validCatalog(candidate) {
  if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) return false;
  const provided = Object.keys(candidate).sort();
  if (provided.length !== keys.length || provided.some((k, i) => k !== keys[i])) return false;
  return keys.every((key) => {
    const entry = candidate[key];
    return typeof entry === "string" && entry.length > 0 &&
      !/[<>]/.test(entry) &&
      JSON.stringify(placeholders(entry)) === JSON.stringify(placeholders(ENGLISH[key]));
  });
}
function announce(text) {
  const region = $("announcements");
  if (region) region.textContent = text;
}

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
  const template = state.messages[key] ?? ENGLISH[key];
  if (typeof template !== "string") return ENGLISH["login.unavailable"];
  return template.replace(/\{([a-z][a-z0-9_]*)\}/g, (_match, name) =>
    Object.prototype.hasOwnProperty.call(values, name) ? String(values[name]) : "—");
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
  // Last requested locale wins. An older fetch may resolve after a newer
  // selection; it must never modify the UI, focus, or user-facing preference.
  const request = ++state.localeRequest;
  let catalog;
  try {
    if (lang === "en") {
      // English works offline even when the catalog HTTP endpoint is down.
      catalog = ENGLISH;
    } else {
      const response = await fetch("/locales/" + lang, {
        credentials: "same-origin", cache: "no-store"
      });
      if (!response.ok) throw new Error("locale catalog unavailable");
      catalog = await response.json();
      if (!validCatalog(catalog)) throw new Error("incomplete locale catalog");
    }
  } catch (error) {
    if (request !== state.localeRequest) return false;
    throw error;
  }
  if (request !== state.localeRequest) return false;
  state.messages = catalog;
  state.locale = lang;
  renderStatic();
  if (state.printer && state.status) renderOverview();
  if (state.diagnostics) renderChecks(state.diagnostics);
  if (state.jobPage && !$("inbox").hidden) renderJobs();
  announce(tr("locale.label") + ": " + $("language").selectedOptions[0].textContent);
  return true;
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
  setText("status-detail", tr(status.status === "ready" ?
    "overview.accepting" : "overview.degraded"));
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
  state.jobPage = await api("/api/v1/jobs?limit=100");
  renderJobs();
}

function renderJobs() {
  const page = state.jobPage;
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
  const requested = $("language").value;
  try {
    await changeLocale(requested);
  } catch (_) {
    // Failure of the latest request preserves the last *committed* locale.
    $("language").value = state.locale;
    announce(tr("errors.generic", {code: "locale_unavailable"}));
  }
});

async function bootstrap() {
  try {
    await changeLocale(browserLocale());
  } catch (_) {
    // The embedded English catalog is always available, including during
    // network failures before an authenticated session has been established.
    state.locale = "en";
    state.messages = ENGLISH;
    renderStatic();
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
