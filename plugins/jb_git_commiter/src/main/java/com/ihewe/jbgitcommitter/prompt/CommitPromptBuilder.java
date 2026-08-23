package com.ihewe.jbgitcommitter.prompt;

import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;

import java.util.List;

/** Builds a bounded, explicit prompt from selected before/after snapshots. */
public final class CommitPromptBuilder {
    private static final String TRUNCATION_MARKER = "\n...[context truncated by AI Git Committer]";

    private CommitPromptBuilder() {
    }

    /** Uses the immutable default unless the user supplies a complete replacement prompt. */
    public static String systemPrompt(AiCommitSettings.SettingsState settings) {
        return settings.usesDefaultPrompt()
                ? AiCommitSettings.DEFAULT_PROMPT
                : settings.customPrompt.trim();
    }

    /** Adds files in selection order and enforces one global character budget before transmission. */
    public static String userPrompt(List<FileChangeSnapshot> changes, int maxCharacters) {
        StringBuilder prompt = new StringBuilder("Generate one commit message for these selected changes:\n");
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
