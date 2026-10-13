"use strict";

// trombi web GUI — single page over the JSON API served by package webserver.
// All processing happens server-side; this file only renders state and
// forwards user actions.

const IMAGE_RE = /\.(jpe?g|png)$/i;
const UPLOAD_CONCURRENCY = 3;
const EDITOR_DEBOUNCE_MS = 150;

const ICON_CROP = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 2v14a2 2 0 0 0 2 2h14"/><path d="M18 22V8a2 2 0 0 0-2-2H2"/></svg>';
const ICON_TRASH = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v6M14 11v6"/></svg>';

const $ = (sel) => document.querySelector(sel);

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

// ---------------------------------------------------------------- API

const api = {
  async request(method, url, body) {
    const opts = { method, headers: {} };
    if (body instanceof FormData) {
      opts.body = body;
    } else if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(url, opts);
    if (!res.ok) {
      let msg = `erreur ${res.status}`;
      try { msg = (await res.json()).error || msg; } catch { /* non-JSON error body */ }
      throw new Error(msg);
    }
    return res.status === 204 ? null : res.json();
  },
  state: () => api.request("GET", "api/state"),
  upload(file, path) {
    const form = new FormData();
    form.append("file", file);
    form.append("path", path);
    return api.request("POST", "api/items", form);
  },
  patch: (id, body) => api.request("PATCH", `api/items/${id}`, body),
  remove: (id) => api.request("DELETE", `api/items/${id}`),
  reset: (id) => api.request("POST", `api/items/${id}/reset`),
  putFraming: (framing) => api.request("PUT", "api/settings/framing", framing),
  putLayout: (layout) => api.request("PUT", "api/settings/layout", layout),
};

// ---------------------------------------------------------------- State

// items mixes server items (numeric id) with local upload placeholders
// ({local: true, id: "up-N"}), replaced by the server item once uploaded.
const state = { framing: null, layout: null, items: [] };
let renderDeferred = false;

function applyServerState(s) {
  state.framing = s.framing;
  state.layout = s.layout;
  // Server items still pending belong to uploads in flight: their local
  // placeholder already stands for them.
  const locals = state.items.filter((i) => i.local);
  state.items = s.items.filter((i) => i.status !== "pending").concat(locals);
}

function replaceItem(oldId, item) {
  const idx = state.items.findIndex((i) => i.id === oldId);
  state.items = state.items.filter((i, k) => k === idx || i.id !== item.id);
  const at = state.items.findIndex((i) => i.id === oldId);
  if (at >= 0) state.items[at] = item; else state.items.push(item);
  render();
}

function findItem(id) {
  return state.items.find((i) => i.id === id);
}

const isUsable = (item) => item.status === "success" || item.status === "warning";

// ---------------------------------------------------------------- Uploads

let uploadSeq = 0;
let activeUploads = 0;
const uploadQueue = [];

function baseName(path) {
  const name = path.split("/").pop();
  return name.replace(/\.[^.]+$/, "");
}

function enqueueUploads(files) {
  const accepted = [];
  for (const f of files) {
    if (IMAGE_RE.test(f.file.name)) {
      accepted.push(f);
    } else {
      state.items.push({
        id: `up-${++uploadSeq}`, local: true, status: "failure",
        name: baseName(f.path), sourcePath: f.path,
        messages: ["Format non supporté : seuls les JPEG et PNG sont pris en charge."],
      });
    }
  }
  accepted.sort((a, b) => a.path.localeCompare(b.path));
  for (const f of accepted) {
    const placeholder = { id: `up-${++uploadSeq}`, local: true, status: "pending", name: baseName(f.path), sourcePath: f.path, messages: [] };
    state.items.push(placeholder);
    uploadQueue.push({ ...f, placeholder });
  }
  render();
  pumpUploads();
}

function pumpUploads() {
  while (activeUploads < UPLOAD_CONCURRENCY && uploadQueue.length > 0) {
    const job = uploadQueue.shift();
    activeUploads++;
    api.upload(job.file, job.path)
      .then((item) => replaceItem(job.placeholder.id, item))
      .catch((err) => {
        state.items = state.items.filter((i) => i.id !== job.placeholder.id);
        render();
        toast(`${job.path} : ${err.message}`);
      })
      .finally(() => {
        activeUploads--;
        pumpUploads();
      });
  }
}

function filesFromInput(input) {
  const files = [...input.files].map((file) => ({ file, path: file.webkitRelativePath || file.name }));
  input.value = "";
  return files;
}

// filesFromDrop walks dropped folders recursively. Entries must be taken
// synchronously from the drop event, before any await.
async function filesFromDrop(dataTransfer) {
  const entries = [...dataTransfer.items]
    .map((item) => (item.webkitGetAsEntry ? item.webkitGetAsEntry() : null))
    .filter(Boolean);
  if (entries.length === 0) {
    return [...dataTransfer.files].map((file) => ({ file, path: file.name }));
  }
  const out = [];
  async function walk(entry, prefix) {
    if (entry.isFile) {
      const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
      out.push({ file, path: prefix + file.name });
    } else if (entry.isDirectory) {
      const reader = entry.createReader();
      let batch;
      do { // readEntries returns its results in chunks
        batch = await new Promise((resolve, reject) => reader.readEntries(resolve, reject));
        for (const child of batch) await walk(child, `${prefix}${entry.name}/`);
      } while (batch.length > 0);
    }
  }
  for (const entry of entries) await walk(entry, "");
  return out;
}

// ---------------------------------------------------------------- Rendering

function render() {
  if (!state.layout) return; // initial state not loaded yet
  if (document.activeElement && document.activeElement.classList.contains("caption")) {
    renderDeferred = true; // rebuilding the sheet would drop the caption being edited
    return;
  }
  renderDeferred = false;
  const hasItems = state.items.length > 0;
  $("#welcome").hidden = hasItems;
  $("#sheet").hidden = !hasItems;
  $("#sheet-actions").hidden = !hasItems;
  renderCounts();
  renderPages();
  renderFailures();
  $("#btn-export").classList.toggle("disabled", !state.items.some(isUsable));
  if (!$("#settings").hidden) fillSettings();
}

function renderCounts() {
  const n = state.items.length;
  if (n === 0) { $("#counts").textContent = ""; return; }
  const count = (status) => state.items.filter((i) => i.status === status).length;
  const parts = [`${n} photo${n > 1 ? "s" : ""}`];
  if (count("pending")) parts.push(`${count("pending")} en cours d'analyse`);
  if (count("warning")) parts.push(`${count("warning")} ⚠`);
  if (count("failure")) parts.push(`${count("failure")} ✖`);
  $("#counts").textContent = parts.join(" · ");
}

function renderPages() {
  const pages = $("#pages");
  const { layout, framing } = state;
  pages.style.setProperty("--cols", layout.cols);
  pages.style.setProperty("--rows", layout.rows);
  pages.style.setProperty("--hgap", layout.hSpacingMm);
  pages.style.setProperty("--vgap", layout.vSpacingMm);
  pages.style.setProperty("--ratio", framing.output.width / framing.output.height);

  // Failed photos are not part of the PDF, hence not of the pages either.
  const placed = state.items.filter((i) => i.status !== "failure");
  const perPage = layout.rows * layout.cols;
  const pageCount = Math.ceil(placed.length / perPage);
  const nodes = [];
  for (let p = 0; p < pageCount; p++) {
    const wrapper = el("div");
    wrapper.append(el("div", "page-label", `Page ${p + 1} / ${pageCount}`));
    const page = el("div", "page");
    for (const item of placed.slice(p * perPage, (p + 1) * perPage)) page.append(renderCell(item));
    wrapper.append(page);
    nodes.push(wrapper);
  }
  pages.replaceChildren(...nodes);
}

function portraitURL(item) {
  const c = item.cropBox;
  const out = state.framing.output;
  return `api/items/${item.id}/portrait.jpg?k=${c.x0}-${c.y0}-${c.x1}-${c.y1}-${out.width}x${out.height}`;
}

function renderCell(item) {
  const cell = el("div", "cell");
  const photo = el("div", "photo");
  const frame = el("div", "frame");
  if (item.status === "pending") {
    const pending = el("div", "pending");
    pending.append(el("span", "spinner"), el("span", "", "Analyse…"));
    frame.append(pending);
  } else {
    const img = el("img");
    img.src = portraitURL(item);
    img.alt = item.name;
    frame.append(img);
  }
  frame.append(renderIconBar(item));
  photo.append(frame);
  cell.append(photo, renderCaption(item));
  return cell;
}

function describeItem(item) {
  const lines = [item.sourcePath];
  if (item.source) lines.push(`${item.source.width} × ${item.source.height} px`);
  if (item.status === "pending") lines.push("Analyse en cours…");
  if (item.status === "success") lines.push("Portrait prêt.");
  return lines.concat(item.messages).join("\n");
}

function renderIconBar(item) {
  const bar = el("div", "iconbar");
  const symbols = { pending: "⏳", success: "✓", warning: "⚠" };
  const status = el("span", `status ${item.status}`, symbols[item.status]);
  status.tabIndex = 0;
  status.dataset.tooltip = describeItem(item);
  bar.append(status);
  if (item.custom) {
    const dot = el("span", "custom-dot", "●");
    dot.title = `Cadrage personnalisé (marges ${item.topMarginPct} % / ${item.bottomMarginPct} %, décalage ${item.horizontalOffsetPct} %)`;
    bar.append(dot);
  }
  bar.append(el("span", "spacer"));
  if (!item.local) {
    if (isUsable(item)) bar.append(iconButton(ICON_CROP, "Modifier le cadrage", () => openEditor(item.id)));
    bar.append(iconButton(ICON_TRASH, "Retirer du trombinoscope", () => removeItem(item.id)));
  }
  return bar;
}

function iconButton(svg, title, onClick) {
  const button = el("button");
  button.type = "button";
  button.title = title;
  button.setAttribute("aria-label", title);
  button.innerHTML = svg;
  button.addEventListener("click", onClick);
  return button;
}

function renderCaption(item) {
  const caption = el("input", "caption");
  caption.value = item.name;
  caption.disabled = !!item.local;
  caption.setAttribute("aria-label", "Nom affiché sous la photo");
  caption.addEventListener("keydown", (e) => {
    if (e.key === "Enter") caption.blur();
    if (e.key === "Escape") { caption.value = item.name; caption.blur(); }
  });
  caption.addEventListener("blur", async () => {
    const name = caption.value.trim();
    if (name !== item.name) {
      try {
        replaceItem(item.id, await api.patch(item.id, { name }));
      } catch (err) {
        caption.value = item.name;
        toast(err.message);
      }
    }
    if (renderDeferred) render();
  });
  return caption;
}

function renderFailures() {
  const failed = state.items.filter((i) => i.status === "failure");
  $("#failures").hidden = failed.length === 0;
  $("#failure-list").replaceChildren(...failed.map((item) => {
    const li = el("li");
    li.title = describeItem(item);
    li.append(
      el("span", "name", item.name),
      el("span", "reason", item.messages.join(" ")),
      iconButton(ICON_TRASH, "Retirer du trombinoscope", () => removeItem(item.id)),
    );
    return li;
  }));
}

async function removeItem(id) {
  // A local item (pending upload placeholder, or a file rejected client-side
  // before ever reaching the server) has nothing to delete server-side.
  if (findItem(id)?.local) {
    state.items = state.items.filter((i) => i.id !== id);
    render();
    return;
  }
  try {
    await api.remove(id);
    state.items = state.items.filter((i) => i.id !== id);
    render();
  } catch (err) {
    toast(err.message);
  }
}

// ---------------------------------------------------------------- Settings

const framingForm = $("#framing-form");
const layoutForm = $("#layout-form");

function ratioValue(r) { return `${r.width}:${r.height}`; }

function fillSettings() {
  const f = state.framing;
  framingForm.top.value = f.topMarginPct;
  framingForm.bottom.value = f.bottomMarginPct;
  framingForm.horizontal.value = f.horizontalOffsetPct;
  const ratio = ratioValue(f.ratio);
  if (![...framingForm.ratio.options].some((o) => o.value === ratio)) framingForm.ratio.add(new Option(ratio, ratio));
  framingForm.ratio.value = ratio;
  framingForm.width.value = f.output.width;
  framingForm.height.value = f.output.height;
  $("#framing-derived").textContent =
    `Actuellement : marge horizontale ≈ ${Math.round(f.derived.horizontalMarginPct)} %, ` +
    `visage centré à ≈ ${Math.round(f.derived.facePositionPct)} % du haut du cadre.`;
  const customCount = state.items.filter((i) => i.custom).length;
  const note = $("#framing-overwrite");
  note.hidden = customCount === 0;
  note.textContent = `${customCount} réglage(s) personnalisé(s) seront écrasés.`;

  const l = state.layout;
  layoutForm.cols.value = l.cols;
  layoutForm.rows.value = l.rows;
  layoutForm.hspacing.value = l.hSpacingMm;
  layoutForm.vspacing.value = l.vSpacingMm;
}

function toggleSettings(open) {
  $("#settings").hidden = !open;
  if (open) fillSettings();
}

framingForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const [rw, rh] = framingForm.ratio.value.split(":").map(Number);
  const framing = {
    topMarginPct: Number(framingForm.top.value),
    bottomMarginPct: Number(framingForm.bottom.value),
    horizontalOffsetPct: Number(framingForm.horizontal.value),
    ratio: { width: rw, height: rh },
    output: { width: Number(framingForm.width.value), height: Number(framingForm.height.value) },
  };
  const button = framingForm.querySelector("button[type=submit]");
  const label = button.textContent;
  button.disabled = true;
  button.textContent = "Recalcul en cours…";
  try {
    applyServerState(await api.putFraming(framing));
    render();
    toast("Cadrage appliqué à toutes les photos.");
  } catch (err) {
    toast(err.message);
  } finally {
    button.disabled = false;
    button.textContent = label;
  }
});

layoutForm.addEventListener("change", async () => {
  const layout = {
    cols: Number(layoutForm.cols.value),
    rows: Number(layoutForm.rows.value),
    hSpacingMm: Number(layoutForm.hspacing.value),
    vSpacingMm: Number(layoutForm.vspacing.value),
  };
  try {
    state.layout = await api.putLayout(layout);
    render();
  } catch (err) {
    toast(err.message);
    fillSettings();
  }
});

// ---------------------------------------------------------------- Framing editor

const dialog = $("#editor");
const editorForm = dialog.querySelector("form");
const editor = { id: null, original: null, seq: 0, timer: null };

function openEditor(id) {
  const item = findItem(id);
  editor.id = id;
  editor.original = { top: item.topMarginPct, bottom: item.bottomMarginPct, horizontal: item.horizontalOffsetPct, custom: item.custom };
  editor.seq++;
  $("#editor-name").textContent = item.name;
  $("#editor-source").src = `api/items/${id}/source.jpg`;
  $("#editor-boxes").setAttribute("viewBox", `0 0 ${item.source.width} ${item.source.height}`);
  $("#editor-top-default").textContent = `(défaut : ${state.framing.topMarginPct} %)`;
  $("#editor-bottom-default").textContent = `(défaut : ${state.framing.bottomMarginPct} %)`;
  $("#editor-horizontal-default").textContent = `(défaut : ${state.framing.horizontalOffsetPct} %)`;
  setEditorSliders(item);
  showEditorItem(item);
  dialog.showModal();
}

function setEditorSliders(item) {
  for (const [name, value] of [["top", item.topMarginPct], ["bottom", item.bottomMarginPct], ["horizontal", item.horizontalOffsetPct]]) {
    const slider = editorForm[name];
    slider.max = Math.max(Number(slider.max), Math.ceil(value));
    slider.min = Math.min(Number(slider.min), Math.floor(value));
    slider.value = value;
  }
  updateSliderOutputs();
}

function updateSliderOutputs() {
  editorForm.topOut.value = `${editorForm.top.value} %`;
  editorForm.bottomOut.value = `${editorForm.bottom.value} %`;
  const h = Number(editorForm.horizontal.value);
  editorForm.horizontalOut.value = `${h > 0 ? "+" : ""}${h} %`;
}

function boxRect(box, className) {
  const rect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
  rect.setAttribute("class", className);
  rect.setAttribute("x", box.x0);
  rect.setAttribute("y", box.y0);
  rect.setAttribute("width", box.x1 - box.x0);
  rect.setAttribute("height", box.y1 - box.y0);
  return rect;
}

function showEditorItem(item) {
  // Boxes are in full-resolution source pixels; the SVG viewBox maps them
  // onto the downscaled preview.
  const rects = [];
  if (item.faceBox) rects.push(boxRect(item.faceBox, "face"));
  if (item.cropBox) rects.push(boxRect(item.cropBox, "crop"));
  $("#editor-boxes").replaceChildren(...rects);
  const portrait = $("#editor-portrait");
  portrait.hidden = !item.cropBox;
  if (item.cropBox) portrait.src = portraitURL(item);
  $("#editor-messages").replaceChildren(...item.messages.map((m) => el("li", "", m)));
  $("#editor-derived").textContent =
    `Marge horizontale ≈ ${Math.round(item.derived.horizontalMarginPct)} %, ` +
    `visage centré à ≈ ${Math.round(item.derived.facePositionPct)} % du haut du cadre.`;
}

async function editorRequest(call) {
  const seq = ++editor.seq;
  try {
    const item = await call();
    replaceItem(editor.id, item);
    if (seq === editor.seq) showEditorItem(item); // ignore superseded responses
    return item;
  } catch (err) {
    toast(err.message);
    return null;
  }
}

function sendEditorFraming() {
  clearTimeout(editor.timer);
  editor.timer = null;
  const id = editor.id;
  const body = {
    topMarginPct: Number(editorForm.top.value),
    bottomMarginPct: Number(editorForm.bottom.value),
    horizontalOffsetPct: Number(editorForm.horizontal.value),
  };
  return editorRequest(() => api.patch(id, body));
}

editorForm.addEventListener("input", (e) => {
  if (e.target.type !== "range") return;
  updateSliderOutputs();
  clearTimeout(editor.timer);
  editor.timer = setTimeout(sendEditorFraming, EDITOR_DEBOUNCE_MS);
});

editorForm.addEventListener("submit", () => {
  if (editor.timer) sendEditorFraming(); // flush the last slider move
});

editorForm.querySelector("[data-action=reset]").addEventListener("click", async () => {
  clearTimeout(editor.timer);
  const id = editor.id;
  const item = await editorRequest(() => api.reset(id));
  if (item) setEditorSliders(item);
});

async function cancelEditor() {
  clearTimeout(editor.timer);
  const { id, original } = editor;
  dialog.close();
  const item = findItem(id);
  if (!item || (item.topMarginPct === original.top && item.bottomMarginPct === original.bottom && item.custom === original.custom)) return;
  try {
    const restored = original.custom
      ? await api.patch(id, { topMarginPct: original.top, bottomMarginPct: original.bottom })
      : await api.reset(id);
    replaceItem(id, restored);
  } catch (err) {
    toast(err.message);
  }
}

for (const button of dialog.querySelectorAll("[data-action=cancel]")) button.addEventListener("click", cancelEditor);
dialog.addEventListener("cancel", (e) => { e.preventDefault(); cancelEditor(); });

// ---------------------------------------------------------------- Toast

let toastTimer = null;
function toast(message) {
  const node = $("#toast");
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { node.hidden = true; }, 6000);
}

// ---------------------------------------------------------------- Wiring

for (const button of document.querySelectorAll("[data-pick]")) {
  button.addEventListener("click", () => $(button.dataset.pick === "folder" ? "#pick-folder" : "#pick-files").click());
}
for (const input of [$("#pick-files"), $("#pick-folder")]) {
  input.addEventListener("change", () => enqueueUploads(filesFromInput(input)));
}

$("#btn-settings").addEventListener("click", () => toggleSettings($("#settings").hidden));
$("#btn-settings-close").addEventListener("click", () => toggleSettings(false));

const dropOverlay = $("#drop-overlay");
const draggingFiles = (e) => e.dataTransfer && [...e.dataTransfer.types].includes("Files");
window.addEventListener("dragover", (e) => {
  if (!draggingFiles(e) || dialog.open) return;
  e.preventDefault();
  dropOverlay.hidden = false;
});
window.addEventListener("dragleave", (e) => {
  if (e.relatedTarget === null) dropOverlay.hidden = true; // left the window
});
window.addEventListener("drop", async (e) => {
  if (!draggingFiles(e)) return;
  e.preventDefault();
  dropOverlay.hidden = true;
  if (dialog.open) return;
  enqueueUploads(await filesFromDrop(e.dataTransfer));
});

const exportDialog = $("#export-progress");
const btnExport = $("#btn-export");
btnExport.addEventListener("click", async (e) => {
  e.preventDefault();
  if (btnExport.classList.contains("disabled")) return;
  btnExport.classList.add("disabled");
  exportDialog.showModal();
  try {
    const res = await fetch(btnExport.href);
    if (!res.ok) {
      let msg = `erreur ${res.status}`;
      try { msg = (await res.json()).error || msg; } catch { /* non-JSON error body */ }
      throw new Error(msg);
    }
    const url = URL.createObjectURL(await res.blob());
    const a = el("a");
    a.href = url;
    a.download = "trombinoscope.pdf";
    a.click();
    URL.revokeObjectURL(url);
  } catch (err) {
    toast(err.message);
  } finally {
    exportDialog.close();
    btnExport.classList.toggle("disabled", !state.items.some(isUsable));
  }
});

api.state()
  .then((s) => { applyServerState(s); render(); })
  .catch((err) => toast(`Impossible de charger l'état : ${err.message}`));
