"use strict";

const $ = (id) => document.getElementById(id);
const state = { printer: null, status: null };

async function api(path, options = {}) {
  const response = await fetch(path, { credentials: "same-origin", ...options });
  if (response.status === 401) {
    showLogin();
    throw new Error("authentication required");
  }
  if (!response.ok) {
    let message = response.statusText;
    try {
      const body = await response.json();
      message = body?.error?.message || message;
    } catch (_) {}
    throw new Error(message);
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
  if (!value) return "No successful prints yet.";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? String(value) : date.toLocaleString();
}

async function loadOverview() {
  const [status, printer] = await Promise.all([
    api("/api/v1/status"),
    api("/api/v1/printer")
  ]);
  state.status = status;
  state.printer = printer;
  showApp();

  const overall = $("overall-status");
  overall.textContent = status.status === "ready" ? "Ready" : status.status;
  overall.className = status.status === "ready" ? "status-ready" : "status-warn";
  setText("status-detail", "FolioRelay is accepting and retaining print jobs.");
  setText("job-count", String(status.inbox_jobs ?? 0));
  setText("last-print", status.last_successful_print_at ? "Last accepted print: " + prettyTime(status.last_successful_print_at) : "No successful prints yet.");
  setText("printer-uri", printer.public_uri);
  setText("windows-state", printer.profiles?.windows_ipp ? "Enabled" : "Not enabled");
  setText("airprint-state", printer.profiles?.airprint ? "Enabled" : "Not enabled");
  setText("airprint-help", printer.profiles?.airprint
    ? "AirPrint is configured. Installed-system discovery is verified separately by deployment diagnostics and real-device acceptance."
    : "AirPrint is not enabled in this runtime profile.");

  renderPrinter(printer);
}

function renderPrinter(printer) {
  const root = $("printer-details");
  root.replaceChildren();
  const entries = [
    ["Name", printer.identity?.display_name],
    ["Location", printer.identity?.location || "—"],
    ["Public URI", printer.public_uri],
    ["Printer UUID", printer.identity?.printer_uuid],
    ["Windows IPP", printer.profiles?.windows_ipp ? "Enabled" : "Disabled"],
    ["AirPrint", printer.profiles?.airprint ? "Enabled" : "Disabled"]
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
    empty.textContent = "No accepted print jobs yet.";
    root.append(empty);
    return;
  }
  for (const job of page.items) {
    const row = document.createElement("div");
    row.className = "job";

    const primary = document.createElement("div");
    const title = document.createElement("strong");
    title.textContent = prettyTime(job.accepted_at);
    const id = document.createElement("code");
    id.textContent = job.job_id;
    primary.append(title, document.createElement("br"), id);

    const format = document.createElement("span");
    format.textContent = job.media_type;
    const size = document.createElement("span");
    size.textContent = formatBytes(job.artifact_bytes);
    const link = document.createElement("a");
    link.href = "/api/v1/jobs/" + encodeURIComponent(job.job_id) + "/artifact";
    link.textContent = "Download";
    link.className = "quiet";
    row.append(primary, format, size, link);
    root.append(row);
  }
}

function formatBytes(value) {
  const bytes = Number(value);
  if (!Number.isFinite(bytes)) return "—";
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KiB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MiB";
}

async function runSelfTest() {
  const receipt = await api("/api/v1/self-test", { method: "POST" });
  const root = $("checks");
  root.replaceChildren();
  for (const check of receipt.checks || []) {
    const article = document.createElement("article");
    article.className = "panel check " + check.status;
    const heading = document.createElement("h2");
    heading.textContent = check.code + " — " + check.status.toUpperCase();
    const message = document.createElement("p");
    message.textContent = check.message;
    article.append(heading, message);
    if (check.detail) {
      const detail = document.createElement("p");
      detail.className = "muted";
      detail.textContent = check.detail;
      article.append(detail);
    }
    if (check.remediation) {
      const remediation = document.createElement("p");
      remediation.textContent = "Next action: " + check.remediation;
      article.append(remediation);
    }
    root.append(article);
  }
  return receipt;
}

async function selectView(name) {
  document.querySelectorAll(".view").forEach((node) => { node.hidden = node.id !== name; });
  document.querySelectorAll(".tabs button").forEach((node) => {
    const selected = node.dataset.view === name;\n    node.classList.toggle("active", selected);\n    node.setAttribute("aria-selected", selected ? "true" : "false");
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
    if (!response.ok) throw new Error("The management credential was not accepted.");
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
  $("copy-uri").textContent = "Copied";
  setTimeout(() => { $("copy-uri").textContent = "Copy"; }, 1200);
});

loadOverview().catch(() => showLogin());
