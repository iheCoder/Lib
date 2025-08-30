(() => {
  const $ = (sel) => document.querySelector(sel);
  const statusIndicator = $("#status-indicator");
  const statusText = $("#status-text");
  const btnStart = $("#btn-start");
  const btnStop = $("#btn-stop");
  const logEl = $("#log");
  const vuBar = $("#vu-bar");

  const UI_STATE = {
    Idle: "未连接",
    Connecting: "连接中…",
    Connected: "已连接",
    Error: "错误",
  };

  let currentState = UI_STATE.Idle;

  function setState(next) {
    currentState = next;
    statusText.textContent = next;
    statusIndicator.classList.remove("is-connected", "is-connecting", "is-error");
    switch (next) {
      case UI_STATE.Connecting:
        statusIndicator.classList.add("is-connecting");
        btnStart.disabled = true;
        btnStop.disabled = false;
        break;
      case UI_STATE.Connected:
        statusIndicator.classList.add("is-connected");
        btnStart.disabled = true;
        btnStop.disabled = false;
        break;
      case UI_STATE.Error:
        statusIndicator.classList.add("is-error");
        btnStart.disabled = false;
        btnStop.disabled = true;
        break;
      default:
        btnStart.disabled = false;
        btnStop.disabled = true;
    }
  }

  function log(msg, level = "info") {
    const line = document.createElement("div");
    line.className = "log__line";
    const t = document.createElement("time");
    t.textContent = new Date().toLocaleTimeString();
    line.appendChild(t);
    line.append(" ", `[${level}]`, " ", msg);
    logEl.appendChild(line);
    logEl.scrollTop = logEl.scrollHeight;
  }

  // Placeholder for mic VU meter until Step 2
  let fakeVuTimer = null;
  function startFakeVu() {
    stopFakeVu();
    fakeVuTimer = setInterval(() => {
      const v = Math.floor(Math.random() * 8) + 2; // 2-10%
      vuBar.style.width = `${v}%`;
    }, 120);
  }
  function stopFakeVu() {
    if (fakeVuTimer) clearInterval(fakeVuTimer);
    fakeVuTimer = null;
    vuBar.style.width = "2%";
  }

  async function onStart() {
    try {
      log("开始：初始化 UI（Step 1，仅占位）");
      setState(UI_STATE.Connecting);
      // Step 1: 不做实际连接，仅模拟 UI 状态变化
      await new Promise((r) => setTimeout(r, 500));
      setState(UI_STATE.Connected);
      log("已进入已连接状态（占位）。下一步将接入麦克风与 VU meter。");
      startFakeVu();
    } catch (err) {
      console.error(err);
      log(String(err), "error");
      setState(UI_STATE.Error);
    }
  }

  async function onStop() {
    log("停止：清理状态（Step 1，占位）");
    setState(UI_STATE.Idle);
    stopFakeVu();
  }

  btnStart.addEventListener("click", onStart);
  btnStop.addEventListener("click", onStop);

  // 初始
  setState(UI_STATE.Idle);
  log("页面就绪。点击“开始通话”体验 Step 1 UI。");
})();

