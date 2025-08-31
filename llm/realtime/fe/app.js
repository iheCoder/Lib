// =============================================
// 前端应用主脚本（含中文注释）
// - 采集麦克风，显示实时音量（AnalyserNode）
// - 调用后端 /session 获取临时密钥
// - 通过 WebRTC 与 OpenAI Realtime 完成 SDP 交换
// - 使用 DataChannel 发送/接收事件
// - 将模型的文本输出渲染为“对话气泡”（Step 6）
// =============================================
(() => {
  const $ = (sel) => document.querySelector(sel);
  const statusIndicator = $("#status-indicator");
  const statusText = $("#status-text");
  const modePill = $("#mode-pill");
  const btnStart = $("#btn-start");
  const btnStop = $("#btn-stop");
  const btnCancel = $("#btn-cancel");
  const btnPTT = $("#btn-ptt");
  const btnContinuous = $("#btn-continuous");
  const btnReconnect = $("#btn-reconnect");
  const logEl = $("#log");
  const vuBar = $("#vu-bar");
  const remoteAudio = $("#remote-audio");
  const chatEl = document.querySelector('#chat');

  const UI_STATE = {
    Idle: "未连接",
    Connecting: "连接中…",
    Connected: "已连接",
    Error: "错误",
  };

  let currentState = UI_STATE.Idle;
  const MODE = { Idle: '空闲', Listening: '聆听中', Generating: '生成中', Error: '错误' };
  let currentMode = MODE.Idle;
  let lastError = '';
  let disconnectTimer = null; // 用于容忍短暂的 disconnected
  let continuousMode = false; // 持续对话偏好（未连接时也可切换）

  function setState(next) {
    currentState = next;
    statusText.textContent = next;
    statusIndicator.classList.remove("is-connected", "is-connecting", "is-error");

    // 根据连接状态自动设置对应的模式，避免状态矛盾
    switch (next) {
      case UI_STATE.Connecting:
        statusIndicator.classList.add("is-connecting");
        setMode(MODE.Idle); // 连接中时模式为空闲
        btnStart.disabled = true;
        btnStop.disabled = false;
        break;
      case UI_STATE.Connected:
        statusIndicator.classList.add("is-connected");
        // 连接成功后，如果当前不是生成模式，则设置为聆听模式
        if (currentMode !== MODE.Generating) {
          setMode(MODE.Listening);
        }
        btnStart.disabled = true;
        btnStop.disabled = false;
        btnCancel.disabled = false;
        btnPTT.disabled = false;
        btnContinuous.disabled = false;
        btnReconnect.disabled = true;
        break;
      case UI_STATE.Error:
        statusIndicator.classList.add("is-error");
        setMode(MODE.Error); // 错误状态时模式也是错误
        btnStart.disabled = false;
        btnStop.disabled = true;
        btnCancel.disabled = true;
        btnPTT.disabled = true;
        // 错误状态仍允许切换"持续对话"偏好
        btnContinuous.disabled = false;
        btnReconnect.disabled = false;
        break;
      default: // Idle
        setMode(MODE.Idle); // 未连接时模式为空闲
        btnStart.disabled = false;
        btnStop.disabled = true;
        btnCancel.disabled = true;
        btnPTT.disabled = true;
        // 未连接也允许设置偏好
        btnContinuous.disabled = false;
        btnReconnect.disabled = true;
    }
  }

  function setMode(next) {
    currentMode = next;
    if (!modePill) return;
    modePill.textContent = next;
    modePill.classList.remove('is-listening','is-generating','is-error');
    if (next === MODE.Listening) modePill.classList.add('is-listening');
    else if (next === MODE.Generating) modePill.classList.add('is-generating');
    else if (next === MODE.Error) modePill.classList.add('is-error');
  }

  function setError(msg) {
    lastError = String(msg || '');
    log(`状态错误：${lastError}`, 'error');
    setState(UI_STATE.Error); // setState 会自动调用 setMode(MODE.Error)
    if (modePill && lastError) modePill.title = lastError;
  }

  function clearDisconnectGuard() {
    if (disconnectTimer) { clearTimeout(disconnectTimer); disconnectTimer = null; }
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

  // —— Step 6: 对话渲染 ——
  // 将模型的文本增量（delta）实时更新到“对话气泡”中
  const chatIndex = new Map(); // responseId -> { el, text }
  function chatStart(role, id) {
    if (!chatEl) return;
    const wrap = document.createElement('div');
    wrap.className = `msg msg--${role}`;
    const bubble = document.createElement('div');
    bubble.className = 'msg__bubble';
    bubble.textContent = '';
    wrap.appendChild(bubble);
    chatEl.appendChild(wrap);
    chatEl.scrollTop = chatEl.scrollHeight;
    chatIndex.set(id || `anon-${Date.now()}`, { el: bubble, text: '' });
  }
  function chatDelta(id, deltaText) {
    const entry = chatIndex.get(id);
    if (!entry) return;
    entry.text += deltaText || '';
    entry.el.textContent = entry.text;
    if (chatEl) chatEl.scrollTop = chatEl.scrollHeight;
  }
  function chatDone(id) {
    // 预留：此处可添加结束标记或动画
  }

  // 轻量事件处理（放在 IIFE 内部，便于访问 chat* 函数）
  let __currentResponseId = null;
  let __currentText = "";
  function handleRealtimeEvent(msg, log) {
    const type = msg?.type;
    switch (type) {
      case "response.created": {
        __currentResponseId = msg?.response?.id || null;
        __currentText = "";
        log(`response.created: ${__currentResponseId || "(no id)"}`);
        // 开始一个助手气泡（空内容，后续用 delta 填充）
        chatStart("assistant", __currentResponseId || `resp-${Date.now()}`);
        setMode(MODE.Generating);
        break;
      }
      case "output_audio_buffer.started": {
        setMode(MODE.Generating);
        break;
      }
      case "output_audio_buffer.stopped": {
        setMode(MODE.Listening);
        break;
      }
      case "response.output_text.delta": {
        const d = msg?.delta || msg?.text || "";
        __currentText += d;
        log(`text.delta += ${d}`);
        if (!__currentResponseId) {
          __currentResponseId = `resp-${Date.now()}`;
          chatStart("assistant", __currentResponseId);
        }
        chatDelta(__currentResponseId, d);
        break;
      }
      case "response.audio_transcript.delta": {
        const d = msg?.delta || msg?.text || msg?.transcript || "";
        __currentText += d;
        log(`audio_transcript.delta += ${d}`);
        if (!__currentResponseId) {
          __currentResponseId = `resp-${Date.now()}`;
          chatStart("assistant", __currentResponseId);
        }
        chatDelta(__currentResponseId, d);
        break;
      }
      case "response.output_text.done": {
        log(`text.done: ${__currentText}`);
        if (__currentResponseId) chatDone(__currentResponseId);
        break;
      }
      case "response.audio_transcript.done": {
        log(`audio_transcript.done: ${__currentText}`);
        if (__currentResponseId) chatDone(__currentResponseId);
        break;
      }
      case "response.completed": {
        log(`response.completed: ${msg?.response?.id || "(no id)"}`);
        if (__currentText) log(`final text: ${__currentText}`);
        if (__currentResponseId) chatDone(__currentResponseId);
        __currentResponseId = null;
        __currentText = "";
        setMode(MODE.Listening);
        break;
      }
      case "response.done": {
        log(`response.done: ${msg?.response?.id || "(no id)"}`);
        if (__currentText) log(`final text: ${__currentText}`);
        if (__currentResponseId) chatDone(__currentResponseId);
        __currentResponseId = null;
        __currentText = "";
        setMode(MODE.Listening);
        break;
      }
      case "error": {
        const e = msg?.error || msg;
        const emsg = e?.message || JSON.stringify(e);
        // 非连接类错误（如参数格式等）不切换到全局错误，仅记录
        log(`Realtime error: ${emsg}`, "error");
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
  // 暴露给外部调用（dc.onmessage 使用）
  window.__handleRealtimeEvent = handleRealtimeEvent;
  // 暴露 chat 函数给外部调用
  window.__chatStart = chatStart;
  window.__chatDelta = chatDelta;
  window.__chatDone = chatDone;

  // Step 2: 真实麦克风采集 + 音量表（AnalyserNode）
  let audioContext = null;
  let analyser = null;
  let micStream = null;
  let sourceNode = null;
  let rafId = null;

  // Step 4: WebRTC 与 OpenAI Realtime
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
      clearDisconnectGuard();
      const ok = await startMic();
      if (!ok) {
        setState(UI_STATE.Error); // setState 会自动调用 setMode
        return;
      }
      // PTT 初始：默认不上行，按住按钮再开启
      setMicSending(false);
      await connectRealtime();
      setState(UI_STATE.Connected); // setState 会自动调用 setMode
      log("WebRTC 已建立，远端音频通道就绪。");
    } catch (err) {
      console.error(err);
      log(String(err), "error");
      setState(UI_STATE.Error); // setState 会自动调用 setMode
    }
  }

  async function onStop() {
    log("停止：关闭连接与麦克风（Step 4）");
    setMicSending(false);
    teardownRTC();
    stopMic();
    clearDisconnectGuard();
    setState(UI_STATE.Idle); // setState 会自动调用 setMode
  }

  btnStart.addEventListener("click", onStart);
  btnStop.addEventListener("click", onStop);
  btnReconnect.addEventListener("click", async () => {
    log('尝试重连…');
    try {
      teardownRTC();
      await connectRealtime();
      setState(UI_STATE.Connected); // setState 会自动调用 setMode
      log('重连成功。');
    } catch (e) {
      log(`重连失败: ${e?.message || e}`, 'error');
      setState(UI_STATE.Error); // setState 会自动调用 setMode
    }
  });
  btnCancel.addEventListener("click", () => {
    const dc = window.__dc;
    if (!dc || dc.readyState !== "open") {
      log("DataChannel 未就绪，无法取消。", "warn");
      return;
    }
    log("发送取消事件（response.cancel）…");
    safeSend(dc, { type: "response.cancel" });
  });

  // 持续对话模式：打开后无需按住说话，麦克风持续上行，并请求服务端使用 VAD（turn_detection）
  function setContinuous(on) {
    continuousMode = !!on;
    if (btnContinuous) btnContinuous.textContent = `持续对话：${continuousMode ? '开' : '关'}`;
    if (btnContinuous) btnContinuous.classList.toggle('is-active', continuousMode);
    const dc = window.__dc;
    if (continuousMode) {
      if (currentState === UI_STATE.Connected) setMicSending(true);
      // 请求服务端开启 server VAD（若支持）
      if (dc && dc.readyState === 'open') {
        safeSend(dc, { type: 'session.update', session: { turn_detection: { type: 'server_vad', threshold: 0.5, silence_duration_ms: 700 } } });
      }
      // PTT 在持续模式下可选使用，但默认不需要
    } else {
      // 关闭持续上行 & 关闭 server VAD
      if (currentState === UI_STATE.Connected) setMicSending(false);
      if (dc && dc.readyState === 'open') {
        safeSend(dc, { type: 'session.update', session: { turn_detection: { type: 'none' } } });
      }
    }
  }
  btnContinuous.addEventListener('click', () => setContinuous(!continuousMode));

  function applySessionMode() {
    if (continuousMode) {
      setMicSending(true);
      const dc = window.__dc;
      if (dc && dc.readyState === 'open') {
        safeSend(dc, { type: 'session.update', session: { turn_detection: { type: 'server_vad', threshold: 0.5, silence_duration_ms: 700 } } });
        log('持续对话模式已启用：麦克风持续上行 + 服务端 VAD');
      }
    }
  }

  // —— Push-to-Talk (按住说话) + Barge-in ——
  let pttActive = false;
  function setMicSending(active) {
    const stream = window.__micStream;
    if (!stream) {
      log("警告：麦克风流不存在，无法设置音频上行", "warn");
      return;
    }

    const audioTracks = stream.getAudioTracks();
    if (audioTracks.length === 0) {
      log("警告：没有找到音频轨道", "warn");
      return;
    }

    audioTracks.forEach(t => {
      t.enabled = !!active;
      log(`音频轨道 ${t.id} enabled: ${t.enabled}`);
    });

    pttActive = !!active;
    log(`PTT: ${pttActive ? '开始说话（上行开启）' : '停止说话（上行关闭）'}`);
    if (btnPTT) btnPTT.classList.toggle('is-active', pttActive);

    // 在持续对话模式下，不要因为麦克风状态改变模式
    if (!continuousMode) {
      setMode(pttActive ? MODE.Listening : MODE.Idle);
    }
  }

  function pttDown() {
    const dc = window.__dc;
    if (dc && dc.readyState === 'open') {
      // barge-in：开始说话前先取消当前生成
      safeSend(dc, { type: 'response.cancel' });
    }
    setMicSending(true);
  }
  function pttUp() { setMicSending(false); }

  // 鼠标/触摸
  // 优先使用 Pointer 事件，兼容性不足则回退到 mouse/touch
  if (window.PointerEvent) {
    btnPTT.addEventListener('pointerdown', (e) => { e.preventDefault(); pttDown(); });
    btnPTT.addEventListener('pointerup', (e) => { e.preventDefault(); pttUp(); });
    btnPTT.addEventListener('pointerleave', () => { if (pttActive) pttUp(); });
  } else {
    btnPTT.addEventListener('mousedown', pttDown);
    btnPTT.addEventListener('mouseup', pttUp);
    btnPTT.addEventListener('mouseleave', () => { if (pttActive) pttUp(); });
    btnPTT.addEventListener('touchstart', (e) => { e.preventDefault(); pttDown(); }, { passive: false });
    btnPTT.addEventListener('touchend', (e) => { e.preventDefault(); pttUp(); }, { passive: false });
  }

  // 键盘：空格按住说话
  let spaceHeld = false;
  window.addEventListener('keydown', (e) => {
    if (e.code === 'Space' && !spaceHeld && !btnPTT.disabled) {
      spaceHeld = true;
      e.preventDefault();
      pttDown();
    }
  });
  window.addEventListener('keyup', (e) => {
    if (e.code === 'Space' && spaceHeld) {
      spaceHeld = false;
      e.preventDefault();
      pttUp();
    }
  });

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
    // 根据用户偏好应用会话模式（如持续对话）
    try { applySessionMode(); } catch {}
    // 强制把状态复位为已连接，避免旧错误残留
    try { setState(UI_STATE.Connected); } catch {} // setState 会自动调用 setMode
  };
  dc.onclose = () => log("DataChannel 已关闭");
  dc.onerror = (e) => log(`DataChannel 错误: ${e?.message || e}`, "error");
  dc.onmessage = (ev) => {
    const raw = String(ev.data || "");
    log(`DC <= ${raw.length > 180 ? raw.slice(0, 180) + "…" : raw}`);
    try {
      const msg = JSON.parse(raw);
      if (window.__handleRealtimeEvent) window.__handleRealtimeEvent(msg, log);
    } catch {}
  };

  // Attach remote audio（收到远端音轨，绑定播放器）
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

  pc.oniceconnectionstatechange = () => {
    const ice = pc.iceConnectionState;
    log(`ICE: ${ice}`);
    if (ice === 'connected' || ice === 'completed') {
      clearDisconnectGuard();
      setState(UI_STATE.Connected); // setState 会自动调用 setMode
    }
  };
  pc.onconnectionstatechange = () => {
    const st = pc.connectionState;
    log(`PC: ${st}`);
    if (st === 'connected') {
      clearDisconnectGuard();
      setState(UI_STATE.Connected); // setState 会自动调用 setMode
    } else if (st === 'failed') {
      clearDisconnectGuard();
      setError('PeerConnection 失败');
    } else if (st === 'disconnected') {
      // 仅记录，不立刻报错；等待 oniceconnectionstatechange 恢复
      log('网络短暂中断，等待恢复…');
    } else if (st === 'closed') {
      clearDisconnectGuard();
      setState(UI_STATE.Idle); // setState 会自动调用 setMode
    }
  };

  // 3) 生成 offer 并等待 ICE 收集完成（非 trickle 模式，便于一次性发送完整 SDP）
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await waitForIceGatheringComplete(pc, log);

  const localSDP = pc.localDescription?.sdp;
  if (!localSDP) throw new Error("no local SDP");

  // 4) 将 application/sdp 发送给 Realtime，换取 answer（也是 application/sdp 文本）
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
  // 再次确保 UI 显示连接成功
  try { setState(UI_STATE.Connected); } catch {} // setState 会自动调用 setMode

  // 显示一个系统气泡，确保"对话"面板可见
  const helloId = `hello-${Date.now()}`;
  const chatStartFn = window.__chatStart || chatStart;
  const chatDeltaFn = window.__chatDelta || chatDelta;
  if (chatStartFn && chatDeltaFn) {
    chatStartFn('assistant', helloId);
    chatDeltaFn(helloId, '（已连接，等待对话…）');
  }

  // save refs for teardown
  window.__pc = pc;
  window.__dc = dc;
  window.__remoteStream = remoteStream;
}

function teardownRTC() {
  const log = window.__log || (() => {});
  const pc = window.__pc;
  const dc = window.__dc;
  clearDisconnectGuard();
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
// 之前的 handleRealtimeEvent 已移入 IIFE，避免作用域问题
