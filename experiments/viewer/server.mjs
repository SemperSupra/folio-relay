// Synthetic-only PDF viewer DoE server. NEVER a deployable FolioRelay route.
// No real document inputs, production token, persistent storage or external network.
import http from "node:http";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const base = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(base, "../..");
const host = "127.0.0.1";
const port = Number(process.env.VIEWER_TEST_PORT || "18763");
const csp = "default-src 'none'; script-src 'self'; worker-src 'self'; connect-src 'self'; style-src 'self'; img-src 'self' blob:; font-src 'self'; frame-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'";

function fakePDF() {
  const pageText = "BT /F1 20 Tf 70 710 Td (FolioRelay SYNTHETIC viewer test) Tj ET\n";
  const objects = [
    "<< /Type /Catalog /Pages 2 0 R >>",
    "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
    "<< /Length " + Buffer.byteLength(pageText) + " >>\nstream\n" + pageText + "endstream",
    "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
  ];
  let content = "%PDF-1.4\n";
  const offsets = [0];
  for (const [i, obj] of objects.entries()) {
    offsets.push(Buffer.byteLength(content));
    content += (i + 1) + " 0 obj\n" + obj + "\nendobj\n";
  }
  const xref = Buffer.byteLength(content);
  content += "xref\n0 " + (objects.length + 1) + "\n0000000000 65535 f \n";
  for (const offset of offsets.slice(1)) content += String(offset).padStart(10, "0") + " 00000 n \n";
  content += "trailer\n<< /Size " + (objects.length + 1) + " /Root 1 0 R >>\nstartxref\n" + xref + "\n%%EOF\n";
  return Buffer.from(content);
}
const pdf = fakePDF();

const files = new Map([
  ["/", ["index.html", "text/html; charset=utf-8"]],
  ["/viewer", ["index.html", "text/html; charset=utf-8"]],
  ["/viewer.mjs", ["viewer.mjs", "text/javascript; charset=utf-8"]],
  ["/viewer.css", ["viewer.css", "text/css; charset=utf-8"]],
  ["/vendor/pdf.mjs", ["node_modules/pdfjs-dist/build/pdf.mjs", "text/javascript; charset=utf-8"]],
  ["/vendor/pdf.worker.mjs", ["node_modules/pdfjs-dist/build/pdf.worker.mjs", "text/javascript; charset=utf-8"]]
]);

http.createServer(async (req, res) => {
  const pathname = new URL(req.url, "http://" + host + ":" + port).pathname;
  const headers = {
    "Content-Security-Policy": csp, "X-Content-Type-Options": "nosniff",
    "Cache-Control": "no-store", "Referrer-Policy": "no-referrer",
    "X-Frame-Options": "DENY"
  };
  function end(code, mime, body) {
    res.writeHead(code, {"Content-Type":mime, ...headers}); res.end(body);
  }
  // SOURCE-ONLY contract fixture: IDs are hardcoded fake roles and cannot
  // name real storage. In a real app, the Go actor-scoped verified-snapshot
  // handler must perform fresh document authorization BEFORE any PDF byte.
  const scoped = /^\/api\/v1\/jobs\/(job-[0-9a-f]{64})\/(preview|artifact)$/.exec(pathname);
  if (scoped) {
    const id = scoped[1];
    if (id === "job-" + "a".repeat(64)) return end(200, "application/pdf", pdf);
    if (id === "job-" + "b".repeat(64)) return end(403, "application/json", '{"error":"denied"}');
    if (id === "job-" + "c".repeat(64)) return end(404, "application/json", '{"error":"missing"}');
    if (id === "job-" + "d".repeat(64)) return end(200, "application/pdf", "NOT A PDF");
    return end(404, "application/json", '{"error":"unknown"}');
  }
  if (pathname === "/synthetic.pdf") return end(200, "application/pdf", pdf);
  if (pathname === "/denied.pdf") return end(403, "application/json", '{"error":"denied"}');
  if (pathname === "/missing.pdf") return end(404, "application/json", '{"error":"missing"}');
  if (pathname === "/corrupt.pdf") return end(200, "application/pdf", "NOT A PDF");
  const entry = files.get(pathname);
  if (!entry) return end(404, "text/plain", "not found");
  const [file, mime] = entry;
  try {
    const target = pathname.startsWith("/vendor/") ? path.join(root, file) : path.join(base, file);
    end(200, mime, await fs.readFile(target));
  } catch {
    end(503, "text/plain", "fixture not available");
  }
}).listen(port, host, () => {
  process.stdout.write("SOURCE_ONLY viewer fixture listening on " + host + ":" + port + "\n");
});
