const form = document.querySelector("#resolve-form");
const shareInput = document.querySelector("#share-text");
const submitButton = document.querySelector("#resolve-button");
const buttonLabel = submitButton.querySelector(".button-label");
const formError = document.querySelector("#form-error");
const loadingState = document.querySelector("#result-loading");
const resultCard = document.querySelector("#result-card");
const resultAuthor = document.querySelector("#result-author");
const resultTitle = document.querySelector("#result-title");
const resultFilename = document.querySelector("#result-filename");
const downloadLink = document.querySelector("#download-link");

let activeRequest = null;

// setState is the single UI-state transition boundary. Keeping loading, error,
// and success visibility together prevents stale result cards from surviving a
// second request or an aborted fetch.
function setState(state, message = "") {
  const isLoading = state === "loading";
  loadingState.hidden = !isLoading;
  resultCard.hidden = state !== "success";
  formError.hidden = state !== "error";
  formError.textContent = state === "error" ? message : "";
  submitButton.disabled = isLoading;
  buttonLabel.textContent = isLoading ? "正在解析" : "解析视频";
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
    throw new Error(payload.error || "视频解析失败，请检查链接后重试。");
  }
  return payload;
}

// renderResult writes all upstream strings with textContent, never HTML. This
// keeps author names and titles inert even if a public work contains markup-like
// characters.
function renderResult(video) {
  resultAuthor.textContent = video.author || "未知作者";
  resultTitle.textContent = video.title || "未命名作品";
  resultFilename.textContent = video.filename;
  downloadLink.href = video.downloadUrl;
  setState("success");
  downloadLink.focus({ preventScroll: true });
}

// resolveVideo cancels any previous in-flight request before starting another.
// The browser only transitions to success after metadata and a local download
// ticket have both been created by the server.
async function resolveVideo(shareText) {
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
      setState("error", error.message || "视频解析失败，请稍后重试。");
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
  resolveVideo(shareText);
});

shareInput.addEventListener("input", () => {
  if (!formError.hidden) setState("idle");
});
