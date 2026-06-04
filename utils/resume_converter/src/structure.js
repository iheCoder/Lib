/**
 * Convert a flat Markdown DOM into explicit header and section regions. The
 * function is serialized into the generated page, so it intentionally depends
 * only on browser globals and the injected section-name expression.
 */
export function enhanceResumeStructure() {
  const resume = document.querySelector("#resume");
  const nodes = [...resume.childNodes].filter(
    (node) => node.nodeType !== Node.TEXT_NODE || node.textContent.trim(),
  );
  const headings = nodes.filter((node) => /^H[1-6]$/.test(node.nodeName));
  const nameHeading = headings[0];
  if (nameHeading) {
    nameHeading.classList.add("resume-name");
  }

  const mainHeadings = headings.filter((heading, index) => {
    if (index === 0) return false;
    return heading.tagName === "H1" || window.MD_RESUME_MAIN_SECTION.test(heading.textContent.trim());
  });
  if (mainHeadings.length === 0) {
    headings.slice(1).forEach((heading) => mainHeadings.push(heading));
  }

  const firstSectionIndex = mainHeadings.length ? nodes.indexOf(mainHeadings[0]) : nodes.length;
  const header = document.createElement("header");
  header.className = "resume-header";
  nodes.slice(0, firstSectionIndex).forEach((node) => header.append(node));
  resume.prepend(header);

  mainHeadings.forEach((heading) => {
    if (!heading.isConnected) return;
    const section = document.createElement("section");
    section.className = "resume-section";
    heading.classList.add("section-title");
    heading.before(section);
    section.append(heading);
    while (section.nextSibling && !mainHeadings.includes(section.nextSibling)) {
      section.append(section.nextSibling);
    }
  });
}
