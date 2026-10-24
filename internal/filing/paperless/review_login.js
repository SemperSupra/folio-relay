"use strict";
// This public helper never stores, logs, places in URLs or transmits the
// bearer outside the verified same-origin TLS-only session exchange.
// A separate trusted actor/grant issuer must supply the credential.
(() => {
  const form = document.querySelector("[data-review-form]");
  const effectInput = document.getElementById("effect");
  const tokenInput = document.getElementById("grant");
  const status = document.querySelector("[data-status]");
  if (!form || !effectInput || !tokenInput || !status) return;
  const messages = {
    en: ["Invalid filing record or credential.", "Checking record-specific permission…", "Unable to open review. Confirm access and try again."],
    de: ["Ungültige Datensatzkennung oder Berechtigung.", "Zugriffsberechtigung wird geprüft…", "Die Prüfung konnte nicht geöffnet werden. Bitte Berechtigung prüfen."],
    th: ["รหัสรายการหรือสิทธิ์ไม่ถูกต้อง", "กำลังตรวจสอบสิทธิ์สำหรับเอกสารรายการนี้…", "ไม่สามารถเปิดหน้าตรวจสอบได้ กรุณาตรวจสอบสิทธิ์แล้วลองอีกครั้ง"]
  };
  const labels = messages[document.documentElement.lang] || messages.en;
  const candidate = new URLSearchParams(window.location.search).get("effect") || "";
  if (/^[a-f0-9]{64}$/.test(candidate)) effectInput.value = candidate;
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const effect = effectInput.value.trim();
    let credential = tokenInput.value.trim();
    tokenInput.value = "";
    if (!/^[a-f0-9]{64}$/.test(effect) || !/^[A-Za-z0-9_-]{43}$/.test(credential)) {
      status.textContent = labels[0];
      credential = "";
      return;
    }
    status.textContent = labels[1];
    try {
      const response = await fetch("/filing/review-sessions", {
        method: "POST",
        mode: "same-origin",
        credentials: "same-origin",
        redirect: "error",
        cache: "no-store",
        referrerPolicy: "no-referrer",
        headers: {
          "Content-Type": "application/json",
          "X-FolioRelay-Review-Session": "1",
          "Authorization": "Bearer " + credential
        },
        body: JSON.stringify({effect_id: effect})
      });
      credential = "";
      if (!response.ok) throw new Error("denied");
      const reply = await response.json();
      const target = "/filing/reviews/" + effect;
      if (reply.review_path !== target) throw new Error("invalid path");
      window.location.assign(target);
    } catch (_) {
      credential = "";
      status.textContent = labels[2];
    }
  });
})();
