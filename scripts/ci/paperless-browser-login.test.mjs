import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import { test } from "node:test";

const source = readFileSync(new URL("../../internal/filing/paperless/review_login.js", import.meta.url), "utf8");
const effect = "a".repeat(64);
const credential = "0123456789abcdef0123456789abcdef01234567890";
assert.equal(credential.length, 43);

function createFixture({ lang = "en", query = "?effect=" + effect, reply = null } = {}) {
  const requests = [];
  const navigations = [];
  const effectInput = { value: "" };
  const tokenInput = { value: "" };
  const status = { textContent: "" };
  const form = {
    handler: undefined,
    addEventListener(name, handler) {
      assert.equal(name, "submit");
      this.handler = handler;
    }
  };
  const response = reply || { ok: true, async json() { return { review_path: "/filing/reviews/" + effect }; } };
  const sandbox = {
    URLSearchParams,
    document: {
      documentElement: { lang },
      querySelector(sel) {
        return sel === "[data-review-form]" ? form : sel === "[data-status]" ? status : null;
      },
      getElementById(id) {
        return id === "effect" ? effectInput : id === "grant" ? tokenInput : null;
      }
    },
    window: {
      location: {
        search: query,
        assign(path) { navigations.push(path); }
      }
    },
    async fetch(url, options) {
      requests.push({ url, options });
      return response;
    }
  };
  runInNewContext(source, sandbox, { filename: "review_login.js", timeout: 2000 });
  assert.equal(typeof form.handler, "function");
  return { form, effectInput, tokenInput, status, requests, navigations };
}

async function submit(f) {
  let prevented = false;
  await f.form.handler({ preventDefault() { prevented = true; } });
  assert.equal(prevented, true, "no native form submission should send a URL credential");
}

test("EN review login sends a single scoped Authorization header and navigates relatively", async () => {
  const f = createFixture();
  assert.equal(f.effectInput.value, effect);
  f.tokenInput.value = credential;
  await submit(f);
  assert.equal(f.tokenInput.value, "", "credential field must be cleared");
  assert.equal(f.requests.length, 1);
  const request = f.requests[0];
  assert.equal(request.url, "/filing/review-sessions");
  assert.equal(request.options.method, "POST");
  assert.equal(request.options.mode, "same-origin");
  assert.equal(request.options.credentials, "same-origin");
  assert.equal(request.options.redirect, "error");
  assert.equal(request.options.cache, "no-store");
  assert.equal(request.options.referrerPolicy, "no-referrer");
  assert.equal(request.options.headers.Authorization, "Bearer " + credential);
  assert.equal(request.options.headers["X-FolioRelay-Review-Session"], "1");
  assert.deepEqual(JSON.parse(request.options.body), { effect_id: effect });
  assert.equal(request.options.body.includes(credential), false);
  assert.deepEqual(f.navigations, ["/filing/reviews/" + effect]);
});

test("unknown or malformed effect and token never reach network", async () => {
  const f = createFixture({ query: "?effect=../../private" });
  assert.equal(f.effectInput.value, "");
  f.effectInput.value = "not-a-sha";
  f.tokenInput.value = credential;
  await submit(f);
  assert.equal(f.requests.length, 0);
  assert.equal(f.tokenInput.value, "");
  assert.deepEqual(f.navigations, []);
  assert.match(f.status.textContent, /Invalid/);
});

test("Thai denial uses localized status and never leaves token in a field", async () => {
  const f = createFixture({ lang: "th", reply: { ok: false } });
  f.tokenInput.value = credential;
  await submit(f);
  assert.equal(f.requests.length, 1);
  assert.equal(f.tokenInput.value, "");
  assert.deepEqual(f.navigations, []);
  assert.match(f.status.textContent, /ไม่สามารถเปิด/);
});

test("German status remains readable on server error without navigating", async () => {
  const f = createFixture({ lang: "de", reply: { ok: false } });
  f.tokenInput.value = credential;
  await submit(f);
  assert.deepEqual(f.navigations, []);
  assert.match(f.status.textContent, /konnte nicht/);
});

test("malicious off-origin or different-record server navigation is rejected", async () => {
  for (const unsafe of [
    "https://evil.example.test/private",
    "//evil.example.test/private",
    "/filing/reviews/" + "b".repeat(64),
    "/api/v1/jobs"
  ]) {
    const f = createFixture({
      reply: { ok: true, async json() { return { review_path: unsafe }; } }
    });
    f.tokenInput.value = credential;
    await submit(f);
    assert.deepEqual(f.navigations, []);
    assert.equal(f.requests.length, 1);
    assert.match(f.status.textContent, /Unable to open/);
  }
});

test("network exceptions also erase credential field and do not navigate", async () => {
  const f = createFixture({
    reply: { ok: true, async json() { throw new Error("transport"); } }
  });
  f.tokenInput.value = credential;
  await submit(f);
  assert.equal(f.tokenInput.value, "");
  assert.deepEqual(f.navigations, []);
});
