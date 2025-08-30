(() => {
  const $ = (sel) => document.querySelector(sel);
  const statusIndicator = $("#status-indicator");
  const statusText = $("#status-text");
  const btnStart = $("#btn-start");
  const btnStop = $("#btn-stop");
  const logEl = $("#log");
  const vuBar = $("#vu-bar");
  const remoteAudio = $("#remote-audio");

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

  // Step 4: WebRTC with OpenAI Realtime
  let pc = null;
  let dc = null;
  let remoteStream = null;

  const DEFAULT_REALTIME_MODEL = "gpt-4o-realtime-preview-2024-12-17";

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
      // Expose for RTC steps
      window.__micStream = micStream;
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
      log("开始：请求麦克风并准备连接 Realtime（Step 4）");
      setState(UI_STATE.Connecting);
      const ok = await startMic();
      if (!ok) {
        setState(UI_STATE.Error);
        return;
      }
      await connectRealtime();
      setState(UI_STATE.Connected);
      log("WebRTC 已建立，远端音频通道就绪。");
    } catch (err) {
      console.error(err);
      log(String(err), "error");
      setState(UI_STATE.Error);
    }
  }

  async function onStop() {
    log("停止：关闭连接与麦克风（Step 4）");
    teardownRTC();
    stopMic();
    setState(UI_STATE.Idle);
  }

  btnStart.addEventListener("click", onStart);
  btnStop.addEventListener("click", onStop);

  // 初始
  setState(UI_STATE.Idle);
  log("页面就绪。点击“开始通话”授予麦克风权限并查看实时音量。");
})();

// ----- Realtime helpers -----
  async function connectRealtime() {
  const log = (window.__log ||= (msg, level) => {
    const el = document.querySelector("#log");
    const line = document.createElement("div");
    line.className = "log__line";
    const t = document.createElement("time");
    t.textContent = new Date().toLocaleTimeString();
    line.appendChild(t);
    line.append(" ", `[${level || "info"}]`, " ", msg);
    el.appendChild(line);
    el.scrollTop = el.scrollHeight;
  });

  // 1) 获取临时密钥
  log("请求 /session 以获取临时密钥…");
  const sResp = await fetch("/session");
  const sText = await sResp.text();
  if (!sResp.ok) {
    log(`/session 响应错误: ${sResp.status}` , "error");
    log(sText || "(空响应体)", "error");
    throw new Error(`/session failed: ${sResp.status}`);
  }
  let session = null;
  try {
    session = JSON.parse(sText);
  } catch (e) {
    log(`解析 /session JSON 失败: ${e?.message || e}`, "error");
    throw e;
  }
  const ephemeralKey = session?.client_secret?.value;
  const modelFromSession = session?.model || session?.default_model;
  const model = modelFromSession || DEFAULT_REALTIME_MODEL;
  if (!ephemeralKey) {
    throw new Error("no ephemeral key in /session response");
  }
  const voice = session?.voice || (session?.default_session && session.default_session.voice);
  log(`会话就绪: model=${model} voice=${voice || "(unspecified)"}`);

  // 2) 创建 RTCPeerConnection
  log("创建 RTCPeerConnection…");
  const pc = (window.__pc = new RTCPeerConnection({
    // OpenAI Realtime works without custom ICE servers for most clients
    // iceServers: [{ urls: ["stun:stun.l.google.com:19302"] }]
  }));

  const micStream = window.__micStream || null;
  if (!micStream) throw new Error("mic not ready");
  micStream.getAudioTracks().forEach((t) => pc.addTrack(t, micStream));

  // We want to receive a remote audio track
  pc.addTransceiver("audio", { direction: "recvonly" });

  // Data channel for events (we'll use in later steps)
  const dc = (window.__dc = pc.createDataChannel("oai-events"));
  dc.onopen = () => { 
    log("DataChannel 已打开");
    try { autoGreet(dc, { conversation: "auto" }); } catch (e) { log(`自动问候发送失败: ${e}`, "error"); }
  };
  dc.onclose = () => log("DataChannel 已关闭");
  dc.onerror = (e) => log(`DataChannel 错误: ${e?.message || e}`, "error");
  dc.onmessage = (ev) => {
    const raw = String(ev.data || "");
    log(`DC <= ${raw.length > 180 ? raw.slice(0, 180) + "…" : raw}`);
    try {
      const msg = JSON.parse(raw);
      handleRealtimeEvent(msg, log);
    } catch {}
  };

  // Attach remote audio
  const remoteStream = (window.__remoteStream = new MediaStream());
  pc.ontrack = (ev) => {
    ev.streams[0].getTracks().forEach((t) => remoteStream.addTrack(t));
    const audio = document.querySelector("#remote-audio");
    if (audio) {
      audio.srcObject = remoteStream;
      // Best-effort autoplay after user gesture
      audio.play && audio.play().catch((e) => log(`audio.play 被阻止: ${e?.message || e}`, "warn"));
    }
    log("收到远端音轨并绑定到播放器");
  };

  pc.oniceconnectionstatechange = () => log(`ICE: ${pc.iceConnectionState}`);
  pc.onconnectionstatechange = () => log(`PC: ${pc.connectionState}`);

  // 3) Create offer and gather ICE
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await waitForIceGatheringComplete(pc, log);

  const localSDP = pc.localDescription?.sdp;
  if (!localSDP) throw new Error("no local SDP");

  // 4) POST SDP to OpenAI Realtime and get answer
  const url = `https://api.openai.com/v1/realtime?model=${encodeURIComponent(model)}`;
  log("向 Realtime 发送 SDP…");
  const r = await fetch(url, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${ephemeralKey}`,
      "Content-Type": "application/sdp",
      Accept: "application/sdp",
      "OpenAI-Beta": "realtime=v1",
    },
    body: localSDP,
  });
  if (!r.ok) {
    const txt = await r.text().catch(() => "");
    log(`发送 SDP 失败: ${r.status}`, "error");
    log(txt || "(空响应体)", "error");
    throw new Error(`Realtime SDP failed: ${r.status}`);
  }
  const answer = await r.text();
  log(`收到 SDP answer（长度=${answer.length}）`);
  await pc.setRemoteDescription({ type: "answer", sdp: answer });
  log("SDP 交换完成，连接建立。");

  // save refs for teardown
  window.__pc = pc;
  window.__dc = dc;
  window.__remoteStream = remoteStream;
}

function teardownRTC() {
  const log = window.__log || (() => {});
  const pc = window.__pc;
  const dc = window.__dc;
  if (dc) {
    try { dc.close(); } catch {}
    window.__dc = null;
  }
  if (pc) {
    try { pc.getSenders().forEach(s => { try { s.track && s.track.stop && s.track.stop(); } catch {} }); } catch {}
    try { pc.close(); } catch {}
    window.__pc = null;
    log("RTCPeerConnection 已关闭");
  }
  const rs = window.__remoteStream;
  if (rs) {
    rs.getTracks().forEach(t => t.stop());
    window.__remoteStream = null;
  }
  const audio = document.querySelector("#remote-audio");
  if (audio) audio.srcObject = null;
}

function waitForIceGatheringComplete(pc, log) {
  return new Promise((resolve) => {
    if (pc.iceGatheringState === "complete") {
      return resolve();
    }
    const timeout = setTimeout(() => {
      log("ICE 收集超时，继续（可能使用 trickle）", "warn");
      resolve();
    }, 3000);
    pc.addEventListener("icegatheringstatechange", function onChange() {
      if (pc.iceGatheringState === "complete") {
        clearTimeout(timeout);
        pc.removeEventListener("icegatheringstatechange", onChange);
        resolve();
      }
    });
  });
}

function safeSend(dc, obj, log = window.__log || (() => {})) {
  try {
    const s = JSON.stringify(obj);
    dc.send(s);
    log(`DC => ${obj.type || "<no type>"}`);
  } catch (e) {
    log(`DC 发送失败: ${e?.message || e}`, "error");
  }
}

function autoGreet(dc, opts = {}) {
  const log = window.__log || (() => {});
  log("发送自动问候事件（response.create）…");
  const conversation = opts.conversation || "auto"; // 'auto' | 'none'
  const evt = {
    type: "response.create",
    response: {
      modalities: ["text", "audio"],
      conversation,
      instructions:
        "你是一个中文语音助手。请用友好简洁的中文打个招呼，并告诉我可以直接开口说话和你对话。",
      // 可选覆盖 voice（通常已在 /session 设置）
      // audio: { voice: "verse" }
    },
  };
  safeSend(dc, evt, log);
}

// Very lightweight event handler for Realtime messages
let __currentResponseId = null;
let __currentText = "";
function handleRealtimeEvent(msg, log) {
  const type = msg?.type;
  switch (type) {
    case "response.created": {
      __currentResponseId = msg?.response?.id || null;
      __currentText = "";
      log(`response.created: ${__currentResponseId || "(no id)"}`);
      break;
    }
    case "response.output_text.delta": {
      const d = msg?.delta || msg?.text || "";
      __currentText += d;
      log(`text.delta += ${d}`);
      break;
    }
    case "response.output_text.done": {
      log(`text.done: ${__currentText}`);
      break;
    }
    case "response.completed": {
      log(`response.completed: ${msg?.response?.id || "(no id)"}`);
      if (__currentText) log(`final text: ${__currentText}`);
      __currentResponseId = null;
      __currentText = "";
      break;
    }
    case "error": {
      const e = msg?.error || msg;
      const emsg = e?.message || JSON.stringify(e);
      log(`Realtime error: ${emsg}`, "error");
      // Auto-retry if 'default' conversation is not accepted (older samples used 'default')
      if (/Supported values are: 'auto' and 'none'/.test(emsg) && window.__dc) {
        log("检测到不支持的 conversation 值，改用 'auto' 重试问候…");
        try { autoGreet(window.__dc, { conversation: "auto" }); } catch {}
      }
      break;
    }
    default: {
      if (type) log(`event: ${type}`);
    }
  }
}
