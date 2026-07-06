const A4_HEIGHT_MM = 297;
const PX_PER_MM = 96 / 25.4;
const PRINT_SAFETY_PX = 28;
const TARGET_USAGE_FLOOR = 0.95;
const MAX_EXPANSION_FACTOR = 4;
const EXPANSION_LIMITS = Object.freeze({
  fontPt: 14,
  lineHeight: 1.68,
  listFactor: 0.28,
  listLineHeight: 1.24,
  paragraphFactor: 2.1,
  sectionFactor: 3.4,
  subheadingFactor: 2.6,
});

/**
 * Measure the rendered document, then tune only bounded CSS variables. The
 * bounds are the readability contract: content that cannot fit is rejected
 * instead of silently becoming an unreadable "one-page" PDF.
 */
export async function fitOnePage(page, options) {
  const availableHeight = (A4_HEIGHT_MM - options.marginMm * 2) * PX_PER_MM;
  // CSS pixels and PDF points round differently near the page boundary. This
  // tiny reserve keeps a visually full page from spilling by a fraction.
  const fitHeight = availableHeight - PRINT_SAFETY_PX;
  const comfortable = {
    fontPt: options.maxFontPt,
    lineHeight: options.lineHeight,
    listFactor: options.listFactor,
    listLineHeight: options.listLineHeight,
    paragraphFactor: options.paragraphFactor,
    sectionFactor: options.sectionFactor,
    subheadingFactor: options.subheadingFactor,
  };
  let measurement = await applyAndMeasure(page, comfortable, fitHeight, availableHeight);

  if (measurement.fits) {
    const expanded = await findLargestFit(page, comfortable, fitHeight, availableHeight);
    return { ...expanded, status: "fit" };
  }

  // Compression happens in perceptual order: remove excess whitespace first,
  // then tighten line height, and only then reduce type toward its hard floor.
  // Preserve heading hierarchy under pressure. Lists and paragraphs tighten
  // first; section boundaries remain visibly stronger than list-item gaps.
  const compact = {
    ...comfortable,
    lineHeight: Math.min(comfortable.lineHeight, 1.36),
    listFactor: Math.max(0, comfortable.listFactor * 0.62),
    listLineHeight: Math.max(1.12, comfortable.listLineHeight - 0.08),
    paragraphFactor: Math.max(0.65, comfortable.paragraphFactor * 0.78),
    sectionFactor: Math.max(0.9, comfortable.sectionFactor * 0.86),
    subheadingFactor: Math.max(0.85, comfortable.subheadingFactor * 0.84),
  };
  measurement = await applyAndMeasure(page, compact, fitHeight, availableHeight);
  if (measurement.fits) {
    const expanded = await findLargestFit(page, compact, fitHeight, availableHeight);
    return { ...expanded, status: "fit" };
  }

  const best = await findBestFontFit(page, compact, fitHeight, availableHeight, options);
  if (best) {
    const expanded = await findLargestFit(page, best.settings, fitHeight, availableHeight);
    return { ...expanded, status: "fit" };
  }

  const failed = await applyAndMeasure(
    page,
    { ...compact, fontPt: options.minFontPt },
    fitHeight,
    availableHeight,
  );
  return { ...failed, status: "overflow" };
}

/** Find the largest readable font that satisfies the strict page-height bound. */
async function findBestFontFit(page, compact, fitHeight, availableHeight, options) {
  let low = options.minFontPt;
  let high = options.maxFontPt;
  let best = null;
  for (let attempt = 0; attempt < 12; attempt += 1) {
    const fontPt = (low + high) / 2;
    const candidate = await applyAndMeasure(page, { ...compact, fontPt }, fitHeight, availableHeight);
    if (candidate.fits) {
      best = candidate;
      low = fontPt;
    } else {
      high = fontPt;
    }
  }
  return best;
}

/** Spend spare vertical room on rhythm while preserving the preferred font. */
async function findLargestFit(page, base, fitHeight, availableHeight) {
  let low = 1;
  let high = MAX_EXPANSION_FACTOR;
  let best = await applyAndMeasure(page, base, fitHeight, availableHeight);
  for (let attempt = 0; attempt < 12; attempt += 1) {
    const factor = (low + high) / 2;
    const candidate = await applyAndMeasure(
      page,
      {
        ...base,
        // Compact presets often leave more spare room than comfortable ones.
        // Spend that room on hierarchy and readable type, not on list rhythm:
        // long Chinese bullets wrap frequently, and expanded list line-height
        // makes the gap between bullet groups look far larger than intended.
        fontPt: Math.min(EXPANSION_LIMITS.fontPt, base.fontPt * (1 + (factor - 1) * 0.18)),
        lineHeight: Math.min(EXPANSION_LIMITS.lineHeight, base.lineHeight + (factor - 1) * 0.18),
        listFactor: Math.min(EXPANSION_LIMITS.listFactor, base.listFactor + (factor - 1) * 0.02),
        listLineHeight: Math.min(EXPANSION_LIMITS.listLineHeight, base.listLineHeight + (factor - 1) * 0.01),
        paragraphFactor: Math.min(EXPANSION_LIMITS.paragraphFactor, base.paragraphFactor + (factor - 1) * 0.6),
        sectionFactor: Math.min(EXPANSION_LIMITS.sectionFactor, base.sectionFactor + (factor - 1) * 1.2),
        subheadingFactor: Math.min(EXPANSION_LIMITS.subheadingFactor, base.subheadingFactor + (factor - 1) * 0.9),
      },
      fitHeight,
      availableHeight,
    );
    if (candidate.fits) {
      best = candidate;
      low = factor;
    } else {
      high = factor;
    }
  }
  return best;
}

/** Apply one layout candidate and return both fit status and useful diagnostics. */
async function applyAndMeasure(page, settings, fitHeight, availableHeight) {
  return page.evaluate(
    ({ availableHeight: pageHeight, fitHeight: heightLimit, settings: next, targetUsageFloor }) => {
      const root = document.documentElement;
      root.style.setProperty("--font-size", `${next.fontPt}pt`);
      root.style.setProperty("--line-height", String(next.lineHeight));
      root.style.setProperty("--list-factor", String(next.listFactor));
      root.style.setProperty("--list-line-height", String(next.listLineHeight));
      root.style.setProperty("--paragraph-factor", String(next.paragraphFactor));
      root.style.setProperty("--section-factor", String(next.sectionFactor));
      root.style.setProperty("--subheading-factor", String(next.subheadingFactor));

      const resume = document.querySelector("#resume");
      const contentHeight = resume.scrollHeight;
      const sections = [...document.querySelectorAll(".resume-section")].map((section) => ({
        height: Math.round(section.getBoundingClientRect().height * 10) / 10,
        title: section.querySelector(".section-title")?.textContent.trim() || "未命名章节",
      }));
      const suggestions = [...document.querySelectorAll(".resume-section p, .resume-section li")]
        .map((element) => ({
          height: Math.round(element.getBoundingClientRect().height * 10) / 10,
          text: element.textContent.trim().replace(/\s+/g, " ").slice(0, 90),
        }))
        .sort((a, b) => b.height - a.height)
        .slice(0, 3);

      return {
        availableHeight: Math.round(pageHeight * 10) / 10,
        contentHeight: Math.round(contentHeight * 10) / 10,
        fits: contentHeight <= heightLimit,
        isNearTarget: contentHeight / pageHeight >= targetUsageFloor,
        overflowPercent: Math.max(0, Math.round((contentHeight / pageHeight - 1) * 1000) / 10),
        printSafetyPx: Math.round((pageHeight - heightLimit) * 10) / 10,
        safeHeight: Math.round(heightLimit * 10) / 10,
        sections: sections.sort((a, b) => b.height - a.height),
        settings: next,
        suggestions,
      };
    },
    { availableHeight, fitHeight, settings, targetUsageFloor: TARGET_USAGE_FLOOR },
  );
}
