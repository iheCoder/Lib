(() => {
  const { STORAGE_KEYS, DEFAULT_PREFERENCES, normalizePreferences } =
    globalThis.ChatGptNewTabSettings;
  const MAX_INPUT_BYTES = 30 * 1024 * 1024;
  const MAX_OUTPUT_BYTES = 6 * 1024 * 1024;
  const MAX_EDGE = 3200;
  const MAX_PIXELS = 7_000_000;
  const SUPPORTED_TYPES = new Set(["image/jpeg", "image/png", "image/webp", "image/avif"]);

  const elements = {
    enabled: document.querySelector("#enabled"),
    file: document.querySelector("#image-file"),
    dropZone: document.querySelector("#drop-zone"),
    preview: document.querySelector(".image-picker__preview"),
    imageTitle: document.querySelector("#image-title"),
    imageHelp: document.querySelector("#image-help"),
    fit: document.querySelector("#fit"),
    dimming: document.querySelector("#dimming"),
    dimmingValue: document.querySelector("#dimming-value"),
    blur: document.querySelector("#blur"),
    blurValue: document.querySelector("#blur-value"),
    surfaceOpacity: document.querySelector("#surface-opacity"),
    surfaceValue: document.querySelector("#surface-value"),
    status: document.querySelector("#status"),
    remove: document.querySelector("#remove-image"),
  };

  let saveTimer;

  /** Render controls from normalized state so storage and UI cannot diverge. */
  function renderPreferences(preferences) {
    elements.enabled.checked = preferences.enabled;
    elements.fit.value = preferences.fit;
    elements.dimming.value = String(preferences.dimming);
    elements.blur.value = String(preferences.blur);
    elements.surfaceOpacity.value = String(preferences.surfaceOpacity);
    elements.dimmingValue.value = `${preferences.dimming}%`;
    elements.blurValue.value = `${preferences.blur} px`;
    elements.surfaceValue.value = `${preferences.surfaceOpacity}%`;
  }

  /** Reflect the selected image without exposing its local filesystem path. */
  function renderImage(image, fileName = "") {
    elements.preview.style.backgroundImage = image ? `url("${image}")` : "none";
    elements.dropZone.classList.toggle("has-image", Boolean(image));
    elements.imageTitle.textContent = image ? fileName || "自定义背景" : "选择本地图片";
    elements.imageHelp.textContent = image ? "点击可替换图片" : "PNG、JPEG、WebP 或 AVIF";
    elements.remove.hidden = !image;
  }

  /** Gather only explicit UI values and normalize them before persistence. */
  function readPreferencesFromUi() {
    return normalizePreferences({
      enabled: elements.enabled.checked,
      fit: elements.fit.value,
      dimming: elements.dimming.value,
      blur: elements.blur.value,
      surfaceOpacity: elements.surfaceOpacity.value,
    });
  }

  /** Coalesce range input events to avoid needlessly rewriting extension storage. */
  function schedulePreferencesSave(immediate = false) {
    window.clearTimeout(saveTimer);
    const save = () =>
      chrome.storage.local.set({
        [STORAGE_KEYS.preferences]: readPreferencesFromUi(),
      });
    if (immediate) {
      void save();
      return;
    }
    saveTimer = window.setTimeout(() => void save(), 120);
  }

  /** Resize large photographs before storage to stay below Chrome's local quota. */
  function calculateOutputSize(width, height) {
    const edgeScale = Math.min(1, MAX_EDGE / Math.max(width, height));
    const pixelScale = Math.min(1, Math.sqrt(MAX_PIXELS / (width * height)));
    const scale = Math.min(edgeScale, pixelScale);
    return {
      width: Math.max(1, Math.round(width * scale)),
      height: Math.max(1, Math.round(height * scale)),
    };
  }

  /** Canvas encoding standardizes image formats and strips unnecessary metadata. */
  async function compressImage(file) {
    const bitmap = await createImageBitmap(file);
    const size = calculateOutputSize(bitmap.width, bitmap.height);
    const canvas = document.createElement("canvas");
    canvas.width = size.width;
    canvas.height = size.height;
    const context = canvas.getContext("2d", { alpha: true });
    if (!context) {
      bitmap.close();
      throw new Error("浏览器无法创建图片处理画布");
    }
    context.drawImage(bitmap, 0, 0, size.width, size.height);
    bitmap.close();

    for (const quality of [0.86, 0.72, 0.58]) {
      const blob = await canvasToBlob(canvas, quality);
      if (blob.size <= MAX_OUTPUT_BYTES) {
        return blob;
      }
    }
    throw new Error("压缩后图片仍然过大，请选择尺寸更小的图片");
  }

  /** Convert callback-based canvas encoding into an explicit failure boundary. */
  function canvasToBlob(canvas, quality) {
    return new Promise((resolve, reject) => {
      canvas.toBlob(
        (blob) => (blob ? resolve(blob) : reject(new Error("浏览器无法处理这张图片"))),
        "image/webp",
        quality,
      );
    });
  }

  /** Data URLs are portable inside chrome.storage and require no file access. */
  function blobToDataUrl(blob) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.addEventListener("load", () => resolve(String(reader.result)), { once: true });
      reader.addEventListener("error", () => reject(new Error("无法读取图片")), { once: true });
      reader.readAsDataURL(blob);
    });
  }

  /** Validate, compress and persist one user-selected local image. */
  async function saveImage(file) {
    if (!SUPPORTED_TYPES.has(file.type)) {
      throw new Error("请选择 PNG、JPEG、WebP 或 AVIF 图片");
    }
    if (file.size > MAX_INPUT_BYTES) {
      throw new Error("原始图片不能超过 30 MB");
    }

    const image = await blobToDataUrl(await compressImage(file));
    await chrome.storage.local.set({
      [STORAGE_KEYS.image]: image,
      [STORAGE_KEYS.fileName]: file.name,
      [STORAGE_KEYS.preferences]: { ...readPreferencesFromUi(), enabled: true },
    });
    elements.enabled.checked = true;
    renderImage(image, file.name);
  }

  /** Keep status copy concise while preserving actionable image errors. */
  function setStatus(message, isError = false) {
    elements.status.textContent = message;
    elements.status.classList.toggle("is-error", isError);
  }

  async function handleSelectedFile(file) {
    if (!file) return;
    setStatus("正在优化图片…");
    try {
      await saveImage(file);
      setStatus("背景已保存，并同步到 ChatGPT 标签页");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "无法保存图片", true);
    } finally {
      elements.file.value = "";
    }
  }

  /** Wire controls once; all page updates then flow through chrome.storage. */
  function bindEvents() {
    elements.file.addEventListener("change", () => void handleSelectedFile(elements.file.files[0]));
    elements.enabled.addEventListener("change", () => schedulePreferencesSave(true));
    elements.fit.addEventListener("change", () => schedulePreferencesSave(true));

    for (const input of [elements.dimming, elements.blur, elements.surfaceOpacity]) {
      input.addEventListener("input", () => {
        renderPreferences(readPreferencesFromUi());
        schedulePreferencesSave();
      });
      input.addEventListener("change", () => schedulePreferencesSave(true));
    }

    elements.remove.addEventListener("click", async () => {
      await chrome.storage.local.remove([STORAGE_KEYS.image, STORAGE_KEYS.fileName]);
      renderImage("");
      setStatus("已恢复 ChatGPT 原始背景");
    });
  }

  /** Restore the popup from local state every time the short-lived UI opens. */
  async function initialize() {
    bindEvents();
    try {
      const stored = await chrome.storage.local.get(Object.values(STORAGE_KEYS));
      renderPreferences(normalizePreferences(stored[STORAGE_KEYS.preferences]));
      renderImage(stored[STORAGE_KEYS.image] || "", stored[STORAGE_KEYS.fileName] || "");
    } catch (error) {
      renderPreferences(DEFAULT_PREFERENCES);
      renderImage("");
      setStatus("无法读取背景设置，请重新打开弹窗", true);
    }
  }

  void initialize();
})();
