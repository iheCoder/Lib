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

  // Step 2: Real microphone capture + VU meter
  let audioContext = null;
  let analyser = null;
  let micStream = null;
  let sourceNode = null;
  let rafId = null;

  function levelToPercent(rms) {
    // rms is 0..1 approx. Apply some gain for visibility
    const percent = Math.min(100, Math.max(2, Math.round(rms * 100 * 1.8)));
    return percent;
  }

  function startVuLoop() {
    if (!analyser) return;
    const buf = new Uint8Array(analyser.fftSize);

    const tick = () => {
      analyser.getByteTimeDomainData(buf);
      let sum = 0;
      for (let i = 0; i < buf.length; i++) {
        const v = (buf[i] - 128) / 128; // [-1, 1]
        sum += v * v;
      }
      const rms = Math.sqrt(sum / buf.length);
      vuBar.style.width = `${levelToPercent(rms)}%`;
      rafId = requestAnimationFrame(tick);
    };
    rafId = requestAnimationFrame(tick);
  }

  async function startMic() {
    try {
      micStream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
        video: false,
      });
      audioContext = new (window.AudioContext || window.webkitAudioContext)();
      // Some browsers require resume() after user gesture; ensured by button click
      if (audioContext.state === "suspended") await audioContext.resume();

      analyser = audioContext.createAnalyser();
      analyser.fftSize = 1024; // lower latency visual
      analyser.smoothingTimeConstant = 0.8;
      sourceNode = audioContext.createMediaStreamSource(micStream);
      sourceNode.connect(analyser);
      startVuLoop();
      log("麦克风已开启，音量表就绪。");
      return true;
    } catch (err) {
      log("无法获取麦克风权限或不在安全上下文（需 http://localhost 或 HTTPS）。", "error");
      log(String(err), "error");
      return false;
    }
  }

  function stopMic() {
    try {
      if (rafId) cancelAnimationFrame(rafId);
      rafId = null;
      vuBar.style.width = "2%";

      if (sourceNode) {
        try { sourceNode.disconnect(); } catch {}
      }
      sourceNode = null;
      analyser = null;

      if (micStream) {
        micStream.getTracks().forEach(t => t.stop());
      }
      micStream = null;

      if (audioContext) {
        const ctx = audioContext;
        audioContext = null;
        // Close may reject on Safari; ignore
        ctx.close && ctx.close().catch(() => {});
      }
    } catch (e) {
      // noop
    }
  }

  async function onStart() {
    try {
      log("开始：请求麦克风并启动音量表（Step 2）");
      setState(UI_STATE.Connecting);
      const ok = await startMic();
      if (!ok) {
        setState(UI_STATE.Error);
        return;
      }
      setState(UI_STATE.Connected);
      log("已连接（本地麦克风就绪）。下一步将接入会话与 WebRTC。");
    } catch (err) {
      console.error(err);
      log(String(err), "error");
      setState(UI_STATE.Error);
    }
  }

  async function onStop() {
    log("停止：关闭麦克风并复位（Step 2）");
    stopMic();
    setState(UI_STATE.Idle);
  }

  btnStart.addEventListener("click", onStart);
  btnStop.addEventListener("click", onStop);

  // 初始
  setState(UI_STATE.Idle);
  log("页面就绪。点击“开始通话”授予麦克风权限并查看实时音量。");
})();
