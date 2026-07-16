const elements = {
  loading: document.querySelector("#loading"),
  empty: document.querySelector("#empty"),
  sessions: document.querySelector("#sessions"),
  message: document.querySelector("#message"),
  refresh: document.querySelector("#refresh"),
};

// loadSessions owns the page's read-state transition. Every refresh clears
// stale session IDs before requesting a newly reconstructed IINA catalog.
async function loadSessions() {
  setLoadingState();
  try {
    const response = await fetch("/api/playback", {headers: {Accept: "application/json"}});
    const payload = await readPayload(response);
    renderSessions(payload.sessions || []);
  } catch (error) {
    showError(error.message);
  }
}

// resumeSession sends only the opaque ID rendered by the backend. Paths remain
// server-owned and are revalidated immediately before iina-cli is invoked.
async function resumeSession(session, button) {
  button.disabled = true;
  elements.message.textContent = `正在恢复 ${session.playbacks.length} 个视频…`;
  try {
    const response = await fetch("/api/resume", {
      method: "POST",
      headers: {Accept: "application/json", "Content-Type": "application/json"},
      body: JSON.stringify({sessionId: session.id}),
    });
    const payload = await readPayload(response);
    const opened = payload.session.availableCount;
    elements.message.textContent = `已交给 IINA，在 ${opened} 个独立窗口中恢复。`;
  } catch (error) {
    showError(error.message);
  } finally {
    button.disabled = session.availableCount === 0;
  }
}

// renderSessions keeps recent batches separate. This matters after upgrading
// from the single-video version, because that attempt may have created a newer
// one-item batch while the desired multi-window batch remains immediately below.
function renderSessions(sessions) {
  elements.loading.classList.add("hidden");
  if (sessions.length === 0) {
    elements.empty.classList.remove("hidden");
    return;
  }
  sessions.forEach((session, index) => elements.sessions.append(createSessionCard(session, index)));
  elements.sessions.classList.remove("hidden");
}

// createSessionCard builds nodes with textContent rather than HTML strings;
// filenames and paths originate on disk and must never become markup.
function createSessionCard(session, index) {
  const card = document.createElement("section");
  card.className = "session-card";

  const header = document.createElement("div");
  header.className = "session-header";
  const copy = document.createElement("div");
  copy.append(createText("strong", sessionLabel(index, session.playbacks.length)));
  copy.append(createText("span", formatClosedAt(session.closedAt)));
  header.append(copy, createText("span", `${session.playbacks.length} 个`, "count"));

  const list = document.createElement("div");
  list.className = "playback-list";
  session.playbacks.forEach((playback) => list.append(createPlaybackRow(playback)));

  const button = createText("button", `恢复这 ${session.availableCount} 个视频`, "primary");
  button.type = "button";
  button.disabled = session.availableCount === 0;
  button.addEventListener("click", () => resumeSession(session, button));
  card.append(header, list, button);
  return card;
}

// createPlaybackRow shows unavailable files instead of dropping them, helping
// the user identify an external disk that needs reconnecting.
function createPlaybackRow(playback) {
  const row = document.createElement("div");
  row.className = `playback-row${playback.available ? "" : " unavailable"}`;
  const icon = createText("span", playback.available ? "▶" : "!", "file-icon");
  const copy = document.createElement("div");
  copy.className = "file-copy";
  copy.append(createText("strong", playback.name || "未命名视频"));
  copy.append(createText("span", playback.path));
  copy.append(createText("span", `看到 ${formatDuration(playback.positionSeconds)}`, "position"));
  row.append(icon, copy);
  return row;
}

// createText is the single DOM text factory, keeping class assignment and safe
// text insertion consistent across dynamically rendered cards.
function createText(tag, text, className = "") {
  const element = document.createElement(tag);
  element.textContent = text;
  if (className) element.className = className;
  return element;
}

async function readPayload(response) {
  // API failures use the same JSON envelope as successes, so event handlers
  // only deal with domain-level messages.
  const payload = await response.json();
  if (!response.ok) throw new Error(payload.error || "本地服务暂时不可用");
  return payload;
}

function setLoadingState() {
  // Reset mutually exclusive panels and discard stale buttons before a request.
  elements.loading.classList.remove("hidden");
  elements.empty.classList.add("hidden");
  elements.sessions.classList.add("hidden");
  elements.sessions.replaceChildren();
  elements.message.textContent = "";
}

function showError(message) {
  // The live region announces failures without destroying visible session data.
  elements.loading.classList.add("hidden");
  elements.message.textContent = message;
}

function sessionLabel(index, count) {
  if (index === 0) return count > 1 ? "最近的多窗口会话" : "最近的会话";
  return count > 1 ? "更早的多窗口会话" : "更早的会话";
}

function formatClosedAt(value) {
  if (!value) return "来自 IINA 最近播放记录";
  return `关闭于 ${new Intl.DateTimeFormat("zh-CN", {dateStyle: "medium", timeStyle: "short"}).format(new Date(value))}`;
}

function formatDuration(totalSeconds) {
  // IINA stores fractional seconds; sub-second precision adds no value here.
  const seconds = Math.max(0, Math.floor(totalSeconds || 0));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`
    : `${minutes}:${String(remainder).padStart(2, "0")}`;
}

elements.refresh.addEventListener("click", loadSessions);
loadSessions();
