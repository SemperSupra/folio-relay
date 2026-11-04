// SOURCE-ONLY viewer experiment. Do not register in product or use this
// synthetic route as a production authorization / integrity implementation.
import { getDocument, GlobalWorkerOptions } from "/vendor/pdf.mjs";
import { readBoundedSyntheticPDF } from "/bounded_reader.mjs";

const byId = id => document.getElementById(id);
const allowedFixturePaths = Object.freeze({
  ok: "/synthetic.pdf",
  denied: "/denied.pdf",
  missing: "/missing.pdf",
  corrupt: "/corrupt.pdf"
});
const query = new URL(location.href).searchParams;

// Locale affects ONLY visible synthetic UI text and accessibility metadata.
// It does NOT choose document bytes, authorize a record or alter request origin.
const copy = Object.freeze({
  en: {
    title:"FolioRelay — Synthetic PDF Viewer Qualification",
    eyebrow:"SOURCE-ONLY QUALIFICATION · SYNTHETIC DOCUMENT",
    heading:"Document preview",
    disclaimer:"This independent browser fixture is not a FolioRelay runtime route or an authorization decision.",
    nav:"Document controls", area:"Document viewing area", canvas:"PDF page preview",
    previous:"Previous page", next:"Next page", zoomOut:"Zoom out", zoomIn:"Zoom in",
    download:"Download synthetic original", page:"Page", of:"of",
    empty:"No document loaded", opening:"Opening synthetic preview…",
    ready:"Preview ready; this is a synthetic document.",
    failed:"Preview rendering failed; the original remains downloadable.",
    denied:"Preview unavailable or denied. No document pages were shown."
  },
  de: {
    title:"FolioRelay — Synthetischer PDF-Anzeigetest",
    eyebrow:"NUR QUELLTEST · SYNTHETISCHES DOKUMENT",
    heading:"Dokumentvorschau",
    disclaimer:"Diese unabhängige Browser-Testumgebung ist weder eine FolioRelay-Produktfunktion noch eine Berechtigungsentscheidung.",
    nav:"Dokumentsteuerung", area:"Dokumentanzeigebereich", canvas:"PDF-Seitenvorschau",
    previous:"Vorherige Seite", next:"Nächste Seite", zoomOut:"Verkleinern", zoomIn:"Vergrößern",
    download:"Synthetisches Original herunterladen", page:"Seite", of:"von",
    empty:"Kein Dokument geladen", opening:"Synthetische Vorschau wird geöffnet…",
    ready:"Vorschau bereit; dies ist ein synthetisches Dokument.",
    failed:"Die Vorschau konnte nicht dargestellt werden; das Original bleibt herunterladbar.",
    denied:"Vorschau nicht verfügbar oder verweigert. Es wurden keine Dokumentseiten angezeigt."
  },
  th: {
    title:"FolioRelay — การทดสอบตัวแสดง PDF สังเคราะห์",
    eyebrow:"ทดสอบซอร์สเท่านั้น · เอกสารสังเคราะห์",
    heading:"ตัวอย่างเอกสาร",
    disclaimer:"หน้าทดสอบเบราว์เซอร์นี้ไม่ใช่หน้าทำงานจริงของ FolioRelay หรือการอนุญาตเข้าถึงเอกสาร",
    nav:"ตัวควบคุมเอกสาร", area:"พื้นที่แสดงเอกสาร", canvas:"ตัวอย่างหน้า PDF",
    previous:"หน้าก่อนหน้า", next:"หน้าถัดไป", zoomOut:"ย่อ", zoomIn:"ขยาย",
    download:"ดาวน์โหลดต้นฉบับสังเคราะห์", page:"หน้า", of:"จาก",
    empty:"ยังไม่ได้โหลดเอกสาร", opening:"กำลังเปิดตัวอย่างสังเคราะห์…",
    ready:"ตัวอย่างพร้อมแสดง นี่คือเอกสารสังเคราะห์",
    failed:"แสดงตัวอย่างไม่สำเร็จ ยังสามารถดาวน์โหลดต้นฉบับได้",
    denied:"ไม่สามารถแสดงตัวอย่างหรือถูกปฏิเสธ ไม่มีการแสดงหน้าเอกสาร"
  }
});
const requestedLocale = query.get("lang");
const locale = Object.hasOwn(copy, requestedLocale) ? requestedLocale : "en";
const words = copy[locale];
document.documentElement.lang = locale;
document.title = words.title;
document.querySelector(".eyebrow").textContent = words.eyebrow;
document.querySelector("h1").textContent = words.heading;
document.querySelector("header p:last-child").textContent = words.disclaimer;
document.querySelector("nav").setAttribute("aria-label", words.nav);
document.querySelector("section").setAttribute("aria-label", words.area);
byId("page-canvas").setAttribute("aria-label", words.canvas);
for (const [id, key] of [
  ["previous","previous"],["next","next"],["zoom-out","zoomOut"],
  ["zoom-in","zoomIn"],["download-original","download"]
]) byId(id).textContent = words[key];
byId("page-status").textContent = words.empty;
byId("status").textContent = words.opening;
// The fake PDF content is English; UI locale must not reclassify its text.
byId("text-result").lang = "en";
const scoped = location.pathname === "/viewer";
const recordID = scoped ? query.get("job_id") : null;
// Do not build a document URL from arbitrary query parameters, and never
// accept a full PDF URL or token from the browser. The synthetic server has
// fake record IDs only; source-side authorization does NOT live here.
const canonicalID = typeof recordID === "string" && /^job-[0-9a-f]{64}$/.test(recordID);
const scenario = query.get("scenario") || "ok";
const pdfPath = scoped
  ? (canonicalID ? "/api/v1/jobs/" + recordID + "/preview" : null)
  : (Object.hasOwn(allowedFixturePaths, scenario) ?
    allowedFixturePaths[scenario] : allowedFixturePaths.denied);
const downloadPath = scoped && canonicalID
  ? "/api/v1/jobs/" + recordID + "/artifact"
  : "/synthetic.pdf";

GlobalWorkerOptions.workerSrc = "/vendor/pdf.worker.mjs";

const state = { document: null, index: 1, zoom: 1, rendering: false };
function status(code, message) {
  byId("preview").dataset.state = code;
  byId("status").textContent = message;
}
function controls() {
  const enabled = Boolean(state.document) && !state.rendering;
  byId("previous").disabled = !enabled || state.index <= 1;
  byId("next").disabled = !enabled || state.index >= state.document.numPages;
  byId("zoom-out").disabled = !enabled || state.zoom <= .5;
  byId("zoom-in").disabled = !enabled || state.zoom >= 2;
  byId("page-status").textContent = state.document ?
    words.page + " " + state.index + " " + words.of + " " + state.document.numPages : words.empty;
}
async function render() {
  if (state.rendering || !state.document) return;
  state.rendering = true;
  controls();
  try {
    const page = await state.document.getPage(state.index);
    const viewport = page.getViewport({ scale: state.zoom });
    const canvas = byId("page-canvas");
    canvas.width = Math.floor(viewport.width);
    canvas.height = Math.floor(viewport.height);
    const canvasContext = canvas.getContext("2d", {alpha: false});
    await page.render({ canvas, canvasContext, viewport }).promise;
    const contents = await page.getTextContent();
    byId("text-result").textContent = contents.items.map(i => i.str || "").join(" ");
    canvas.hidden = false;
    byId("download-original").href = downloadPath;
    byId("download-original").hidden = false;
    status("ready", words.ready);
  } catch {
    byId("page-canvas").hidden = true;
    byId("download-original").hidden = true;
    status("failed", words.failed);
  } finally {
    state.rendering = false;
    controls();
  }
}
for (const [id, delta] of [["previous", -1], ["next", 1]]) {
  byId(id).addEventListener("click", async () => {
    if (!state.document || state.rendering) return;
    const page = state.index + delta;
    if (page < 1 || page > state.document.numPages) return;
    state.index = page;
    await render();
  });
}
for (const [id, factor] of [["zoom-in", 1.25], ["zoom-out", .8]]) {
  byId(id).addEventListener("click", async () => {
    if (state.rendering || !state.document) return;
    state.zoom = Math.min(2, Ma// Tested separately with controlled ReadableStreams in Playwright.
// Fixture cap is 10 MiB, NOT an accepted production document budget.
 finally {
    parts.length=0;
    reader.releaseLock();
  }
}

async function open() {
  try {
    if (!pdfPath) throw new Error("invalid or missing scoped document id");
    const response = await fetch(pdfPath, {
      cache: "no-store", credentials: "same-origin",
      headers: { Accept: "application/pdf" },
      redirect: "error"
    });
    if (!response.ok || response.headers.get("Content-Type") !== "application/pdf") {
      throw new Error("not admitted");
    }
    const bytes = await readBoundedSyntheticPDF(response);
    const task = getDocument({
      data: bytes,
      isEvalSupported: false, enableScripting: false, stopAtErrors: true
    });
    state.document = await task.promise;
    if (state.document.numPages < 1 || state.document.numPages > 20) {
      throw new Error("unsupported page count");
    }
    await render();
  } catch {
    byId("page-canvas").hidden = true;
    byId("download-original").hidden = true;
    state.document = null;
    status("denied", words.denied);
    controls();
  }
}
open();
