// SOURCE-ONLY synthetic fixture admission, NOT a deployed FolioRelay route.
// Hard bounds and no redirects are independent of real #96 actor authorization.
export const syntheticPDFMaxBytes = 10 * 1024 * 1024;

export async function readBoundedSyntheticPDF(response) {
  const declared = response.headers.get("Content-Length");
  const rejectBody = async () => {
    if (response.body) {
      try { await response.body.cancel(); } catch { /* fail closed */ }
    }
  };
  if (declared !== null) {
    if (!/^(0|[1-9][0-9]*)$/.test(declared) ||
        !Number.isSafeInteger(Number(declared)) ||
        Number(declared) > syntheticPDFMaxBytes) {
      await rejectBody();
      throw new Error("invalid or oversized declared fixture");
    }
  }
  const reader = response.body?.getReader();
  if (!reader) throw new Error("missing response stream");
  const parts = [];
  let total = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!(value instanceof Uint8Array)) throw new Error("invalid PDF chunk");
      total += value.byteLength;
      if (total > syntheticPDFMaxBytes) throw new Error("oversized streamed fixture");
      parts.push(value);
    }
    if (total < 16 || (declared !== null && Number(declared) !== total)) {
      throw new Error("short or inconsistent fixture");
    }
    // Still a fixture, not a production memory limit: retained chunks +
    // assembled copy peak at roughly 2*max plus the incoming network chunk.
    const admitted = new Uint8Array(total);
    let cursor = 0;
    for (const part of parts) { admitted.set(part, cursor); cursor += part.byteLength; }
    return admitted;
  } catch (err) {
    try { await reader.cancel(); } catch { /* fail closed */ }
    throw err;
  } finally {
    parts.length = 0;
    reader.releaseLock();
  }
}
