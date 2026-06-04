const A4_HEIGHT_MM = 297;
const PX_PER_MM = 96 / 25.4;
const PRINT_SAFETY_PX = 6;

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
    lineHeight: 1.44,
    sectionFactor: 1,
    spaceFactor: 1,
  };
  let measurement = await applyAndMeasure(page, comfortable, fitHeight, availableHeight);

  if (measurement.fits) {
    const expanded = await findLargestFit(page, comfortable, fitHeight, availableHeight);
    return { ...expanded, status: "fit" };
  }

  // Compression happens in perceptual order: remove excess whitespace first,
  // then tighten line height, and only then reduce type toward its hard floor.
  const compact = { ...comfortable, lineHeight: 1.36, sectionFactor: 0.72, spaceFactor: 0.72 };
  measurement = await applyAndMeasure(page, compact, fitHeight, availableHeight);
  if (measurement.fits) {
    return { ...measurement, status: "fit" };
  }

  const best = await findBestFontFit(page, compact, fitHeight, availableHeight, options);
  if (best) {
    return { ...best, status: "fit" };
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
  let high = 1.35;
  let best = await applyAndMeasure(page, base, fitHeight, availableHeight);
  for (let attempt = 0; attempt < 10; attempt += 1) {
    const factor = (low + high) / 2;
    const candidate = await applyAndMeasure(
      page,
      {
        ...base,
        lineHeight: 1.44 + (factor - 1) * 0.24,
        sectionFactor: factor,
        spaceFactor: factor,
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
    ({ availableHeight: pageHeight, fitHeight: heightLimit, settings: next }) => {
      const root = document.documentElement;
      root.style.setProperty("--font-size", `${next.fontPt}pt`);
      root.style.setProperty("--line-height", String(next.lineHeight));
      root.style.setProperty("--section-factor", String(next.sectionFactor));
      root.style.setProperty("--space-factor", String(next.spaceFactor));

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
        overflowPercent: Math.max(0, Math.round((contentHeight / pageHeight - 1) * 1000) / 10),
        sections: sections.sort((a, b) => b.height - a.height),
        settings: next,
        suggestions,
      };
    },
    { availableHeight, fitHeight, settings },
  );
}
