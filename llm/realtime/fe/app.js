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
  if (!sResp.ok) throw new Error(`/session failed: ${sResp.status}`);
  const session = await sResp.json();
  const ephemeralKey = session?.client_secret?.value;
  const modelFromSession = session?.model || session?.default_model;
  const model = modelFromSession || DEFAULT_REALTIME_MODEL;
  if (!ephemeralKey) {
    throw new Error("no ephemeral key in /session response");
  }

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
  dc.onopen = () => log("DataChannel 已打开");
  dc.onclose = () => log("DataChannel 已关闭");
  dc.onerror = (e) => log(`DataChannel 错误: ${e?.message || e}`, "error");
  dc.onmessage = (ev) => {
    // Raw events; in Step 6 we will parse and render
    log(`DC <= ${ev.data}`);
  };

  // Attach remote audio
  const remoteStream = (window.__remoteStream = new MediaStream());
  pc.ontrack = (ev) => {
    ev.streams[0].getTracks().forEach((t) => remoteStream.addTrack(t));
    const audio = document.querySelector("#remote-audio");
    if (audio) audio.srcObject = remoteStream;
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
    throw new Error(`Realtime SDP failed: ${r.status} ${txt}`);
  }
  const answer = await r.text();
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
