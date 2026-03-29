// ==UserScript==
// @name         m3u8视频侦测下载器【自动嗅探】
// @name:zh-CN   m3u8视频侦测下载器【自动嗅探】
// @name:zh-TW   m3u8視頻偵測下載器【自動嗅探】
// @name:en      M3U8 Video Detector and Downloader
// @version      1.5.0
// @description  自动检测页面m3u8视频并进行完整下载。检测到m3u8链接后会自动出现在页面右上角位置，点击下载即可跳转到m3u8下载器。
// @description:zh-CN  自动检测页面m3u8视频并进行完整下载。检测到m3u8链接后会自动出现在页面右上角位置，点击下载即可跳转到m3u8下载器。
// @description:zh-TW  自動檢測頁面m3u8視頻並進行完整下載。檢測到m3u8鏈接後會自動出現在頁面右上角位置，點擊下載即可跳轉到m3u8下載器。
// @description:en  Automatically detect the m3u8 video of the page and download it completely. Once detected the m3u8 link, it will appear in the upper right corner of the page. Click download to jump to the m3u8 downloader.
// @icon         https://tools.thatwind.com/favicon.png
// @author       allFull
// @namespace    https://tools.thatwind.com/
// @homepage     https://tools.thatwind.com/tool/m3u8downloader
// @match        *://*/*
// @exclude      *://www.diancigaoshou.com/*
// @require      https://cdn.jsdelivr.net/npm/m3u8-parser@4.7.1/dist/m3u8-parser.min.js
// @connect      *
// @grant        unsafeWindow
// @grant        GM_openInTab
// @grant        GM.openInTab
// @grant        GM_getValue
// @grant        GM.getValue
// @grant        GM_setValue
// @grant        GM.setValue
// @grant        GM_deleteValue
// @grant        GM.deleteValue
// @grant        GM_xmlhttpRequest
// @grant        GM.xmlHttpRequest
// @grant        GM_download
// @run-at       document-start
// @downloadURL https://update.greasyfork.org/scripts/449581/m3u8%E8%A7%86%E9%A2%91%E4%BE%A6%E6%B5%8B%E4%B8%8B%E8%BD%BD%E5%99%A8%E3%80%90%E8%87%AA%E5%8A%A8%E5%97%85%E6%8E%A2%E3%80%91.user.js
// @updateURL https://update.greasyfork.org/scripts/449581/m3u8%E8%A7%86%E9%A2%91%E4%BE%A6%E6%B5%8B%E4%B8%8B%E8%BD%BD%E5%99%A8%E3%80%90%E8%87%AA%E5%8A%A8%E5%97%85%E6%8E%A2%E3%80%91.meta.js
// ==/UserScript==

(function () {
    'use strict';

    const T_langs = {
        "en": {
            play: "Play with Pikpak",
            copy: "Copy Link",
            copied: "Copied",
            download: "Download",
            stop: "Stop",
            downloading: "Downloading",
            multiLine: "Multi",
            mins: "mins"
        },
        "zh-CN": {
            play: '使用 Pikpak 播放',
            copy: "复制链接",
            copied: "已复制",
            download: "下载",
            stop: "停止",
            downloading: "下载中",
            multiLine: "多轨",
            mins: "分钟"
        }
    };
    let l = navigator.language || "en";
    if (l.startsWith("en-")) l = "en";
    else if (l.startsWith("zh-")) l = "zh-CN";
    else l = "en";
    const T = T_langs[l] || T_langs["zh-CN"];

    if (location.host.endsWith('mail.qq.com')) {
        // 修复 @DostGit 提出的在qq邮箱无限刷新问题
        return;
    }

    // 下载任务对象：
    // 这里把旧版 download() 里揉在一起的策略判断、取消控制、进度回调、错误处理拆成一个稳定对象。
    // 这样做的目标不是“为了面向对象而面向对象”，而是为了让下载生命周期有明确边界：
    // idle -> probing/downloading -> completed/failed/cancelled。
    // 后面如果继续扩展为任务列表、失败重试、并发控制，也能继续沿着这个结构演化。
    class DownloadTask {
        constructor(api, details) {
            this.api = api;
            this.details = details || {};
            this.url = this.details.url;
            this.filename = this.details.name || 'download.mp4';
            this.state = 'idle';
            this.progress = 0;
            this.strategy = null;
            this.isCancelled = false;
            this.abortController = null;
            this.gmRequest = null;
            this.stopNotified = false;

            // 任务创建后立即启动，外部只需要持有实例并在需要时 cancel。
            this.start();
        }

        // 统一的内部状态更新入口。
        // 注意这里只更新任务模型自身，不直接碰 UI；
        // UI 仍然通过 reportProgress/onComplete/onError/onStop 这些外部回调驱动。
        setState(nextState, extra = {}) {
            this.state = nextState;
            if (typeof extra.progress === "number") {
                this.progress = extra.progress;
            }
            if (extra.strategy) {
                this.strategy = extra.strategy;
            }
        }

        // 统一上报进度。
        // 旧实现里每种下载策略各自计算百分比并直接调回调，
        // 现在收口之后，进度语义会更一致，也方便未来做节流或格式化。
        reportProgress(progress) {
            if (typeof progress !== "number" || !Number.isFinite(progress)) return;
            this.progress = progress;
            this.details.reportProgress?.(progress);
        }

        // 取消事件只允许向外广播一次。
        // 这是因为用户主动 cancel、fetch 的 AbortError、GM 的 onabort，
        // 这三条路径理论上都可能触发“停止”，如果不收口，UI 很容易重复复位。
        notifyStopOnce() {
            if (this.stopNotified) return;
            this.stopNotified = true;
            this.details.onStop?.();
        }

        // 成功完成任务。
        // 这里顺手把进度强制补到 100%，避免某些响应没有 length 或最后一次 onprogress 没打满。
        complete() {
            if (this.isCancelled) return;
            this.setState('completed', { progress: 100 });
            this.reportProgress(100);
            this.details.onComplete?.();
        }

        // 统一错误出口。
        // 一旦是用户主动取消，就不再把它继续向外表现成“失败”。
        fail(error) {
            if (this.isCancelled) return;
            this.setState('failed', { progress: 0 });
            this.details.onError?.(error);
        }

        // 对外暴露的取消动作。
        // 无论当前底层是 fetch 还是 GM 请求，调用方都不需要知道细节，只调这一个入口。
        cancel() {
            if (this.isCancelled) return;
            this.isCancelled = true;
            this.setState('cancelled', { progress: 0 });

            // 不同下载策略各自持有不同的可中断句柄，这里统一收口。
            if (this.abortController) {
                this.abortController.abort();
            }
            if (this.gmRequest && typeof this.gmRequest.abort === 'function') {
                this.gmRequest.abort();
            }

            this.notifyStopOnce();
        }

        // 任务主流程：
        // 1. 先把 URL 规范化并校验是否合法；
        // 2. 同域资源优先走浏览器原生下载；
        // 3. 跨域且浏览器支持文件系统 API 时，优先走流式写盘；
        // 4. 如果流式能力不可用或执行失败，再降级到 GM 代理下载。
        // 这里故意省掉“探测一次再下载一次”的预检请求，避免平白多一次网络成本。
        async start() {
            if (this.isCancelled) return;

            try {
                const url = this.resolveUrl(this.url);
                if (window.location.origin === url.origin) {
                    // 同域资源优先交给浏览器原生下载，避免无意义的代理与内存占用。
                    this.strategy = 'anchor';
                    this.setState('downloading', { strategy: 'anchor', progress: 100 });
                    this.triggerAnchorDownload(url.href, this.filename);
                    this.complete();
                    return;
                }

                const supportsFileSystem = typeof unsafeWindow.showSaveFilePicker === 'function';
                if (supportsFileSystem) {
                    try {
                        // 优先尝试流式写盘，减少大文件下载时 blob 常驻内存的风险。
                        this.strategy = 'stream';
                        await this.streamDownload(url.href, this.filename, this.details.headers);
                        return;
                    } catch (error) {
                        if (this.isCancelled || error?.name === 'AbortError') {
                            this.notifyStopOnce();
                            return;
                        }
                        console.error("策略2执行失败，降级到策略3:", error);
                    }
                }

                if (this.isCancelled) return;

                // 浏览器流式能力不可用或失败时，再退回到 GM 代理下载。
                this.strategy = 'gm';
                this.gmDownload();
            } catch (error) {
                this.fail(error);
            }
        }

        // 统一做 URL 解析，兼容相对路径，同时把异常都变成一致的错误文案。
        // 统一 URL 解析。
        // 好处是：任何调用方传相对路径或非法值时，错误行为都能保持一致。
        resolveUrl(url) {
            try {
                return new URL(url, location.href);
            } catch (error) {
                throw new Error(`无效的 URL: ${url}`);
            }
        }

        // 利用浏览器原生下载行为保存文件。
        // 如果传入的是 blob URL，这里会在稍后 revoke，避免对象 URL 长时间泄漏。
        // 浏览器原生下载触发器。
        // 同域资源与 GM blob 分支都会走到这里，避免保存逻辑在多个地方重复实现。
        triggerAnchorDownload(blobUrl, name) {
            const element = document.createElement('a');
            element.setAttribute('href', blobUrl);
            element.setAttribute('download', name);
            element.style.display = 'none';
            document.body.appendChild(element);
            element.click();
            document.body.removeChild(element);
            if (blobUrl.startsWith('blob:')) {
                setTimeout(() => URL.revokeObjectURL(blobUrl), 10000);
            }
        }

        // 基于 fetch + File System Access 的流式下载。
        // 这条路径的价值在于：一边从网络读，一边写入磁盘，不需要把整个视频先堆到内存里。
        async streamDownload(url, name, headers) {
            this.setState('probing', { strategy: 'stream', progress: 0 });

            let handle;
            try {
                // 先让用户确认保存位置，再发起网络请求，避免无效下载。
                handle = await unsafeWindow.showSaveFilePicker({
                    suggestedName: name,
                    types: [{
                        description: 'Video File',
                        accept: { 'video/mp4': ['.mp4'], 'application/octet-stream': ['.bin', '.ts'] }
                    }],
                });
            } catch (error) {
                if (error?.name === 'AbortError') throw error;
                throw new Error("无法打开文件保存对话框");
            }

            if (this.isCancelled) throw new DOMException('Download cancelled', 'AbortError');

            // 只有用户确认了保存位置后，才真正创建写流并开始网络请求。
            const writable = await handle.createWritable();
            this.abortController = new AbortController();
            this.setState('downloading', { strategy: 'stream', progress: 0 });

            let response;
            try {
                response = await fetch(url, {
                    headers: headers || {},
                    signal: this.abortController.signal
                });
            } catch (error) {
                await this.abortWritable(writable);
                throw error;
            }

            // 非 2xx 直接视为失败，让上层去决定是否降级到 GM 方案。
            if (!response.ok) {
                await this.abortWritable(writable);
                throw new Error(`请求失败，状态码: ${response.status}`);
            }

            // 没有 body 说明当前环境不能做流式读取，这条策略没法继续。
            if (!response.body) {
                await this.abortWritable(writable);
                throw new Error('ReadableStream not supported.');
            }

            const reader = response.body.getReader();
            const contentLength = +response.headers.get('Content-Length');
            let receivedLength = 0;

            try {
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;

                    if (this.isCancelled) {
                        throw new DOMException('Download cancelled', 'AbortError');
                    }

                    // 边读边写，避免把整个文件堆到内存里再一次性保存。
                    await writable.write(value);
                    receivedLength += value.length;

                    // 只有拿到总大小时，百分比才是可信的；
                    // 没有 Content-Length 时宁可只显示“下载中”，也不伪造进度。
                    if (contentLength > 0) {
                        const percent = Math.min(100, Number(((receivedLength / contentLength) * 100).toFixed(2)));
                        this.reportProgress(percent);
                    }
                }

                await writable.close();
                this.abortController = null;
                this.complete();
            } catch (error) {
                await this.abortWritable(writable);
                if (error?.name === 'AbortError' || this.isCancelled) {
                    throw new DOMException('Download cancelled', 'AbortError');
                }
                throw error;
            } finally {
                this.abortController = null;
            }
        }

        // 统一关闭/中止写流。
        // 某些实现支持 abort，某些只有 close，这里做一层兼容。
        async abortWritable(writable) {
            try {
                if (typeof writable.abort === "function") {
                    await writable.abort();
                } else {
                    await writable.close();
                }
            } catch { }
        }

        // GM 代理下载兜底分支。
        // 兼容性好，但缺点也很明确：最终会把整份文件放进 blob，再由浏览器保存。
        // 所以它适合作为后备方案，而不是优先方案。
        gmDownload() {
            this.setState('downloading', { strategy: 'gm', progress: 0 });
            this.gmRequest = this.api.xmlHttpRequest({
                method: "GET",
                url: this.url,
                responseType: 'blob',
                headers: this.details.headers || {},
                onload: (res) => {
                    this.gmRequest = null;
                    if (this.isCancelled) return;
                    if (res.status >= 200 && res.status < 300) {
                        // GM 分支只能拿到完整 blob，再转成临时 URL 交给浏览器保存。
                        const blob = res.response;
                        const blobUrl = URL.createObjectURL(blob);
                        this.triggerAnchorDownload(blobUrl, this.filename);
                        this.api.message("下载完成，正在保存...", 3000);
                        this.complete();
                        return;
                    }
                    this.fail(new Error(`请求失败，状态码: ${res.status}`));
                },
                onprogress: (event) => {
                    if (this.isCancelled) return;
                    if (event.lengthComputable && event.total > 0) {
                        const percent = Number(((event.loaded / event.total) * 100).toFixed(2));
                        this.reportProgress(percent);
                    }
                },
                onerror: (error) => {
                    this.gmRequest = null;
                    if (this.isCancelled) return;
                    this.fail(error);
                },
                onabort: () => {
                    this.gmRequest = null;
                    if (this.isCancelled) {
                        this.notifyStopOnce();
                    }
                }
            });
        }
    }

    const mgmapi = {

        addStyle(s) {
            let style = document.createElement("style");
            style.innerHTML = s;
            document.documentElement.appendChild(style);
        },
        async getValue(name, defaultVal) {
            return await ((typeof GM_getValue === "function") ? GM_getValue : GM.getValue)(name, defaultVal);
        },
        async setValue(name, value) {
            return await ((typeof GM_setValue === "function") ? GM_setValue : GM.setValue)(name, value);
        },
        async deleteValue(name) {
            return await ((typeof GM_deleteValue === "function") ? GM_deleteValue : GM.deleteValue)(name);
        },
        openInTab(url, open_in_background = false) {
            return ((typeof GM_openInTab === "function") ? GM_openInTab : GM.openInTab)(url, open_in_background);
        },
        xmlHttpRequest(details) {
            return ((typeof GM_xmlhttpRequest === "function") ? GM_xmlhttpRequest : GM.xmlHttpRequest)(details);
        },
        download(details) {
            return new DownloadTask(this, details);
        },


        copyText(text) {
            return copyTextToClipboard(text);
            async function copyTextToClipboard(text) {
                // 复制文本
                try {
                    await navigator.clipboard.writeText(text);
                } catch (e) {
                    var copyFrom = document.createElement("textarea");
                    copyFrom.textContent = text;
                    document.body.appendChild(copyFrom);
                    copyFrom.select();
                    document.execCommand('copy');
                    copyFrom.blur();
                    document.body.removeChild(copyFrom);
                }

            }
        },
        message(text, disappearTime = 5000) {
            const id = "f8243rd238-gm-message-panel";
            let p = document.querySelector(`#${id}`);
            if (!p) {
                p = document.createElement("div");
                p.id = id;
                p.style = `
                    position: fixed;
                    bottom: 20px;
                    right: 20px;
                    display: flex;
                    flex-direction: column;
                    align-items: end;
                    z-index: 999999999999999;
                `;
                (document.body || document.documentElement).appendChild(p);
            }
            let mdiv = document.createElement("div");
            mdiv.innerText = text;
            mdiv.style = `
                padding: 3px 8px;
                border-radius: 5px;
                background: black;
                box-shadow: #000 1px 2px 5px;
                margin-top: 10px;
                font-size: small;
                color: #fff;
                text-align: right;
            `;
            p.appendChild(mdiv);
            setTimeout(() => {
                p.removeChild(mdiv);
            }, disappearTime);
        },
        waitEle(selector) {
            return new Promise((resolve, reject) => {
                const found = document.querySelector(selector);
                if (found) {
                    resolve(found);
                    return;
                }

                const observer = new MutationObserver(() => {
                    const ele = document.querySelector(selector);
                    if (!ele) return;
                    observer.disconnect();
                    clearTimeout(timer);
                    resolve(ele);
                });

                let timer = null;
                // 只监听新增/删除节点，避免轮询或 attributes 全监听带来的高频开销。
                observer.observe(document.documentElement, {
                    childList: true,
                    subtree: true
                });

                timer = setTimeout(() => {
                    observer.disconnect();
                    reject(new Error(`等待元素超时: ${selector}`));
                }, 15000);
            });
        }
    };


    function headersToObject(headers) {
        if (!headers) return {};
        if (headers instanceof Headers) {
            return Object.fromEntries(headers.entries());
        }
        if (Array.isArray(headers)) {
            return Object.fromEntries(headers);
        }
        return { ...headers };
    }

    function parseResponseHeaders(rawHeaders) {
        const headers = new Headers();
        String(rawHeaders || "")
            .split(/\r?\n/)
            .filter(Boolean)
            .forEach((line) => {
                const index = line.indexOf(":");
                if (index === -1) return;
                const name = line.slice(0, index).trim();
                const value = line.slice(index + 1).trim();
                if (name) headers.append(name, value);
            });
        return headers;
    }

    function getFetchRequestMeta(input, init) {
        const requestLike = input instanceof Request ? input : null;
        const url = requestLike ? requestLike.url : getFetchUrl(input);
        const method = String(init?.method || requestLike?.method || "GET").toUpperCase();
        const headers = {
            ...headersToObject(requestLike?.headers),
            ...headersToObject(init?.headers)
        };

        return {
            url,
            method,
            headers,
            body: init?.body,
            canProxy: !init?.body && (method === "GET" || method === "HEAD")
        };
    }

    function createProxyResponse({ url, status, statusText, headers, arrayBuffer }) {
        const responseHeaders = headers instanceof Headers ? headers : new Headers(headers || {});
        const bodyBuffer = arrayBuffer || new ArrayBuffer(0);
        return {
            ok: status >= 200 && status < 300,
            status,
            statusText: statusText || "",
            url,
            redirected: false,
            type: "basic",
            headers: responseHeaders,
            async text() {
                return new TextDecoder().decode(bodyBuffer);
            },
            async json() {
                return JSON.parse(await this.text());
            },
            async arrayBuffer() {
                return bodyBuffer.slice(0);
            },
            async blob() {
                return new Blob([bodyBuffer], { type: responseHeaders.get("content-type") || "" });
            },
            clone() {
                return createProxyResponse({
                    url,
                    status,
                    statusText,
                    headers: new Headers(responseHeaders),
                    arrayBuffer: bodyBuffer.slice(0)
                });
            }
        };
    }

    if (location.host === "tools.thatwind.com" || location.host === "localhost:3000") {
        mgmapi.addStyle("#userscript-tip{display:none !important;}");

        let hostNeedsProxy = new Set();

        // 对请求做代理
        const _fetch = unsafeWindow.fetch;
        unsafeWindow.fetch = async function (input, init) {
            const meta = getFetchRequestMeta(input, init);
            if (!meta.url) {
                return await _fetch.apply(this, arguments);
            }

            let hostname = new URL(meta.url).hostname;

            if (hostNeedsProxy.has(hostname)) {
                if (meta.canProxy) {
                    return await mgmapiFetch(meta);
                }
                return await _fetch.apply(this, arguments);
            }

            try {
                let response = await _fetch.apply(this, arguments);
                return response;
            } catch (e) {
                // 失败请求使用代理
                if (meta.canProxy) {
                    console.log(`域名 ${hostname} 需要请求代理，url示例：${meta.url}`);
                    hostNeedsProxy.add(hostname);
                    return await mgmapiFetch(meta);
                } else {
                    throw e;
                }
            }
        }

        function mgmapiFetch(meta) {
            return new Promise((resolve, reject) => {
                let referer = new URLSearchParams(location.hash.slice(1)).get("referer");
                let headers = {
                    ...meta.headers
                };
                if (referer) {
                    referer = new URL(referer);
                    headers = {
                        ...headers,
                        "origin": referer.origin,
                        "referer": referer.href
                    };
                }
                mgmapi.xmlHttpRequest({
                    method: meta.method,
                    url: meta.url,
                    responseType: 'arraybuffer',
                    headers,
                    onload(r) {
                        const responseHeaders = parseResponseHeaders(r.responseHeaders);
                        resolve(createProxyResponse({
                            url: meta.url,
                            status: r.status,
                            statusText: responseHeaders.get("statusText") || "",
                            headers: responseHeaders,
                            arrayBuffer: r.response
                        }));
                    },
                    onerror() {
                        reject(new Error());
                    }
                });
            });
        }

        return;
    }


    // iframe 信息交流
    // 目前只用于获取顶部标题
    window.addEventListener("message", async (e) => {
        if (e.data === "3j4t9uj349-gm-get-title") {
            let name = `top-title-${Date.now()}`;
            await mgmapi.setValue(name, document.title);
            e.source.postMessage(`3j4t9uj349-gm-top-title-name:${name}`, "*");
        }
    });

    function getTopTitle() {
        return new Promise(resolve => {
            window.addEventListener("message", async function l(e) {
                if (typeof e.data === "string") {
                    if (e.data.startsWith("3j4t9uj349-gm-top-title-name:")) {
                        let name = e.data.slice("3j4t9uj349-gm-top-title-name:".length);
                        await new Promise(r => setTimeout(r, 5)); // 等5毫秒 确定 setValue 已经写入
                        resolve(await mgmapi.getValue(name));
                        mgmapi.deleteValue(name);
                        window.removeEventListener("message", l);
                    }
                }
            });
            window.top.postMessage("3j4t9uj349-gm-get-title", "*");
        });
    }

    // m3u8 检测策略说明：
    // 旧实现是全局改写 Response.prototype.text，然后任何文本响应被读取时都顺手检查一次。
    // 这种做法优点是简单粗暴，缺点是拦截面太大，容易把无关接口也卷进来。
    // 这里改成“有限拦截 + 多层过滤”：
    // 1. 只在 fetch/XHR 出口做检查；
    // 2. 只检查 GET；
    // 3. 只检查体积较小的文本响应；
    // 4. URL / content-type 先做启发式筛选；
    // 5. 最后再用正文首行 #EXTM3U 做最终确认。
    const M3U8_TEXT_SIZE_LIMIT = 512 * 1024;
    // 常见的 m3u8 专用内容类型。
    const M3U8_CONTENT_TYPES = [
        "application/vnd.apple.mpegurl",
        "application/x-mpegurl",
        "audio/mpegurl",
        "audio/x-mpegurl"
    ];
    // 一些站点会错误地把 m3u8 当成普通文本返回，所以保留文本类兜底。
    // 但命中这里只代表“值得进一步看正文”，不代表已经确认是 m3u8。
    const TEXT_LIKE_CONTENT_TYPES = [
        "text/",
        "application/json",
        "application/xml",
        "application/javascript",
        "application/x-www-form-urlencoded"
    ];
    // 调度状态：
    // scheduled 负责避免重复 queueMicrotask，
    // running 负责避免正在消费时再次重入消费逻辑。
    let m3uQueueScheduled = false;
    let m3uQueueRunning = false;
    // pending 记录“已经入队但还没彻底处理完”的 URL，
    // queue 记录真正待消费的任务体。
    const pendingM3UDetections = new Set();
    const queuedM3UDetections = new Map();
    
    // 资源 URL 规范化：
    // 1. 兼容相对路径；
    // 2. 去掉 hash，避免同一资源仅因片段不同被认为是两份资源；
    // 3. 返回统一 href 作为去重 key。
    function normalizeResourceUrl(rawUrl) {
        try {
            const url = new URL(rawUrl, location.href);
            url.hash = "";
            return url.href;
        } catch {
            return null;
        }
    }

    // 最终内容判定。
    // 前面的 URL / header 过滤都只是为了降低检查成本，真正确认仍然依赖正文首行。
    function checkM3UContent(content) {
        return typeof content === "string" && content.trim().startsWith("#EXTM3U");
    }

    // 兼容不同 header 读取形式：
    // fetch 返回 Headers 对象，而某些兜底实现可能只给原始字符串。
    function getHeaderValue(headers, name) {
        if (!headers) return "";
        if (typeof headers.get === "function") {
            return headers.get(name) || "";
        }
        if (typeof headers === "string") {
            const match = headers.match(new RegExp(`^${name}:\\s*(.+)$`, "im"));
            return match ? match[1] : "";
        }
        return "";
    }

    // 兼容 fetch 的 string / URL / Request 入参。
    // 统一拿到 URL 后，后面的过滤逻辑就不需要关心调用形态差异。
    function getFetchUrl(input) {
        if (!input) return "";
        if (typeof input === "string") return input;
        if (typeof URL !== "undefined" && input instanceof URL) return input.href;
        if (typeof input.url === "string") return input.url;
        return String(input);
    }

    // URL 层面的轻量启发式判断。
    // 这里只是用来缩小候选范围，不要求百分之百准确。
    function isLikelyM3U8Url(rawUrl) {
        try {
            const { href, pathname, search } = new URL(rawUrl, location.href);
            return /\.m3u8($|[?#])/i.test(href)
                || /(?:playlist|manifest|m3u8)/i.test(pathname)
                || /(?:playlist|manifest|m3u8)/i.test(search);
        } catch {
            return false;
        }
    }

    // 判断一个响应是否值得继续 clone().text()。
    // 这一步是收缩拦截面的关键：不是每个响应都值得付出“再读一遍正文”的成本。
    function isInspectableTextResponse(rawUrl, contentType, contentLength) {
        const normalizedType = (contentType || "").toLowerCase();
        const normalizedLength = Number(contentLength);
        const isSmallEnough = !Number.isFinite(normalizedLength) || normalizedLength <= M3U8_TEXT_SIZE_LIMIT;
        const isM3U8Type = M3U8_CONTENT_TYPES.some(type => normalizedType.includes(type));
        const isTextLike = TEXT_LIKE_CONTENT_TYPES.some(type => normalizedType.includes(type));
        return isSmallEnough && (isLikelyM3U8Url(rawUrl) || isM3U8Type || isTextLike);
    }

    // 把疑似资源压入检测队列。
    // 这里先做 shown/pending 去重，是为了避免 fetch 和 XHR 同时命中同一 URL 时重复解析。
    function enqueueM3UDetection({ url, content }) {
        const normalizedUrl = normalizeResourceUrl(url);
        if (!normalizedUrl || !shouldRenderResource("m3u8", normalizedUrl) || pendingM3UDetections.has(normalizedUrl)) {
            return;
        }

        // 用 Set/Map 做去重和排队，避免同一资源被 fetch 与 XHR 双重命中后重复解析。
        queuedM3UDetections.set(normalizedUrl, { url: normalizedUrl, content });
        pendingM3UDetections.add(normalizedUrl);

        if (!m3uQueueScheduled) {
            m3uQueueScheduled = true;
            queueMicrotask(processQueuedM3UDetections);
        }
    }

    // 串行消费队列。
    // 这里故意不并发处理，主要是为了让去重、日志和 UI 注入顺序都保持简单、稳定、可预期。
    async function processQueuedM3UDetections() {
        if (m3uQueueRunning) return;

        m3uQueueScheduled = false;
        m3uQueueRunning = true;
        try {
            // 串行处理可以降低并发解析带来的重复 UI 注入和异常交叉影响。
            for (const [normalizedUrl, payload] of queuedM3UDetections) {
                queuedM3UDetections.delete(normalizedUrl);
                try {
                    await doM3U(payload);
                } catch (error) {
                    console.error("M3U8 检测失败:", error);
                } finally {
                    pendingM3UDetections.delete(normalizedUrl);
                }
            }
        } finally {
            m3uQueueRunning = false;
            if (queuedM3UDetections.size > 0) {
                processQueuedM3UDetections();
            }
        }
    }

    {
        // fetch 分支：
        // 不再全局改写 Response.prototype.text，而是在 fetch 出口就地检查。
        // 这样只影响真正经过 fetch 的响应，侵入范围明显比旧方案小。
        const originalFetch = unsafeWindow.fetch;
        if (typeof originalFetch === "function") {
            unsafeWindow.fetch = async function (input, init) {
                const response = await originalFetch.apply(this, arguments);

                try {
                    const method = ((init && init.method) || input?.method || "GET").toUpperCase();
                    const url = getFetchUrl(input);
                    const contentType = getHeaderValue(response.headers, "content-type");
                    const contentLength = getHeaderValue(response.headers, "content-length");

                    // 只有小体积、文本类、且 URL/类型可疑的响应才继续读正文。
                    // 这样做的目的是保留命中率，同时尽量减少对普通业务接口的打扰。
                    if (method === "GET" && isInspectableTextResponse(url, contentType, contentLength)) {
                        response.clone().text().then((content) => {
                            if (checkM3UContent(content)) {
                                enqueueM3UDetection({ url, content });
                            }
                        }).catch(() => { });
                    }
                } catch (error) {
                    console.debug("fetch 响应检查跳过:", error);
                }

                return response;
            };
        }

        // XHR 分支：
        // 只在 open 阶段挂一次 load 监听，不去改更底层的原型 getter/setter。
        // 这样相对更温和，也更容易和目标站点自身的 XHR 包装共存。
        const originalXHROpen = unsafeWindow.XMLHttpRequest.prototype.open;
        unsafeWindow.XMLHttpRequest.prototype.open = function (method, url) {
            this.__wtmzjkMethod = method;
            this.__wtmzjkUrl = url;

            if (!this.__wtmzjkListenerAttached) {
                this.__wtmzjkListenerAttached = true;
                this.addEventListener("load", () => {
                    try {
                        const requestMethod = String(this.__wtmzjkMethod || "GET").toUpperCase();
                        const requestUrl = getFetchUrl(this.__wtmzjkUrl);
                        if (requestMethod !== "GET") return;
                        if (this.responseType && this.responseType !== "" && this.responseType !== "text") return;

                        const contentType = this.getResponseHeader("content-type") || "";
                        const contentLength = this.getResponseHeader("content-length") || "";
                        // XHR 分支同样坚持“先过滤，再读正文”的原则。
                        // 否则很多普通文本接口也会被拖进检测流程。
                        if (!isInspectableTextResponse(requestUrl, contentType, contentLength)) return;

                        const content = this.responseText;
                        if (checkM3UContent(content)) {
                            enqueueM3UDetection({ url: requestUrl, content });
                        }
                    } catch { }
                });
            }

            return originalXHROpen.apply(this, arguments);
        };

        // 视频扫描改成“初始化一次 + DOM 增量跟踪”，避免每秒全量轮询整页 video。
        whenDOMReady(() => {
            scanVideosInTree(document);
        });
    }

    const rootDiv = document.createElement("div");
    rootDiv.style = `
        position: fixed;
        z-index: 9999999999999999;
        opacity: 0.9;
    `;
    rootDiv.style.display = "none";
    document.documentElement.appendChild(rootDiv);

    const shadowDOM = rootDiv.attachShadow({ mode: 'open' });
    const wrapper = document.createElement("div");
    shadowDOM.appendChild(wrapper);


    // 指示器
    const bar = document.createElement("div");
    bar.style = `
        text-align: right;
    `;
    bar.innerHTML = `
        <span
            class="number-indicator"
            data-number="0"
            style="
                display: inline-flex;
                width: 25px;
                height: 25px;
                background: black;
                padding: 10px;
                border-radius: 100px;
                margin-bottom: 5px;
                cursor: pointer;
                border: 3px solid #83838382;
            "
        >
            <svg
            style="
                filter: invert(1);
            "
            version="1.1" id="Capa_1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" x="0px" y="0px" viewBox="0 0 585.913 585.913" style="enable-background:new 0 0 585.913 585.913;" xml:space="preserve">
                <g>
                    <path d="M11.173,46.2v492.311l346.22,47.402V535.33c0.776,0.058,1.542,0.109,2.329,0.109h177.39
                    c20.75,0,37.627-16.883,37.627-37.627V86.597c0-20.743-16.877-37.628-37.627-37.628h-177.39c-0.781,0-1.553,0.077-2.329,0.124V0
                    L11.173,46.2z M110.382,345.888l-1.37-38.273c-0.416-11.998-0.822-26.514-0.822-41.023l-0.415,0.01
                    c-2.867,12.767-6.678,26.956-10.187,38.567l-10.961,38.211l-15.567-0.582l-9.239-37.598c-2.801-11.269-5.709-24.905-7.725-37.361
                    l-0.25,0.005c-0.503,12.914-0.879,27.657-1.503,39.552L50.84,343.6l-17.385-0.672l5.252-94.208l25.415-0.996l8.499,32.064
                    c2.724,11.224,5.467,23.364,7.428,34.819h0.389c2.503-11.291,5.535-24.221,8.454-35.168l9.643-33.042l27.436-1.071l5.237,101.377
                    L110.382,345.888z M172.479,349.999c-12.569-0.504-23.013-4.272-28.539-8.142l4.504-17.249c3.939,2.226,13.1,6.445,22.373,6.687
                    c12.009,0.32,18.174-5.497,18.174-13.218c0-10.068-9.838-14.683-19.979-14.74l-9.253-0.052v-16.777l8.801-0.066
                    c7.708-0.208,17.646-3.262,17.646-11.905c0-6.121-4.914-10.562-14.635-10.331c-7.95,0.189-16.245,3.914-20.213,6.446l-4.52-16.693
                    c5.693-4.008,17.224-8.11,29.883-8.588c21.457-0.795,33.643,10.407,33.643,24.625c0,11.029-6.197,19.691-18.738,24.161v0.314
                    c12.229,2.216,22.266,11.663,22.266,25.281C213.89,338.188,197.866,351.001,172.479,349.999z M331.104,302.986
                    c0,36.126-19.55,52.541-51.193,51.286c-29.318-1.166-46.019-17.103-46.019-52.044v-61.104l25.711-1.006v64.201
                    c0,19.191,7.562,29.146,21.179,29.502c14.234,0.368,22.189-8.976,22.189-29.26v-66.125l28.122-1.097v65.647H331.104z
                    M359.723,70.476h177.39c8.893,0,16.125,7.236,16.125,16.126v411.22c0,8.888-7.232,16.127-16.125,16.127h-177.39
                    c-0.792,0-1.563-0.116-2.329-0.232V380.782c17.685,14.961,40.504,24.032,65.434,24.032c56.037,0,101.607-45.576,101.607-101.599
                    c0-56.029-45.581-101.603-101.607-101.603c-24.93,0-47.749,9.069-65.434,24.035V70.728
                    C358.159,70.599,358.926,70.476,359.723,70.476z M390.873,364.519V245.241c0-1.07,0.615-2.071,1.586-2.521
                    c0.981-0.483,2.13-0.365,2.981,0.307l93.393,59.623c0.666,0.556,1.065,1.376,1.065,2.215c0,0.841-0.399,1.67-1.065,2.215
                    l-93.397,59.628c-0.509,0.4-1.114,0.61-1.743,0.61l-1.233-0.289C391.488,366.588,390.873,365.585,390.873,364.519z" />
                </g>
            </svg>
        </span>
    `;

    wrapper.appendChild(bar);

    // 样式
    const style = document.createElement("style");

    style.innerHTML = `
        .number-indicator{
            position:relative;
        }

        .number-indicator::after{
            content: attr(data-number);
            position: absolute;
            bottom: 0;
            right: 0;
            color: #40a9ff;
            font-size: 14px;
            font-weight: bold;
            background: #000;
            border-radius: 10px;
            padding: 3px 5px;
        }

        .copy-link:active{
            color: #ccc;
        }

        .download-btn:hover{
            text-decoration: underline;
        }
        .download-btn:active{
            opacity: 0.9;
        }

        .stop-btn:hover{
            text-decoration: underline;
        }
        .stop-btn:active{
            opacity: 0.9;
        }

        .m3u8-item{
            color: white;
            margin-bottom: 5px;
            display: flex;
            flex-direction: row;
            background: black;
            padding: 5px 10px;
            border-radius: 5px;
            font-size: 14px;
            user-select: none;
        }

        [data-shown="false"] {
            opacity: 0.8;
            zoom: 0.8;
        }

        [data-shown="false"]:hover{
            opacity: 1;
        }

        [data-shown="false"] .m3u8-item{
            display: none;
        }

    `;

    wrapper.appendChild(style);




    const barBtn = bar.querySelector(".number-indicator");

    // 关于显隐和移动

    (async function () {

        let shown = await GM_getValue("shown", true);
        wrapper.setAttribute("data-shown", shown);


        let x = await GM_getValue("x", 10);
        let y = await GM_getValue("y", 10);

        x = Math.min(innerWidth - 50, x);
        y = Math.min(innerHeight - 50, y);

        if (x < 0) x = 0;
        if (y < 0) y = 0;

        rootDiv.style.top = `${y}px`;
        rootDiv.style.right = `${x}px`;

        barBtn.addEventListener("mousedown", e => {
            let startX = e.pageX;
            let startY = e.pageY;

            let moved = false;

            let mousemove = e => {
                let offsetX = e.pageX - startX;
                let offsetY = e.pageY - startY;
                if (moved || (Math.abs(offsetX) + Math.abs(offsetY)) > 5) {
                    moved = true;
                    rootDiv.style.top = `${y + offsetY}px`;
                    rootDiv.style.right = `${x - offsetX}px`;
                }
            };
            let mouseup = e => {

                let offsetX = e.pageX - startX;
                let offsetY = e.pageY - startY;

                if (moved) {
                    x -= offsetX;
                    y += offsetY;
                    mgmapi.setValue("x", x);
                    mgmapi.setValue("y", y);
                } else {
                    shown = !shown;
                    mgmapi.setValue("shown", shown);
                    wrapper.setAttribute("data-shown", shown);
                }

                removeEventListener("mousemove", mousemove);
                removeEventListener("mouseup", mouseup);
            }
            addEventListener("mousemove", mousemove);
            addEventListener("mouseup", mouseup);
        });
    })();


    let count = 0;
    // 统一资源索引：
    // 以后无论是 m3u8、直链视频，还是其他可下载资源，都可以往这个 store 里挂统一元数据，
    // 避免继续依赖单一 Set/数组导致状态越来越散。
    const resourceStore = new Map();
    // DOM 级去重：
    // video/magnet 的 DOM 注入是“同一个节点只处理一次”的问题，和 URL 资源去重不是一回事，
    // 所以这里单独用 WeakSet 跟踪节点级处理状态。
    const trackedVideoElements = new WeakSet();
    const processedMagnetTextNodes = new WeakSet();
    const processedMagnetElements = new WeakSet();
    const magnetAttrSelectors = ['href', 'value', 'data-clipboard-text', 'data-value', 'title', 'alt', 'data-url', 'data-magnet', 'data-copy'];

    // 资源唯一键目前由“资源类型 + 规范化 URL”组成。
    // 这样同一个 URL 的 m3u8 与 mp4 不会互相覆盖，但同类资源能稳定去重。
    function createResourceKey(type, normalizedUrl) {
        return `${type}:${normalizedUrl}`;
    }

    function getResource(type, rawUrl) {
        const normalizedUrl = normalizeResourceUrl(rawUrl);
        if (!normalizedUrl) return null;

        const key = createResourceKey(type, normalizedUrl);
        let resource = resourceStore.get(key);
        if (!resource) {
            resource = {
                key,
                type,
                rawUrl,
                normalizedUrl,
                rendered: false
            };
            resourceStore.set(key, resource);
        }

        return resource;
    }

    function shouldRenderResource(type, rawUrl) {
        const resource = getResource(type, rawUrl);
        return !!(resource && !resource.rendered);
    }

    function markResourceRendered(type, rawUrl, extra = {}) {
        const resource = getResource(type, rawUrl);
        if (!resource) return null;
        Object.assign(resource, extra, { rendered: true });
        return resource;
    }

    // 处理一个具体的 <video> 元素。
    // 这里不再扫描整页，而是在元素生命周期事件触发时按需检查。
    function detectVideoElement(video) {
        if (!(video instanceof HTMLVideoElement)) return;
        if (!video.duration || !video.src || !video.src.startsWith("http")) return;
        if (!shouldRenderResource("video", video.src)) return;

        const src = video.src;

        let { updateDownloadState } = showVideo({
            type: "video",
            url: new URL(src),
            duration: `${Math.ceil(video.duration * 10 / 60) / 10} ${T.mins}`,
            download() {
                const details = {
                    url: src,
                    name: (() => {
                        let name = new URL(src).pathname.split("/").slice(-1)[0];
                        if (!/\.\w+$/.test(name)) {
                            if (name.match(/^\s*$/)) name = Date.now();
                            name = name + ".mp4";
                        }
                        return name;
                    })(),
                    headers: {
                        origin: location.origin
                    },
                    onError(e) {
                        console.error(e);
                        updateDownloadState({
                            downloading: false,
                            cancel: null,
                            progress: 0
                        });

                        mgmapi.openInTab(src);
                        mgmapi.message("下载失败，链接已在新窗口打开", 5000);
                    },
                    reportProgress(progress) {
                        updateDownloadState({
                            downloading: true,
                            cancel: null,
                            progress
                        });
                    },
                    onComplete() {
                        mgmapi.message("下载完成", 5000);
                        updateDownloadState({
                            downloading: false,
                            cancel: null,
                            progress: 0
                        });
                    },
                    onStop() {
                        updateDownloadState({
                            downloading: false,
                            cancel: null,
                            progress: 0
                        });
                    }
                };
                let { cancel } = mgmapi.download(details);

                updateDownloadState({
                    downloading: true,
                    cancel() {
                        cancel();
                    },
                    progress: 0
                });
            }
        });

        markResourceRendered("video", src, {
            duration: video.duration
        });
    }

    // 给 video 元素挂一次性跟踪器。
    // 后面如果这个元素懒加载元数据或切换 src，事件会再次触发 detectVideoElement。
    function trackVideoElement(video) {
        if (!(video instanceof HTMLVideoElement) || trackedVideoElements.has(video)) return;
        trackedVideoElements.add(video);

        const handleVideoState = () => detectVideoElement(video);
        video.addEventListener("loadedmetadata", handleVideoState);
        video.addEventListener("durationchange", handleVideoState);
        video.addEventListener("canplay", handleVideoState);

        handleVideoState();
    }

    function collectVideoElements(root) {
        if (!root) return [];
        if (root instanceof HTMLVideoElement) return [root];
        if (!(root instanceof Element || root instanceof Document || root instanceof DocumentFragment)) return [];
        return Array.from(root.querySelectorAll("video"));
    }

    function scanVideosInTree(root) {
        for (const video of collectVideoElements(root)) {
            trackVideoElement(video);
        }
    }


    // 处理一个已经确认或高度怀疑为 m3u8 的资源。
    // 这一步负责 URL 规范化、清单解析和面板项渲染，不负责网络层拦截。
    async function doM3U({ url, content }) {

        url = new URL(url, location.href);
        const normalizedUrl = normalizeResourceUrl(url.href);

        if (!normalizedUrl || !shouldRenderResource("m3u8", normalizedUrl)) return;

        // 如果队列里已经带了正文，就直接复用；
        // 没带正文时才兜底再请求一次，避免平白多一次网络访问。
        content = content || await (await fetch(url)).text();
        if (!checkM3UContent(content)) return;

        // 解析 m3u8 清单结构，给 UI 提供总时长或“多轨”信息。
        const parser = new m3u8Parser.Parser();
        parser.push(content);
        parser.end();
        const manifest = parser.manifest;

        if (manifest.segments) {
            let duration = 0;
            manifest.segments.forEach((segment) => {
                duration += segment.duration;
            });
            manifest.duration = duration;
        }

        showVideo({
            type: "m3u8",
            url,
            duration: manifest.duration ? `${Math.ceil(manifest.duration * 10 / 60) / 10} ${T.mins}` : manifest.playlists ? `${T.multiLine}(${manifest.playlists.length})` : "未知(unknown)",
            async download() {
                mgmapi.openInTab(
                    `https://tools.thatwind.com/tool/m3u8downloader#${new URLSearchParams({
                        m3u8: url.href,
                        referer: location.href,
                        filename: (await getTopTitle()) || ""
                    })}`
                );
            }
        })

    }

    // 渲染一个资源条目到悬浮面板。
    // 这里既是 UI 创建点，也是 UI 与下载任务之间的连接点。
    function showVideo({
                           type,
                           url,
                           duration,
                           download
                       }) {
        let div = document.createElement("div");
        div.className = "m3u8-item";
        div.innerHTML = `
            <span ${type == "m3u8" ? "style=\"color:#40a9ff\"" : ""}>${type}</span>
            <span
                title="${url}"
                style="
                    color: #ccc;
                    font-size: small;
                    max-width: 200px;
                    text-overflow: ellipsis;
                    white-space: nowrap;
                    overflow: hidden;
                    margin-left: 10px;
                "
            >${url.pathname}</span>
            <span
                style="
                    color: #ccc;
                    margin-left: 10px;
                    flex-grow: 1;
                "
            >${duration}</span>
            <span
                class="copy-link"
                title="${url}"
                style="
                    margin-left: 10px;
                    cursor: pointer;
                "
            >${T.copy}</span>
            <span
                class="download-btn"
                style="
                    margin-left: 10px;
                    cursor: pointer;
            ">${T.download}</span>
            <span>
                <span
                    class="progress"
                    style="
                        display: none;
                        margin-left: 10px;
                    "
                ></span>
            <span
                class="stop-btn"
                style="
                    display: none;
                    margin-left: 10px;
                    cursor: pointer;
            ">${T.stop}</span>
        `;



        // 当前条目只缓存一个“当前可取消任务”的入口。
        // UI 不需要知道 DownloadTask 的完整结构，只需要知道如何停止即可。
        let cancelDownload;

        let downloadBtn = div.querySelector(".download-btn");
        let stopBtn = div.querySelector(".stop-btn");
        let progressText = div.querySelector(".progress");

        div.querySelector(".copy-link").addEventListener("click", () => {
            // 复制链接
            mgmapi.copyText(url.href);
            mgmapi.message(T.copied, 2000);
        });

        downloadBtn.addEventListener("click", download);

        stopBtn.addEventListener("click", () => {
            cancelDownload && cancelDownload();
        });

        rootDiv.style.display = "block";

        count++;

        // UI 展示成功后再登记，避免失败分支把资源永久标记为“已展示”。
        markResourceRendered(type, url.href);

        bar.querySelector(".number-indicator").setAttribute("data-number", count);

        wrapper.appendChild(div);

        return {
            // 通过这个方法让下载逻辑把状态同步回 UI。
            // 这样 UI 与下载器之间保持单向依赖：下载器告诉 UI“现在是什么状态”，
            // UI 再决定显示下载按钮、进度文案还是停止按钮。
            updateDownloadState({ downloading, progress, cancel }) {
                if (downloading) {
                    if (cancel) cancelDownload = cancel;
                    downloadBtn.style.display = "none";
                    progressText.style.display = "";
                    progressText.textContent = `${T.downloading} ${progress}%`;
                    stopBtn.style.display = "";
                } else {
                    cancelDownload = null;
                    downloadBtn.style.display = "";
                    progressText.style.display = "none";
                    stopBtn.style.display = "none";
                }
            }
        }
    }

    // - PLAY

    let pikpakLogged = false;

    (async function refreshLogState() {
        pikpakLogged = await mgmapi.getValue("pikpak-logged", false);
        if (!pikpakLogged) {
            setTimeout(refreshLogState, 5000);
        }
    })();

    if (location.host.endsWith("pikpak.com")) {

        const _fetch = unsafeWindow.fetch;
        unsafeWindow.fetch = (...arg) => {
            if (arg[0].includes('area_accessible')) {
                return new Promise(() => {
                    throw new Error();
                });
            } else {
                return _fetch(...arg);
            }
        };

        whenLoad(async () => {
            for (let i = 0; i < 20; i++) {
                if (document.querySelector("a.avatar-box") && document.querySelector("a.avatar-box").clientWidth) {
                    mgmapi.setValue("pikpak-logged", true);
                    break;
                }
                await sleep(1000);
            }
        });

        let link = new URLSearchParams(location.hash.slice(1)).get("link");
        if (link) {
            whenLoad(async () => {
                // await sleep(3000);

                let input = await mgmapi.waitEle(`.public-page-input input[type="text"]`);
                let button = await mgmapi.waitEle(`.public-page-input button`);

                input.value = link;
                input.dispatchEvent(new Event("input"));
                input.dispatchEvent(new Event("blur"));

                await sleep(100);

                button.click();

            });
        }

        // location.hash = '';
    }

    const reg = /magnet:\?xt=urn:btih:\w{10,}([-a-zA-Z0-9()@:%_\+.~#?&//=]*)/;

    whenDOMReady(() => {
        // 样式部分：重构为按钮组 (Button Group) 风格
        mgmapi.addStyle(`
            /* 按钮组容器 */
            .wtmzjk-btn-group {
                display: inline-flex;
                align-items: center;
                margin: 2px 8px;
                border-radius: 6px; /* 整体圆角 */
                overflow: hidden;   /* 确保子元素不溢出圆角 */
                box-shadow: 0 2px 5px rgba(0,0,0,0.15);
                vertical-align: middle;
                font-size: 12px;
                line-height: 1;
            }

            /* 按钮通用样式 */
            .wtmzjk-btn {
                all: initial;
                display: inline-flex;
                align-items: center;
                justify-content: center;
                padding: 6px 10px;
                cursor: pointer;
                background: #306eff;; /* 主色调，可调整 */
                color: white;
                border: none;
                font-family: sans-serif;
                font-size: inherit;
                font-weight: 600;
                transition: background 0.2s, filter 0.2s;
                text-decoration: none;
                height: 24px;
                box-sizing: border-box;
            }

            .wtmzjk-btn:hover {
                background: #497dfd; /* Hover 深色 */
            }

            .wtmzjk-btn:active {
                background: #1e5ced; /* Active 更深 */
            }

            /* 图标样式 */
            .wtmzjk-btn svg, .wtmzjk-btn img {
                height: 14px;
                width: 14px;
                fill: white;
                pointer-events: none;
                margin-right: 4px; /* 图标与文字间距 */
            }

            /* 仅图标模式修正 */
            .wtmzjk-btn.icon-only svg {
                margin-right: 0;
            }

            /* 分割线：通过右边框实现 */
            .wtmzjk-btn:not(:last-child) {
                border-right: 1px solid rgba(255, 255, 255, 0.3);
            }
        `);

        // 事件监听保持不变，稍作逻辑调整以适应新结构
        window.addEventListener("click", onEvents, true);
        window.addEventListener("mousedown", onEvents, true); // 如果不需要拖拽等操作，通常 click 就够了
        window.addEventListener("mouseup", onEvents, true);

        work(document.body);
        watchBodyChange(work);
    });

    function onEvents(e) {
        // 向上查找，防止点击到图标或span时失效
        const target = e.target.closest('[data-wtmzjk-action]');

        if (target) {
            e.preventDefault();
            e.stopPropagation();

            // 仅在 click 时触发，避免 mouseup/down 重复触发
            if (e.type !== "click") return;

            const action = target.getAttribute('data-wtmzjk-action');
            const url = target.getAttribute('data-wtmzjk-url');

            if (action === 'play') {
                let a = document.createElement('a');
                // 保持你原有的逻辑
                if (pikpakLogged) {
                    a.href = `https://mypikpak.com/drive/all?action=create_task&url=${encodeURIComponent(url)}&launcher=url_checker&speed_save=1&scene=official_website&invitation-code=86120234`;
                } else {
                    a.href = 'https://mypikpak.com?invitation-code=86120234#' + new URLSearchParams({ link: url });
                }

                a.target = "_blank";
                a.click();
            } else if (action === 'copy') {
                // 实现复制功能
                mgmapi.copyText(url).then(() => {
                    // 简单的视觉反馈
                    const originalText = target.querySelector('span').innerText;
                    target.querySelector('span').innerText = T.copied;
                    setTimeout(() => {
                        target.querySelector('span').innerText = originalText;
                    }, 2000);
                }).catch(err => {
                    console.error('Copy failed', err);
                });
            }
        }
    }

    function createWatchButton(url, isForPlain = false) {
        // 创建容器
        let group = document.createElement("div");
        group.className = "wtmzjk-btn-group";
        if (isForPlain) group.setAttribute('data-wtmzjk-button-for-plain', '');

        // 1. 复制按钮 (左侧)
        let copyBtn = document.createElement("button");
        copyBtn.className = "wtmzjk-btn";
        copyBtn.setAttribute('data-wtmzjk-action', 'copy');
        copyBtn.setAttribute('data-wtmzjk-url', url);
        copyBtn.title = T.copy;
        // 这里的图标可以换成你想要的 Copy 图标
        copyBtn.innerHTML = `
            <svg viewBox="0 0 448 512"><path d="M384 336H192c-8.8 0-16-7.2-16-16V64c0-8.8 7.2-16 16-16l140.1 0L400 115.9V320c0 8.8-7.2 16-16 16zM192 384H384c35.3 0 64-28.7 64-64V115.9c0-12.7-5.1-24.9-14.1-33.9L366.1 14.1c-9-9-21.2-14.1-33.9-14.1H192c-35.3 0-64 28.7-64 64V320c0 35.3 28.7 64 64 64zM64 128c-35.3 0-64 28.7-64 64V448c0 35.3 28.7 64 64 64H256c35.3 0 64-28.7 64-64V416H272v32c0 8.8-7.2 16-16 16H64c-8.8 0-16-7.2-16-16V192c0-8.8 7.2-16 16-16H96V128H64z"/></svg>
            <span>${T.copy}</span>
        `;

        // 2. 播放按钮 (右侧)
        let playBtn = document.createElement("button");
        playBtn.className = "wtmzjk-btn";
        playBtn.setAttribute('data-wtmzjk-action', 'play');
        playBtn.setAttribute('data-wtmzjk-url', url);
        playBtn.title = T.play;
        // 注意：这里是你要求的自定义图标位置，src 留空给你填
        playBtn.innerHTML = `
            <svg style="width: auto;height: 20px;" width="60" height="60" viewBox="0 0 60 60" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path fill="#FFFFFF" clip-rule="evenodd" fill-rule="evenodd" d="M26.5835 15.2408C27.0625 15.1802 27.4695 14.8617 27.6434 14.4113L29.0836 10.6824C29.2028 10.3737 28.9496 10.0495 28.6213 10.0904L13.4653 11.9768C12.6864 12.0738 12.0192 12.5804 11.7165 13.3045L10.032 17.3354L16.8876 16.4679L26.5835 15.2408ZM33.4485 15.2408C32.9695 15.1802 32.5625 14.8617 32.3885 14.4113L30.9484 10.6824C30.8292 10.3737 31.0824 10.0495 31.4107 10.0904L46.5667 11.9768C47.3455 12.0738 48.0128 12.5804 48.3154 13.3045L50 17.3354L33.4485 15.2408ZM10 17.336H50V39.3048C50 41.7755 47.9971 43.7784 45.5263 43.7784H14.4737C12.0029 43.7784 10 41.7755 10 39.3048V17.336ZM22.889 24.2091C23.8772 24.2091 24.6784 25.0102 24.6784 25.9985V29.5541C24.6784 30.5424 23.8772 31.3435 22.889 31.3435C21.9007 31.3435 21.0995 30.5424 21.0995 29.5541V25.9985C21.0995 25.0102 21.9007 24.2091 22.889 24.2091ZM38.9006 25.9985C38.9006 25.0102 38.0994 24.2091 37.1111 24.2091C36.1228 24.2091 35.3217 25.0102 35.3217 25.9985V29.5541C35.3217 30.5424 36.1228 31.3435 37.1111 31.3435C38.0994 31.3435 38.9006 30.5424 38.9006 29.5541V25.9985ZM35.4241 36.7358C35.6534 37.314 35.3706 37.9687 34.7924 38.198L34.615 37.7507C34.6956 37.954 34.7923 38.1981 34.792 38.1982L34.7906 38.1987L34.788 38.1998L34.7803 38.2028L34.7552 38.2125C34.7343 38.2205 34.705 38.2316 34.668 38.2454C34.5939 38.2728 34.4885 38.3108 34.3562 38.3558C34.0921 38.4457 33.7183 38.5645 33.2711 38.683C32.3873 38.9173 31.1692 39.1638 29.9243 39.1638C28.6786 39.1638 27.4701 38.9171 26.5947 38.6821C26.152 38.5633 25.7828 38.4443 25.522 38.354C25.3913 38.3089 25.2872 38.2707 25.2141 38.2431C25.1774 38.2293 25.1485 38.2181 25.1278 38.21L25.1029 38.2002L25.0952 38.1971L25.0925 38.1961L25.0911 38.1955C25.0908 38.1954 25.3266 37.6115 25.3889 37.4575L25.0907 38.1953C24.514 37.9623 24.2354 37.3058 24.4685 36.7291C24.7014 36.1528 25.3571 35.8742 25.9335 36.1063L25.9352 36.107L25.948 36.1121C25.9605 36.1169 25.9808 36.1248 26.0085 36.1353C26.064 36.1561 26.1486 36.1872 26.2582 36.2252C26.478 36.3011 26.7958 36.4037 27.1787 36.5065C27.9545 36.7148 28.9518 36.9112 29.9243 36.9112C30.8975 36.9112 31.9059 36.7145 32.6939 36.5056C33.0826 36.4026 33.4062 36.2997 33.6304 36.2234C33.7423 36.1853 33.8288 36.154 33.8856 36.133C33.9139 36.1225 33.9349 36.1145 33.9478 36.1096L33.961 36.1045L33.9618 36.1041L33.9622 36.104L33.9624 36.1039L33.9628 36.1038C34.5408 35.8752 35.1949 36.158 35.4241 36.7358Z" />
            </svg>
            <span>${T.play}</span>
        `;

        //             <svg style="margin-left:4px; margin-right:0;" viewBox="0 0 384 512"><path d="M73 39c-14.8-9.1-33.4-9.4-48.5-.9S0 62.6 0 80V432c0 17.4 9.4 33.4 24.5 41.9s33.7 8.1 48.5-.9L361 297c14.3-8.7 23-24.2 23-41s-8.7-32.2-23-41L73 39z"/></svg>


        group.appendChild(copyBtn);
        group.appendChild(playBtn);

        return group;
    }

    // 处理正文里的纯文本 magnet 链接。
    // 这里只处理单个文本节点，不再像旧逻辑那样每次都递归扫完整个 body。
    function processMagnetTextNode(node) {
        if (!(node instanceof Text) || processedMagnetTextNodes.has(node)) return;
        if (!node.parentNode || !node.nodeValue || /^\s*$/.test(node.nodeValue)) return;
        if (node.nextSibling && node.nextSibling.hasAttribute && node.nextSibling.className.includes('wtmzjk-btn-group')) return;

        const match = node.nodeValue.match(reg);
        if (!match) return;

        const url = match[0];
        const parent = node.parentNode;
        processedMagnetTextNodes.add(node);
        parent.insertBefore(document.createTextNode(node.nodeValue.slice(0, match.index + url.length)), node);
        parent.insertBefore(createWatchButton(url, true), node);
        parent.insertBefore(document.createTextNode(node.nodeValue.slice(match.index + url.length)), node);
        parent.removeChild(node);
    }

    // 处理属性里携带 magnet 的元素，例如 href、data-url、title 等。
    function processMagnetElement(element) {
        if (!(element instanceof Element) || processedMagnetElements.has(element)) return;
        if (element.nextSibling && element.nextSibling.hasAttribute && element.nextSibling.className.includes('wtmzjk-btn-group')) {
            processedMagnetElements.add(element);
            return;
        }
        if (reg.test(element.textContent || "")) return;

        for (let attr of element.getAttributeNames()) {
            let val = element.getAttribute(attr);
            if (!val || !reg.test(val)) continue;
            let url = val.match(reg)[0];
            element.parentNode?.insertBefore(createWatchButton(url), element.nextSibling);
            processedMagnetElements.add(element);
            break;
        }
    }

    // 处理一个增量 root。
    // root 可以是新增元素子树，也可以是刚刚变动过的文本节点，因此这里分别处理。
    function processMagnetRoot(root) {
        if (!root) return;

        if (root instanceof Text) {
            processMagnetTextNode(root);
            return;
        }

        if (!(root instanceof Element || root instanceof Document || root instanceof DocumentFragment)) return;

        const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
            acceptNode(node) {
                const parentTag = node.parentElement?.tagName;
                if (["STYLE", "SCRIPT", "BASE", "COMMAND", "LINK", "META", "TITLE", "XTRANS-TXT", "XTRANS-TXT-GROUP", "XTRANS-POPUP"].includes(parentTag)) {
                    return NodeFilter.FILTER_REJECT;
                }
                return /^\s*$/.test(node.nodeValue || "") ? NodeFilter.FILTER_SKIP : NodeFilter.FILTER_ACCEPT;
            }
        });

        let currentTextNode;
        while ((currentTextNode = walker.nextNode())) {
            processMagnetTextNode(currentTextNode);
        }

        const selector = magnetAttrSelectors.map(n => `[${n}*="magnet:?xt=urn:btih:"]`).join(',');
        if (root instanceof Element && root.matches(selector)) {
            processMagnetElement(root);
        }
        for (const element of Array.from(root.querySelectorAll(selector))) {
            processMagnetElement(element);
        }
    }

    // 增量工作入口：
    // 同一个入口同时处理 magnet 注入和新增 video 跟踪，避免多个观察器各自全量扫子树。
    function work(root = document.body) {
        if (!document.body || !root) return;
        processMagnetRoot(root);
        scanVideosInTree(root);
    }


    // 观察 DOM 增量变化：
    // 1. 只关心 childList 和 characterData，不再监听所有 attributes；
    // 2. 不再每次变化就重扫全页，而是收集受影响的根节点后批量增量处理。
    function watchBodyChange(onchange) {
        let pendingRoots = new Set();
        let timeout;
        let observer = new MutationObserver((mutations) => {
            for (const mutation of mutations) {
                if (mutation.type === "childList") {
                    mutation.addedNodes.forEach((node) => {
                        if (node instanceof Element || node instanceof Text) {
                            pendingRoots.add(node);
                        }
                    });
                } else if (mutation.type === "characterData" && mutation.target instanceof Text) {
                    pendingRoots.add(mutation.target);
                }
            }

            if (!pendingRoots.size || timeout) return;

            timeout = setTimeout(() => {
                const roots = Array.from(pendingRoots);
                pendingRoots.clear();
                timeout = null;
                for (const root of roots) {
                    onchange(root);
                }
            }, 120);
        });
        observer.observe(document.body || document.documentElement, {
            childList: true,
            subtree: true,
            characterData: true
        });

    }

    function whenDOMReady(f) {
        if (document.body) f();
        else window.addEventListener("DOMContentLoaded", function l() {
            window.removeEventListener("DOMContentLoaded", l);
            f();
        });
    }

    function whenLoad(f) {
        if (document.body) f();
        else window.addEventListener("load", function l() {
            window.removeEventListener("load", l);
            f();
        });
    }

    function sleep(t) {
        return new Promise(resolve => setTimeout(resolve, t));
    }


})();
