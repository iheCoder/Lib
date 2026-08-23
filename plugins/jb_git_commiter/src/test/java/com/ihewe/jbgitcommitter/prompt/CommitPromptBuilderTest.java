package com.ihewe.jbgitcommitter.prompt;

import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
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

    /** Empty custom text selects the immutable default and the visible default language. */
    @Test
    void usesDefaultPrompt() {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
        String prompt = CommitPromptBuilder.systemPrompt(settings);

        assertTrue(prompt.startsWith(AiCommitSettings.DEFAULT_PROMPT));
        assertTrue(prompt.contains("Write the commit message in English"));
        assertFalse(prompt.contains("within 0"));
    }

    /** A custom prompt replaces the default completely instead of being appended to it. */
    @Test
    void customPromptOverridesDefault() {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
        settings.customPrompt = "Write a detailed multi-line message with rationale.";

        String prompt = CommitPromptBuilder.systemPrompt(settings);

        assertTrue(prompt.startsWith(settings.customPrompt));
        assertFalse(prompt.contains(AiCommitSettings.DEFAULT_PROMPT));
    }

    /** A positive length is appended as an explicit prompt constraint; zero remains unlimited. */
    @Test
    void appendsConfiguredLanguageAndLength() {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
        settings.outputLanguage = "中文";
        settings.messageMaxCharacters = 80;

        String prompt = CommitPromptBuilder.systemPrompt(settings);

        assertTrue(prompt.contains("Write the commit message in 中文"));
        assertTrue(prompt.contains("within 80 Unicode characters"));
    }
}
