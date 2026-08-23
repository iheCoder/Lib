const form = document.querySelector("#resolve-form");
const shareInput = document.querySelector("#share-text");
const submitButton = document.querySelector("#resolve-button");
const buttonLabel = submitButton.querySelector(".button-label");
const formError = document.querySelector("#form-error");
const loadingState = document.querySelector("#result-loading");
const resultCard = document.querySelector("#result-card");
const resultAuthor = document.querySelector("#result-author");
const resultTitle = document.querySelector("#result-title");
const resultMeta = document.querySelector("#result-meta");
const resultFilename = document.querySelector("#result-filename");
const downloadLink = document.querySelector("#download-link");
const imageGallery = document.querySelector("#image-gallery");
const imageGrid = document.querySelector("#image-grid");

let activeRequest = null;

// setState is the single UI-state transition boundary. Keeping loading, error,
// and success visibility together prevents stale result cards from surviving a
// second request or an aborted fetch.
function setState(state, message = "") {
  const isLoading = state === "loading";
  loadingState.hidden = !isLoading;
  resultCard.hidden = state !== "success";
  if (state !== "success") imageGallery.hidden = true;
  formError.hidden = state !== "error";
  formError.textContent = state === "error" ? message : "";
  submitButton.disabled = isLoading;
  buttonLabel.textContent = isLoading ? "正在解析" : "解析作品";
}

// readResponse handles both expected JSON and an unexpected proxy error page,
// ensuring the user always receives a useful local message instead of a JSON
// parsing exception.
async function readResponse(response) {
  const contentType = response.headers.get("content-type") || "";
  if (!contentType.includes("application/json")) {
    throw new Error("服务返回了无法识别的响应，请稍后重试。");
  }
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.error || "作品解析失败，请检查链接后重试。");
  }
  return payload;
}

// renderResult writes all upstream strings with textContent, never HTML. This
// keeps author names and titles inert even if a public work contains markup-like
// characters.
function renderResult(work) {
  const isImagePost = work.kind === "images";
  resultAuthor.textContent = work.author || "未知作者";
  resultTitle.textContent = work.title || "未命名作品";
  resultMeta.textContent = isImagePost ? `图集，${work.assetCount} 张` : "视频，最高可用 H.264";
  resultFilename.textContent = work.filename || "";
  resultFilename.hidden = isImagePost;
  downloadLink.hidden = isImagePost;
  imageGallery.hidden = !isImagePost;
  if (isImagePost) {
    renderImageGallery(work.assets || []);
  } else {
    imageGrid.replaceChildren();
    downloadLink.href = work.downloadUrl;
    downloadLink.textContent = "下载原片";
  }
  setState("success");
  const focusTarget = isImagePost ? imageGrid.querySelector(".image-download") : downloadLink;
  focusTarget?.focus({ preventScroll: true });
}

// renderImageGallery builds same-origin previews without interpreting upstream
// metadata as HTML. Each image remains independently downloadable if another
// image's preview request expires or fails.
function renderImageGallery(assets) {
  const items = assets.map((asset) => createImageItem(asset));
  imageGrid.replaceChildren(...items);
}

// createImageItem keeps one preview, status message, and download action in a
// single accessible unit. Preview failures do not disable the download link.
function createImageItem(asset) {
  const item = document.createElement("article");
  item.className = "image-item";

  const image = document.createElement("img");
  image.src = asset.previewUrl;
  image.alt = `作品图片 ${asset.index}`;
  image.loading = "lazy";
	const frame = document.createElement("div");
	frame.className = "image-frame";
	frame.append(image);

  const footer = document.createElement("div");
  footer.className = "image-item-footer";
  const label = document.createElement("span");
  label.textContent = `第 ${asset.index} 张`;
  const action = document.createElement("a");
  action.className = "image-download";
  action.href = asset.downloadUrl;
  action.textContent = "下载这张";
  footer.append(label, action);

  const status = document.createElement("p");
  status.className = "image-status";
  status.hidden = true;
  status.textContent = "预览加载失败，仍可尝试下载。";
  image.addEventListener("error", () => {
    item.classList.add("is-error");
    status.hidden = false;
  }, { once: true });
	item.append(frame, footer, status);
  return item;
}

// resolveVideo cancels any previous in-flight request before starting another.
// The browser only transitions to success after metadata and a local download
// ticket have both been created by the server.
async function resolveWork(shareText) {
  if (activeRequest) activeRequest.abort();
  activeRequest = new AbortController();
  setState("loading");
  try {
    const response = await fetch("/api/resolve", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ shareText }),
      signal: activeRequest.signal,
    });
    renderResult(await readResponse(response));
  } catch (error) {
    if (error.name !== "AbortError") {
      setState("error", error.message || "作品解析失败，请稍后重试。");
    }
  } finally {
    activeRequest = null;
  }
}

form.addEventListener("submit", (event) => {
  event.preventDefault();
  const shareText = shareInput.value.trim();
  if (!shareText) {
    setState("error", "请先粘贴抖音分享文本或链接。");
    shareInput.focus();
    return;
  }
  resolveWork(shareText);
});

shareInput.addEventListener("input", () => {
  if (!formError.hidden) setState("idle");
});
