// Exact SOURCE-ONLY viewer reader contract; no browser, credentials or real documents.
// Run on a public Actions Node runner before expensive browser dependencies.
import test from "node:test";
import assert from "node:assert/strict";
import { readBoundedSyntheticPDF } from "../../experiments/viewer/bounded_reader.mjs";

test("overdeclared bytes cancel before source pull", async () => {
  let pulls = 0, cancels = 0;
  const body = new ReadableStream({
    pull(c) { pulls++; c.enqueue(new Uint8Array(256)); },
    cancel() { cancels++; }
  }, { highWaterMark: 0 });
  await assert.rejects(
    readBoundedSyntheticPDF(new Response(body, { headers: { "Content-Length": "10485761" } })),
    /oversized/
  );
  assert.deepEqual([pulls, cancels], [0, 1]);
});

test("invalid deadline fails closed before source pull", async () => {
  let pulls = 0, cancels = 0;
  const body = new ReadableStream({
    pull(c) { pulls++; c.enqueue(new Uint8Array(256)); },
    cancel() { cancels++; }
  }, { highWaterMark: 0 });
  await assert.rejects(
    readBoundedSyntheticPDF(new Response(body), { readDeadlineMs: 0 }),
    /invalid synthetic stream timeout/
  );
  assert.deepEqual([pulls, cancels], [0, 1]);
});

test("exact verified declared length produces unmodified synthetic bytes", async () => {
  const source = new Uint8Array(32).fill(0x50);
  const actual = await readBoundedSyntheticPDF(
    new Response(source, { headers: { "Content-Length": "32" } })
  );
  assert.deepEqual(actual, source);
});

test("short declared-length response is rejected", async () => {
  await assert.rejects(
    readBoundedSyntheticPDF(new Response(new Uint8Array(32), {
      headers: { "Content-Length": "33" }
    })),
    /short or inconsistent fixture/
  );
});

test("body that sends one chunk but never EOF is cancelled at deadline", async () => {
  let cancels = 0;
  const body = new ReadableStream({
    start(c) { c.enqueue(new Uint8Array(64).fill(0x50)); },
    cancel() { cancels++; }
  }, { highWaterMark: 0 });
  await assert.rejects(
    readBoundedSyntheticPDF(new Response(body), { readDeadlineMs: 75 }),
    /synthetic stream timed out/
  );
  assert.equal(cancels, 1);
});

// The stream's cancel() completion is *not* trusted. Failing to await cleanup
// is essential; otherwise a hostile source can hang the denial path itself.
test("oversize rejection never awaits a nonsettling cancel source", async () => {
  let calls = 0;
  const stream = new ReadableStream({
    cancel() { calls++; return new Promise(() => {}); }
  }, { highWaterMark: 0 });
  const result = await Promise.race([
    readBoundedSyntheticPDF(new Response(stream, {
      headers: { "Content-Length": "10485761" }
    })).then(() => "ADMITTED", error => error.message),
    new Promise(resolve => setTimeout(() => resolve("CLEANUP_HUNG"), 500))
  ]);
  assert.match(result, /invalid or oversized declared fixture/);
  assert.equal(calls, 1);
});

test("timeout denial does not await a nonsettling reader cancellation", async () => {
  let calls = 0;
  const stream = new ReadableStream({
    start(c) { c.enqueue(new Uint8Array(64).fill(0x50)); },
    cancel() { calls++; return new Promise(() => {}); }
  }, { highWaterMark: 0 });
  const result = await Promise.race([
    readBoundedSyntheticPDF(new Response(stream), { readDeadlineMs: 35 })
      .then(() => "ADMITTED", error => error.message),
    new Promise(resolve => setTimeout(() => resolve("CLEANUP_HUNG"), 500))
  ]);
  assert.match(result, /synthetic stream timed out/);
  assert.equal(calls, 1);
});

test("empty streamed chunk fails without waiting for the deadline", async () => {
  let cancels = 0;
  const stream = new ReadableStream({
    pull(c) { c.enqueue(new Uint8Array(0)); },
    cancel() { cancels++; }
  }, { highWaterMark: 0 });
  await assert.rejects(readBoundedSyntheticPDF(new Response(stream)), /empty or excessive/);
  assert.equal(cancels, 1);
});

test("thousands of tiny stream chunks hit the independent chunk budget", async () => {
  let cancels = 0, pulls = 0;
  const stream = new ReadableStream({
    pull(c) { pulls++; c.enqueue(new Uint8Array(16).fill(0x50)); },
    cancel() { cancels++; }
  }, { highWaterMark: 0 });
  await assert.rejects(readBoundedSyntheticPDF(new Response(stream)), /empty or excessive/);
  assert.equal(pulls, 8193);
  assert.equal(cancels, 1);
});
