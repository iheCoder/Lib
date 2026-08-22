(() => {
  const { STORAGE_KEYS, normalizePreferences } = globalThis.ChatGptNewTabSettings;
  const ACTIVE_ATTRIBUTE = "data-cgnt-wallpaper";
  const WALLPAPER_HOST_ID = "cgnt-wallpaper-host";
  let wallpaperHost;
  let wallpaperSurface;
  let observedConversationArea;
  let resizeObserver;
  let replacementObserver;
  let layoutFrame;

  /** Read one coherent snapshot so a page never renders mixed old/new values. */
  async function readWallpaperState() {
    const stored = await chrome.storage.local.get([
      STORAGE_KEYS.image,
      STORAGE_KEYS.preferences,
    ]);
    return {
      image: typeof stored[STORAGE_KEYS.image] === "string" ? stored[STORAGE_KEYS.image] : "",
      preferences: normalizePreferences(stored[STORAGE_KEYS.preferences]),
    };
  }

  /** Remove every visible value owned by the extension and restore the website. */
  function clearWallpaper(root) {
    root.removeAttribute(ACTIVE_ATTRIBUTE);
    root.style.removeProperty("--cgnt-surface-opacity");
    wallpaperHost?.remove();
    wallpaperHost = undefined;
    wallpaperSurface = undefined;
    stopLayoutTracking();
  }

  /** Translate a user-facing fit mode into explicit, predictable CSS values. */
  function resolveFit(fit) {
    if (fit === "contain") {
      return { size: "contain", repeat: "no-repeat" };
    }
    if (fit === "repeat") {
      return { size: "auto", repeat: "repeat" };
    }
    return { size: "cover", repeat: "no-repeat" };
  }

  /** Select the largest visible main landmark, excluding ChatGPT's sidebar. */
  function findConversationArea() {
    const candidates = [...document.querySelectorAll("main")];
    return candidates
      .map((element) => ({ element, rectangle: element.getBoundingClientRect() }))
      .filter(({ rectangle }) => rectangle.width > 0 && rectangle.height > 0)
      .sort((left, right) => {
        const rightArea = right.rectangle.width * right.rectangle.height;
        const leftArea = left.rectangle.width * left.rectangle.height;
        return rightArea - leftArea;
      })[0];
  }

  /** Rebind geometry observation when ChatGPT replaces its main SPA landmark. */
  function observeConversationArea(element) {
    if (element === observedConversationArea) return;
    resizeObserver?.disconnect();
    observedConversationArea = element;
    if (element) {
      resizeObserver ??= new ResizeObserver(scheduleLayoutSync);
      resizeObserver.observe(element);
    }
  }

  /** Scope the wallpaper to the live conversation rectangle, not the viewport. */
  function syncWallpaperBounds() {
    layoutFrame = undefined;
    if (!wallpaperHost?.isConnected) return;

    const area = findConversationArea();
    observeConversationArea(area?.element);
    const rectangle = area?.rectangle ?? {
      left: 0,
      top: 0,
      width: window.innerWidth,
      height: window.innerHeight,
    };
    Object.assign(wallpaperHost.style, {
      left: `${Math.round(rectangle.left)}px`,
      top: `${Math.round(rectangle.top)}px`,
      width: `${Math.round(rectangle.width)}px`,
      height: `${Math.round(rectangle.height)}px`,
    });
  }

  /** Collapse resize bursts into one layout read and write per animation frame. */
  function scheduleLayoutSync() {
    if (layoutFrame !== undefined) return;
    layoutFrame = window.requestAnimationFrame(syncWallpaperBounds);
  }

  /** Track sidebar transitions and main-node replacement without reading chats. */
  function startLayoutTracking() {
    replacementObserver ??= new MutationObserver(() => {
      if (!observedConversationArea?.isConnected) scheduleLayoutSync();
    });
    replacementObserver.observe(document.body, { childList: true, subtree: true });
    window.addEventListener("resize", scheduleLayoutSync, { passive: true });
    scheduleLayoutSync();
  }

  /** Stop all layout work when the wallpaper is disabled or removed. */
  function stopLayoutTracking() {
    resizeObserver?.disconnect();
    replacementObserver?.disconnect();
    observedConversationArea = undefined;
    window.removeEventListener("resize", scheduleLayoutSync);
    if (layoutFrame !== undefined) {
      window.cancelAnimationFrame(layoutFrame);
      layoutFrame = undefined;
    }
  }

  /** Wait for a safe sibling of ChatGPT's app root instead of entering its tree. */
  async function waitForBody() {
    if (document.body) return document.body;
    await new Promise((resolve) => {
      document.addEventListener("DOMContentLoaded", resolve, { once: true });
    });
    return document.body;
  }

  /**
   * Keep the image data inside a closed shadow root. The host page may see the
   * inert host element, but cannot inspect the local image stored beneath it.
   */
  async function ensureWallpaperSurface() {
    if (wallpaperHost?.isConnected && wallpaperSurface) return wallpaperSurface;

    wallpaperHost = document.createElement("cgnt-wallpaper");
    wallpaperHost.id = WALLPAPER_HOST_ID;
    wallpaperHost.setAttribute("aria-hidden", "true");
    const shadowRoot = wallpaperHost.attachShadow({ mode: "closed" });
    wallpaperSurface = document.createElement("div");
    Object.assign(wallpaperSurface.style, {
      position: "absolute",
      inset: "0",
      backgroundPosition: "center",
    });
    shadowRoot.append(wallpaperSurface);
    (await waitForBody()).prepend(wallpaperHost);
    startLayoutTracking();
    return wallpaperSurface;
  }

  /** Apply sanitized settings while keeping image bytes out of the page DOM. */
  async function applyWallpaper({ image, preferences }) {
    const root = document.documentElement;
    if (!image || !preferences.enabled) {
      clearWallpaper(root);
      return;
    }

    const fit = resolveFit(preferences.fit);
    const surface = await ensureWallpaperSurface();
    root.setAttribute(ACTIVE_ATTRIBUTE, "active");
    root.style.setProperty("--cgnt-surface-opacity", String(preferences.surfaceOpacity / 100));
    const dimming = preferences.dimming / 100;
    surface.style.backgroundImage = [
      `linear-gradient(rgb(0 0 0 / ${dimming}), rgb(0 0 0 / ${dimming}))`,
      `url("${image}")`,
    ].join(", ");
    surface.style.backgroundRepeat = fit.repeat;
    surface.style.backgroundSize = fit.size;
    surface.style.filter = `blur(${preferences.blur}px)`;
  }

  /** Refresh failures must never interfere with the actual ChatGPT application. */
  async function refreshWallpaper() {
    try {
      await applyWallpaper(await readWallpaperState());
    } catch (error) {
      console.warn("ChatGPT New Tab could not apply the local wallpaper.", error);
    }
  }

  chrome.storage.onChanged.addListener((changes, areaName) => {
    const relevantChange = Object.values(STORAGE_KEYS).some((key) => key in changes);
    if (areaName === "local" && relevantChange) {
      void refreshWallpaper();
    }
  });

  void refreshWallpaper();
})();
