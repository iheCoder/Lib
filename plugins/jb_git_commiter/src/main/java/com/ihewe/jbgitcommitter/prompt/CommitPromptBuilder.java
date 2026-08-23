package com.ihewe.jbgitcommitter.prompt;

import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.ihewe.jbgitcommitter.model.LimitedDependencyContext;
import com.ihewe.jbgitcommitter.model.LimitedDependencyContext.SymbolRelation;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;

import java.util.List;

/** Builds a bounded, explicit prompt from selected before/after snapshots. */
public final class CommitPromptBuilder {
    private static final String TRUNCATION_MARKER = "\n...[context truncated by AI Git Committer]";

    private CommitPromptBuilder() {
    }

    /** Selects default/custom intent, then appends only the explicit UI output constraints. */
    public static String systemPrompt(AiCommitSettings.SettingsState settings) {
        String generationPrompt = settings.usesDefaultPrompt()
                ? AiCommitSettings.DEFAULT_PROMPT
                : settings.customPrompt.trim();
        StringBuilder effectivePrompt = new StringBuilder(generationPrompt)
                .append("\n\nOutput constraints:")
                .append("\n- Write the commit message in ").append(settings.outputLanguage).append('.');
        if (settings.messageMaxCharacters > 0) {
            effectivePrompt.append("\n- Keep the complete commit message within ")
                    .append(settings.messageMaxCharacters)
                    .append(" Unicode characters.");
        }
        return effectivePrompt.toString();
    }

    /** Adds files in selection order and enforces one global character budget before transmission. */
    public static String userPrompt(
            List<FileChangeSnapshot> changes,
            LimitedDependencyContext dependencyContext,
            int maxCharacters
    ) {
        StringBuilder prompt = new StringBuilder("Generate one commit message for these selected changes:\n")
                .append(dependencyBlock(dependencyContext));
        for (FileChangeSnapshot change : changes) {
            appendWithinBudget(prompt, fileBlock(change), maxCharacters);
            if (prompt.length() >= maxCharacters) {
                break;
            }
        }
        if (prompt.length() >= maxCharacters && !prompt.toString().endsWith(TRUNCATION_MARKER)) {
            int keep = Math.max(0, maxCharacters - TRUNCATION_MARKER.length());
            prompt.setLength(Math.min(prompt.length(), keep));
            prompt.append(TRUNCATION_MARKER);
        }
        return prompt.toString();
    }

    /** Renders bounded PSI evidence ahead of raw revisions so intent extraction sees structure first. */
    private static String dependencyBlock(LimitedDependencyContext context) {
        StringBuilder block = new StringBuilder("\n## Limited Dependency Relations\n")
                .append("Analysis: ").append(context.analysisNote()).append('\n')
                .append("Relevant project paths:\n");
        appendList(block, context.relevantPaths());
        block.append("Changed symbols:\n");
        if (context.symbols().isEmpty()) {
            block.append("- [none detected]\n");
        } else {
            context.symbols().forEach(symbol -> appendSymbol(block, symbol));
        }
        return block.toString();
    }

    /** Keeps each changed symbol and its three relation kinds visually scannable for the model. */
    private static void appendSymbol(StringBuilder block, SymbolRelation symbol) {
        block.append("- ").append(symbol.changeKind()).append(' ').append(symbol.symbol())
                .append(" (package: ").append(symbol.packageName())
                .append(", file: ").append(symbol.filePath()).append(")\n")
                .append("  dependencies: ").append(inlineList(symbol.dependencies())).append('\n')
                .append("  dependents: ").append(inlineList(symbol.dependents())).append('\n')
                .append("  related tests: ").append(inlineList(symbol.relatedTests())).append('\n');
    }

    /** Uses a stable marker for empty relation kinds instead of making the model infer absence. */
    private static String inlineList(List<String> values) {
        return values.isEmpty() ? "[none found]" : String.join("; ", values);
    }

    /** Renders relevant paths as a sparse project slice rather than transmitting the whole tree. */
    private static void appendList(StringBuilder destination, List<String> values) {
        if (values.isEmpty()) {
            destination.append("- [none]\n");
            return;
        }
        values.forEach(value -> destination.append("- ").append(value).append('\n'));
    }

    /** Formats null revision content explicitly so deletions and binary files remain distinguishable. */
    private static String fileBlock(FileChangeSnapshot change) {
        return new StringBuilder()
                .append("\n=== ").append(change.path()).append(" [").append(change.changeType()).append("] ===\n")
                .append("--- BEFORE ---\n").append(contentOrMarker(change.before()))
                .append("\n--- AFTER ---\n").append(contentOrMarker(change.after()))
                .append('\n')
                .toString();
    }

    /** Treats null as unavailable while preserving an intentionally empty text file. */
    private static String contentOrMarker(String content) {
        return content == null ? "[not available: absent or binary]" : content;
    }

    /** Appends only the remaining prefix; the caller adds a single consistent truncation marker. */
    private static void appendWithinBudget(StringBuilder destination, String value, int maxCharacters) {
        int remaining = maxCharacters - destination.length();
        if (remaining <= 0) {
            return;
        }
        destination.append(value, 0, Math.min(value.length(), remaining));
    }
}
