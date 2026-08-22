(() => {
  const STORAGE_KEYS = Object.freeze({
    image: "wallpaperImage",
    fileName: "wallpaperFileName",
    preferences: "wallpaperPreferences",
  });

  const DEFAULT_PREFERENCES = Object.freeze({
    enabled: true,
    dimming: 28,
    blur: 0,
    surfaceOpacity: 78,
    fit: "cover",
  });

  const SUPPORTED_FITS = new Set(["cover", "contain", "repeat"]);

  /** Keep stored numeric settings inside the range supported by the UI. */
  function clamp(value, minimum, maximum, fallback) {
    const numericValue = Number(value);
    return Number.isFinite(numericValue)
      ? Math.min(maximum, Math.max(minimum, numericValue))
      : fallback;
  }

  /**
   * Normalize persisted data before it reaches either UI or page CSS. This is
   * also the migration boundary for settings written by future versions.
   */
  function normalizePreferences(candidate = {}) {
    return {
      enabled: candidate.enabled !== false,
      dimming: clamp(candidate.dimming, 0, 80, DEFAULT_PREFERENCES.dimming),
      blur: clamp(candidate.blur, 0, 20, DEFAULT_PREFERENCES.blur),
      surfaceOpacity: clamp(
        candidate.surfaceOpacity,
        45,
        100,
        DEFAULT_PREFERENCES.surfaceOpacity,
      ),
      fit: SUPPORTED_FITS.has(candidate.fit) ? candidate.fit : DEFAULT_PREFERENCES.fit,
    };
  }

  globalThis.ChatGptNewTabSettings = Object.freeze({
    STORAGE_KEYS,
    DEFAULT_PREFERENCES,
    normalizePreferences,
  });
})();
