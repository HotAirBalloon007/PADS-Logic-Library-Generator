const $ = (id) => document.getElementById(id);

let pins = [];
let selectedFile = null;
let selectedMaterialFile = null;
let currentLang = localStorage.getItem("padslogic.lang") || "zh";

const i18n = {
  zh: {
    appTitle: "PADS Logic 封装生成器",
    subtitle: "拖入芯片 pinout 截图，校对引脚表，生成 .c / .p 库文件。",
    languageButton: "English",
    loadConfig: "读取配置",
    apiTitle: "大模型 API",
    visionModelPlaceholder: "gpt-4o / qwen-vl-plus，空则使用 Text Model",
    textModelPickerLabel: "Text Model 列表",
    visionModelPickerLabel: "Vision Model 列表",
    modelSelectPlaceholder: "先获取模型，或手工填写",
    loadModels: "获取模型",
    saveConfig: "保存配置",
    testApi: "测试接口",
    testVision: "测试视觉",
    materialTitle: "物料信息",
    materialDropText: "可手工填写，也可拖入物料信息截图识别",
    parseMaterial: "识别物料信息",
    lcscNumber: "商品编号",
    packageDesc: "封装描述",
    pcbDecalName: "PCB Decal 名称",
    pinoutTitle: "截图识别",
    pinoutDropStrong: "拖拽 pinout 图片到这里",
    pinoutDropHint: "或点击选择文件。识别后必须人工校对。",
    parsePinout: "调用大模型识别",
    addPin: "新增引脚",
    normalizePins: "规范化 Pin 名",
    pinTableTitle: "Pin 表校对",
    generate: "生成 .c / .p",
    download: "下载",
    auto: "auto",
    delete: "删除",
    configLoaded: "配置已读取",
    configSaved: "配置已保存",
    loadingModels: "正在获取模型...",
    modelsLoaded: "已获取 {count} 个模型",
    loadModelsFailed: "获取模型失败：{message}",
    testing: "测试中...",
    apiOk: "接口可用：{reply}，耗时 {elapsed}s",
    testFailed: "测试失败：{message}",
    visionTesting: "视觉测试中...",
    visionOk: "视觉接口可用：{reply}，耗时 {elapsed}s",
    visionFailed: "视觉测试失败：{message}",
    selectedFile: "已选择 {name}",
    chooseMaterialFirst: "请先选择物料信息截图",
    compressing: "正在压缩图片...",
    recognizingWithSize: "正在识别... ({size} KB)",
    materialFilled: "物料信息已填入，可继续手工修改",
    recognizeFailed: "识别失败：{message}",
    choosePinoutFirst: "请先选择 pinout 图片",
    pinoutDone: "识别完成，请人工校对",
    pinsNormalized: "Pin 名已规范化",
    generating: "正在生成...",
    generated: "已生成 {logic} / {part}",
  },
  en: {
    appTitle: "PADS Logic Library Generator",
    subtitle: "Drop a chip pinout screenshot, review the pin table, and generate .c / .p library files.",
    languageButton: "中文",
    loadConfig: "Load Config",
    apiTitle: "Model API",
    visionModelPlaceholder: "gpt-4o / qwen-vl-plus, empty means Text Model",
    textModelPickerLabel: "Text Model List",
    visionModelPickerLabel: "Vision Model List",
    modelSelectPlaceholder: "Fetch models first, or type manually",
    loadModels: "Fetch Models",
    saveConfig: "Save Config",
    testApi: "Test API",
    testVision: "Test Vision",
    materialTitle: "Part Information",
    materialDropText: "Fill manually or drop a part information screenshot for recognition",
    parseMaterial: "Recognize Part Info",
    lcscNumber: "LCSC Number",
    packageDesc: "Package Description",
    pcbDecalName: "PCB Decal Name",
    pinoutTitle: "Screenshot Recognition",
    pinoutDropStrong: "Drop pinout image here",
    pinoutDropHint: "Or click to choose a file. Manual review is required after recognition.",
    parsePinout: "Run Model Recognition",
    addPin: "Add Pin",
    normalizePins: "Normalize Pin Names",
    pinTableTitle: "Pin Table Review",
    generate: "Generate .c / .p",
    download: "Download",
    auto: "auto",
    delete: "Delete",
    configLoaded: "Config loaded",
    configSaved: "Config saved",
    loadingModels: "Fetching models...",
    modelsLoaded: "Fetched {count} models",
    loadModelsFailed: "Failed to fetch models: {message}",
    testing: "Testing...",
    apiOk: "API available: {reply}, elapsed {elapsed}s",
    testFailed: "Test failed: {message}",
    visionTesting: "Testing vision...",
    visionOk: "Vision API available: {reply}, elapsed {elapsed}s",
    visionFailed: "Vision test failed: {message}",
    selectedFile: "Selected {name}",
    chooseMaterialFirst: "Please choose a part information screenshot first",
    compressing: "Compressing image...",
    recognizingWithSize: "Recognizing... ({size} KB)",
    materialFilled: "Part information has been filled. You can still edit it manually",
    recognizeFailed: "Recognition failed: {message}",
    choosePinoutFirst: "Please choose a pinout image first",
    pinoutDone: "Recognition complete. Please review manually",
    pinsNormalized: "Pin names normalized",
    generating: "Generating...",
    generated: "Generated {logic} / {part}",
  },
};

function t(key, vars = {}) {
  const template = i18n[currentLang]?.[key] || i18n.zh[key] || key;
  return template.replace(/\{(\w+)\}/g, (_, name) => vars[name] ?? "");
}

function applyLanguage() {
  document.documentElement.lang = currentLang === "zh" ? "zh-CN" : "en";
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.textContent = t(el.dataset.i18n);
  });
  document.querySelectorAll("[data-i18n-placeholder]").forEach((el) => {
    el.placeholder = t(el.dataset.i18nPlaceholder);
  });
  $("languageBtn").textContent = t("languageButton");
  updateModelSelects();
  renderPins();
}

const fields = {
  baseUrl: $("baseUrl"),
  apiKey: $("apiKey"),
  model: $("model"),
  modelSelect: $("modelSelect"),
  visionModel: $("visionModel"),
  visionModelSelect: $("visionModelSelect"),
  temperature: $("temperature"),
  timeoutSec: $("timeoutSec"),
  partName: $("partName"),
  mpn: $("mpn"),
  lcscNumber: $("lcscNumber"),
  value: $("value"),
  packageName: $("packageName"),
  pcbDecalName: $("pcbDecalName"),
  description: $("description"),
};

let availableModels = [];

function setStatus(text) {
  $("status").textContent = text || "";
}

function setApiStatus(text) {
  $("apiStatus").textContent = text || "";
}

function setMaterialStatus(text) {
  $("materialStatus").textContent = text || "";
}

async function requestJSON(url, options = {}) {
  const res = await fetch(url, options);
  const text = await res.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { raw: text };
  }
  if (!res.ok) {
    throw new Error(data?.error || text || res.statusText);
  }
  return data;
}

function getPartInfo() {
  return {
    part_name: fields.partName.value.trim(),
    mpn: fields.mpn.value.trim(),
    lcsc_number: fields.lcscNumber.value.trim(),
    package_name: fields.packageName.value.trim(),
    pcb_decal_name: fields.pcbDecalName.value.trim(),
    description: fields.description.value.trim(),
    value: fields.value.value.trim(),
    pins: pins.map((p) => ({
      number: Number(p.number),
      name: String(p.name || "").trim(),
      side: String(p.side || "").trim(),
      hidden: Boolean(p.hidden),
    })),
  };
}

function appendModelConfig(form) {
  form.append("base_url", fields.baseUrl.value.trim());
  form.append("api_key", fields.apiKey.value.trim());
  form.append("model", fields.model.value.trim());
  form.append("vision_model", fields.visionModel.value.trim());
  form.append("temperature", String(Number(fields.temperature.value || 0)));
  form.append("timeout_sec", String(Number(fields.timeoutSec.value || 60)));
}

function currentModelConfig(timeoutFallback = 60) {
  return {
    base_url: fields.baseUrl.value.trim(),
    api_key: fields.apiKey.value.trim(),
    model: fields.model.value.trim(),
    vision_model: fields.visionModel.value.trim(),
    temperature: Number(fields.temperature.value || 0),
    timeout_sec: Number(fields.timeoutSec.value || timeoutFallback),
  };
}

function updateModelSelects() {
  const placeholder = t("modelSelectPlaceholder");
  fillModelSelect(fields.modelSelect, fields.model.value.trim(), placeholder);
  fillModelSelect(fields.visionModelSelect, fields.visionModel.value.trim(), placeholder);
}

function fillModelSelect(select, selectedValue, placeholder) {
  if (!select) return;
  select.innerHTML = "";
  const empty = document.createElement("option");
  empty.value = "";
  empty.textContent = placeholder;
  select.appendChild(empty);
  availableModels.forEach((model) => {
    const option = document.createElement("option");
    option.value = model;
    option.textContent = model;
    if (model === selectedValue) {
      option.selected = true;
    }
    select.appendChild(option);
  });
}

async function compressImageFile(file, maxEdge = 1400, quality = 0.82) {
  if (!file || !file.type.startsWith("image/")) return file;
  const bitmap = await createImageBitmap(file);
  const scale = Math.min(1, maxEdge / Math.max(bitmap.width, bitmap.height));
  const width = Math.max(1, Math.round(bitmap.width * scale));
  const height = Math.max(1, Math.round(bitmap.height * scale));
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, width, height);
  ctx.drawImage(bitmap, 0, 0, width, height);
  const blob = await new Promise((resolve) => canvas.toBlob(resolve, "image/jpeg", quality));
  if (!blob) return file;
  return new File([blob], file.name.replace(/\.[^.]+$/, "") + "_compressed.jpg", { type: "image/jpeg" });
}

function applyPartInfo(part) {
  fields.partName.value = part.part_name || fields.partName.value;
  fields.mpn.value = part.mpn || fields.mpn.value;
  fields.lcscNumber.value = part.lcsc_number || fields.lcscNumber.value;
  fields.packageName.value = part.package_name || fields.packageName.value;
  fields.pcbDecalName.value = part.pcb_decal_name || fields.pcbDecalName.value;
  fields.description.value = part.description || fields.description.value;
  fields.value.value = part.value || fields.value.value || part.part_name || "";
  pins = Array.isArray(part.pins) ? part.pins : [];
  renderPins();
}

function renderPins() {
  const tbody = $("pinTable");
  tbody.innerHTML = "";
  pins
    .slice()
    .sort((a, b) => Number(a.number) - Number(b.number))
    .forEach((pin, index) => {
      const originalIndex = pins.indexOf(pin);
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td class="pinNo"><input type="number" value="${pin.number || ""}" /></td>
        <td><input value="${escapeAttr(pin.name || "")}" /></td>
        <td class="pinSide">
          <select>
            ${["", "left", "right", "top", "bottom", "hidden"].map((s) => `<option value="${s}" ${pin.side === s ? "selected" : ""}>${s || t("auto")}</option>`).join("")}
          </select>
        </td>
        <td class="pinHidden"><input type="checkbox" ${pin.hidden ? "checked" : ""} /></td>
        <td class="pinDelete"><button class="secondary" type="button">${t("delete")}</button></td>
      `;
      const inputs = tr.querySelectorAll("input,select");
      inputs[0].addEventListener("input", (e) => (pins[originalIndex].number = Number(e.target.value)));
      inputs[1].addEventListener("input", (e) => (pins[originalIndex].name = e.target.value));
      inputs[2].addEventListener("change", (e) => (pins[originalIndex].side = e.target.value));
      inputs[3].addEventListener("change", (e) => (pins[originalIndex].hidden = e.target.checked));
      tr.querySelector("button").addEventListener("click", () => {
        pins.splice(originalIndex, 1);
        renderPins();
      });
      tbody.appendChild(tr);
    });
}

function escapeAttr(s) {
  return String(s).replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;");
}

$("loadConfigBtn").addEventListener("click", async () => {
  try {
    const cfg = await requestJSON("/api/config");
    fields.baseUrl.value = cfg.base_url || "";
    fields.apiKey.value = cfg.api_key || "";
    fields.model.value = cfg.model || "";
    fields.visionModel.value = cfg.vision_model || "";
    fields.temperature.value = cfg.temperature ?? 0;
    fields.timeoutSec.value = cfg.timeout_sec || 60;
    updateModelSelects();
    setStatus(t("configLoaded"));
  } catch (err) {
    setStatus(err.message);
  }
});

$("languageBtn").addEventListener("click", () => {
  currentLang = currentLang === "zh" ? "en" : "zh";
  localStorage.setItem("padslogic.lang", currentLang);
  applyLanguage();
});

$("loadModelsBtn").addEventListener("click", async () => {
  try {
    setApiStatus(t("loadingModels"));
    const data = await requestJSON("/api/models", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(currentModelConfig(30)),
    });
    availableModels = Array.isArray(data.models) ? data.models : [];
    updateModelSelects();
    if (!fields.model.value.trim() && availableModels.length > 0) {
      fields.model.value = availableModels[0];
      updateModelSelects();
    }
    setApiStatus(t("modelsLoaded", { count: availableModels.length }));
  } catch (err) {
    setApiStatus(t("loadModelsFailed", { message: err.message }));
  }
});

$("modelSelect").addEventListener("change", (e) => {
  if (e.target.value) {
    fields.model.value = e.target.value;
  }
});

$("visionModelSelect").addEventListener("change", (e) => {
  if (e.target.value) {
    fields.visionModel.value = e.target.value;
  }
});

$("model").addEventListener("input", updateModelSelects);
$("visionModel").addEventListener("input", updateModelSelects);

$("saveConfigBtn").addEventListener("click", async () => {
  try {
    await requestJSON("/api/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        base_url: fields.baseUrl.value.trim(),
        api_key: fields.apiKey.value.trim(),
        model: fields.model.value.trim(),
        vision_model: fields.visionModel.value.trim(),
        temperature: Number(fields.temperature.value || 0),
        timeout_sec: Number(fields.timeoutSec.value || 60),
      }),
    });
    setStatus(t("configSaved"));
  } catch (err) {
    setStatus(err.message);
  }
});

$("testConfigBtn").addEventListener("click", async () => {
  try {
    const started = performance.now();
    setApiStatus(t("testing"));
    const data = await requestJSON("/api/test-model", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(currentModelConfig(30)),
    });
    const elapsed = ((performance.now() - started) / 1000).toFixed(1);
    setApiStatus(t("apiOk", { reply: data.reply || data.status, elapsed }));
  } catch (err) {
    setApiStatus(t("testFailed", { message: err.message }));
  }
});

$("testVisionBtn").addEventListener("click", async () => {
  try {
    const started = performance.now();
    setApiStatus(t("visionTesting"));
    const data = await requestJSON("/api/test-vision", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(currentModelConfig(60)),
    });
    const elapsed = ((performance.now() - started) / 1000).toFixed(1);
    setApiStatus(t("visionOk", { reply: data.reply || data.status, elapsed }));
  } catch (err) {
    setApiStatus(t("visionFailed", { message: err.message }));
  }
});

$("imageInput").addEventListener("change", (e) => {
  selectedFile = e.target.files?.[0] || null;
  setStatus(selectedFile ? t("selectedFile", { name: selectedFile.name }) : "");
});

$("materialImageInput").addEventListener("change", (e) => {
  selectedMaterialFile = e.target.files?.[0] || null;
  setMaterialStatus(selectedMaterialFile ? t("selectedFile", { name: selectedMaterialFile.name }) : "");
});

$("materialDropZone").addEventListener("dragover", (e) => {
  e.preventDefault();
});

$("materialDropZone").addEventListener("drop", (e) => {
  e.preventDefault();
  selectedMaterialFile = e.dataTransfer.files?.[0] || null;
  setMaterialStatus(selectedMaterialFile ? t("selectedFile", { name: selectedMaterialFile.name }) : "");
});

$("parseMaterialBtn").addEventListener("click", async () => {
  if (!selectedMaterialFile) {
    setMaterialStatus(t("chooseMaterialFirst"));
    return;
  }
  const form = new FormData();
  setMaterialStatus(t("compressing"));
  const uploadFile = await compressImageFile(selectedMaterialFile);
  form.append("image", uploadFile);
  appendModelConfig(form);
  const part = getPartInfo();
  Object.entries({
    part_name: part.part_name,
    mpn: part.mpn,
    lcsc_number: part.lcsc_number,
    package_name: part.package_name,
    pcb_decal_name: part.pcb_decal_name,
    description: part.description,
    value: part.value,
  }).forEach(([k, v]) => form.append(k, v || ""));
  try {
    setMaterialStatus(t("recognizingWithSize", { size: Math.round(uploadFile.size / 1024) }));
    const data = await requestJSON("/api/parse-material", { method: "POST", body: form });
    applyPartInfo({ ...getPartInfo(), ...data.part, pins });
    setMaterialStatus(t("materialFilled"));
  } catch (err) {
    setMaterialStatus(t("recognizeFailed", { message: err.message }));
  }
});

$("dropZone").addEventListener("dragover", (e) => {
  e.preventDefault();
});

$("dropZone").addEventListener("drop", (e) => {
  e.preventDefault();
  selectedFile = e.dataTransfer.files?.[0] || null;
  setStatus(selectedFile ? t("selectedFile", { name: selectedFile.name }) : "");
});

$("parseBtn").addEventListener("click", async () => {
  if (!selectedFile) {
    setStatus(t("choosePinoutFirst"));
    return;
  }
  const form = new FormData();
  setStatus(t("compressing"));
  const uploadFile = await compressImageFile(selectedFile);
  form.append("image", uploadFile);
  appendModelConfig(form);
  const part = getPartInfo();
  Object.entries({
    part_name: part.part_name,
    mpn: part.mpn,
    lcsc_number: part.lcsc_number,
    package_name: part.package_name,
    pcb_decal_name: part.pcb_decal_name,
    description: part.description,
    value: part.value,
  }).forEach(([k, v]) => form.append(k, v || ""));
  try {
    setStatus(t("recognizingWithSize", { size: Math.round(uploadFile.size / 1024) }));
    const data = await requestJSON("/api/parse-image", { method: "POST", body: form });
    applyPartInfo(data.part);
    setStatus(t("pinoutDone"));
  } catch (err) {
    setStatus(err.message);
  }
});

$("addPinBtn").addEventListener("click", () => {
  const next = pins.reduce((m, p) => Math.max(m, Number(p.number) || 0), 0) + 1;
  pins.push({ number: next, name: "", side: "", hidden: false });
  renderPins();
});

$("normalizeBtn").addEventListener("click", async () => {
  try {
    const data = await requestJSON("/api/normalize-pins", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(getPartInfo()),
    });
    applyPartInfo(data);
    setStatus(t("pinsNormalized"));
  } catch (err) {
    setStatus(err.message);
  }
});

$("generateBtn").addEventListener("click", async () => {
  try {
    setStatus(t("generating"));
    const data = await requestJSON("/api/generate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(getPartInfo()),
    });
    $("logicText").value = data.logic_text || "";
    $("partText").value = data.part_text || "";
    $("logicDownload").hidden = false;
    $("partDownload").hidden = false;
    $("logicDownload").href = `/api/download/${encodeURIComponent(data.logic_file)}`;
    $("partDownload").href = `/api/download/${encodeURIComponent(data.part_file)}`;
    $("logicDownload").download = data.logic_file;
    $("partDownload").download = data.part_file;
    setStatus(t("generated", { logic: data.logic_file, part: data.part_file }));
  } catch (err) {
    setStatus(err.message);
  }
});

window.addEventListener("DOMContentLoaded", () => {
  applyLanguage();
  $("loadConfigBtn").click();
});
