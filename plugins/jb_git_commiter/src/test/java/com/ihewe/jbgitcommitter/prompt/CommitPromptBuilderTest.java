package com.ihewe.jbgitcommitter.prompt;

import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CommitPromptBuilderTest {
    /** Verifies path, status, and both revisions remain explicit to the model. */
    @Test
    void formatsSelectedChange() {
        String prompt = CommitPromptBuilder.userPrompt(
                List.of(new FileChangeSnapshot("src/App.java", "MODIFICATION", "old", "new")),
                10_000
        );

        assertTrue(prompt.contains("src/App.java [MODIFICATION]"));
        assertTrue(prompt.contains("--- BEFORE ---\nold"));
        assertTrue(prompt.contains("--- AFTER ---\nnew"));
    }

    /** Verifies the outbound context never exceeds the configured privacy/cost boundary. */
    @Test
    void truncatesAtGlobalBudget() {
        int budget = 180;
        String prompt = CommitPromptBuilder.userPrompt(
                List.of(new FileChangeSnapshot("large.txt", "MODIFICATION", "a".repeat(500), "b".repeat(500))),
                budget
        );

        assertEquals(budget, prompt.length());
        assertTrue(prompt.endsWith("...[context truncated by AI Git Committer]"));
    }

    /** Empty custom text selects the exact immutable default prompt. */
    @Test
    void usesDefaultPrompt() {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();

        assertEquals(AiCommitSettings.DEFAULT_PROMPT, CommitPromptBuilder.systemPrompt(settings));
    }

    /** A custom prompt replaces the default completely instead of being appended to it. */
    @Test
    void customPromptOverridesDefault() {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
        settings.customPrompt = "Write a detailed multi-line message with rationale.";

        assertEquals(settings.customPrompt, CommitPromptBuilder.systemPrompt(settings));
    }
}
