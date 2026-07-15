const elements = {
  loading: document.querySelector("#loading"),
  empty: document.querySelector("#empty"),
  playback: document.querySelector("#playback"),
  fileName: document.querySelector("#file-name"),
  filePath: document.querySelector("#file-path"),
  position: document.querySelector("#position"),
  message: document.querySelector("#message"),
  resume: document.querySelector("#resume"),
  refresh: document.querySelector("#refresh"),
};

// loadPlayback owns the page's read-state transition. Every refresh starts from
// a known loading state so stale file details cannot remain actionable.
async function loadPlayback() {
  setLoadingState();
  try {
    const response = await fetch("/api/playback", {headers: {Accept: "application/json"}});
    const payload = await readPayload(response);
    renderPlayback(payload.playback);
  } catch (error) {
    showError(error.message);
  }
}

// resumePlayback contains the only write action. It sends no path because the
// server deliberately re-reads IINA's latest record at click time.
async function resumePlayback() {
  elements.resume.disabled = true;
  elements.message.textContent = "正在打开 IINA…";
  try {
    const response = await fetch("/api/resume", {
      method: "POST",
      headers: {Accept: "application/json", "Content-Type": "application/json"},
      body: "{}",
    });
    await readPayload(response);
    elements.message.textContent = "已交给 IINA，祝你观影愉快。";
  } catch (error) {
    showError(error.message);
  } finally {
    elements.resume.disabled = false;
  }
}

async function readPayload(response) {
  // API failures use the same JSON envelope as successes, so parsing stays in
  // one place and event handlers only deal with domain-level messages.
  const payload = await response.json();
  if (!response.ok) throw new Error(payload.error || "本地服务暂时不可用");
  return payload;
}

function setLoadingState() {
  // Reset all mutually exclusive panels before a request to prevent stale data
  // from remaining clickable during a refresh.
  elements.loading.classList.remove("hidden");
  elements.empty.classList.add("hidden");
  elements.playback.classList.add("hidden");
  elements.resume.disabled = true;
  elements.message.textContent = "";
}

// renderPlayback treats an unavailable local file as visible-but-disabled. The
// user can then see which external disk or moved file needs attention.
function renderPlayback(playback) {
  elements.loading.classList.add("hidden");
  if (!playback) {
    elements.empty.classList.remove("hidden");
    return;
  }

  elements.fileName.textContent = playback.name || "未命名视频";
  elements.filePath.textContent = playback.path;
  elements.position.textContent = `上次看到 ${formatDuration(playback.positionSeconds)}`;
  elements.playback.classList.remove("hidden");
  elements.resume.disabled = !playback.available;
  if (!playback.available) showError("文件当前不可用，请连接对应磁盘或恢复原路径。 ");
}

function showError(message) {
  // The live region announces failures without replacing useful file context.
  elements.loading.classList.add("hidden");
  elements.message.textContent = message;
}

function formatDuration(totalSeconds) {
  // IINA stores fractional seconds. The UI floors them because sub-second
  // precision adds noise and does not affect where IINA resumes.
  const seconds = Math.max(0, Math.floor(totalSeconds || 0));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`
    : `${minutes}:${String(remainder).padStart(2, "0")}`;
}

elements.resume.addEventListener("click", resumePlayback);
elements.refresh.addEventListener("click", loadPlayback);
loadPlayback();
