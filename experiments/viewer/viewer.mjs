// SOURCE-ONLY viewer experiment. Do not register in product or use this
// synthetic route as a production authorization / integrity implementation.
import { getDocument, GlobalWorkerOptions } from "/vendor/pdf.mjs";

const byId = id => document.getElementById(id);
const allowedFixturePaths = Object.freeze({
  ok: "/synthetic.pdf",
  denied: "/denied.pdf",
  missing: "/missing.pdf",
  corrupt: "/corrupt.pdf"
});
const query = new URL(location.href).searchParams;
const scenario = query.get("scenario") || "ok";
const pdfPath = Object.hasOwn(allowedFixturePaths, scenario) ?
  allowedFixturePaths[scenario] : allowedFixturePaths.denied;

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
    "Page " + state.index + " of " + state.document.numPages : "No document loaded";
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
    byId("download-original").hidden = false;
    status("ready", "Preview ready; this is a synthetic document.");
  } catch {
    byId("page-canvas").hidden = true;
    byId("download-original").hidden = true;
    status("failed", "Preview rendering failed; the original remains downloadable.");
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
    state.zoom = Math.min(2, Math.max(.5, state.zoom * factor));
    await render();
  });
}

async function open() {
  try {
    const response = await fetch(pdfPath, {
      cache: "no-store", credentials: "same-origin",
      headers: { Accept: "application/pdf" },
      redirect: "error"
    });
    if (!response.ok || response.headers.get("Content-Type") !== "application/pdf") {
      throw new Error("not admitted");
    }
    const bytes = await response.arrayBuffer();
    if (bytes.byteLength < 16 || bytes.byteLength > 10 * 1024 * 1024) {
      throw new Error("invalid fixture size");
    }
    const task = getDocument({
      data: new Uint8Array(bytes),
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
    status("denied", "Preview unavailable or denied. No document pages were shown.");
    controls();
  }
}
open();
