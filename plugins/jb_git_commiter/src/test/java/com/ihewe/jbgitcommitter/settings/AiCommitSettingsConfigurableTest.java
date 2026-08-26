package com.ihewe.jbgitcommitter.settings;

import com.ihewe.jbgitcommitter.api.ModelProvider;
import org.junit.jupiter.api.Test;

import javax.swing.JComponent;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class AiCommitSettingsConfigurableTest {
    /** Reset must survive combo-box listeners without replacing a persisted custom URL or model. */
    @Test
    void preservesCustomProviderValuesWhenOpeningSettings() {
        AiCommitSettings settings = new AiCommitSettings();
        AiCommitSettings.SettingsState custom = new AiCommitSettings.SettingsState();
        custom.provider = ModelProvider.ANTHROPIC.id();
        custom.endpoint = "https://gateway.example.test/anthropic/messages";
        custom.model = "claude-private-deployment";
        settings.loadState(custom);
        AiCommitSettingsConfigurable configurable = new AiCommitSettingsConfigurable(settings);
        try {
            JComponent component = configurable.createComponent();

            assertTrue(component != null);
            assertFalse(configurable.isModified());
        } finally {
            configurable.disposeUIResources();
        }
    }

    /** Old endpoint-only state gains a provider without losing its custom model or URL. */
    @Test
    void migratesLegacyProviderState() {
        AiCommitSettings.SettingsState state = new AiCommitSettings.SettingsState();
        state.provider = null;
        state.endpoint = "https://api.deepseek.com/chat/completions";
        state.model = "custom-deepseek-model";
        AiCommitSettings settings = new AiCommitSettings();

        settings.loadState(state);

        assertEquals(ModelProvider.DEEPSEEK, settings.getState().provider());
        assertEquals("custom-deepseek-model", settings.getState().model);
    }

    /** Upward wheel input escapes an inner editor only after that editor reaches its top. */
    @Test
    void forwardsUpwardWheelAtTop() {
        assertTrue(AiCommitSettingsConfigurable.shouldForwardWheel(-1, 0, 20, 0, 100));
        assertFalse(AiCommitSettingsConfigurable.shouldForwardWheel(-1, 10, 20, 0, 100));
    }

    /** Downward wheel input escapes an inner editor only after that editor reaches its bottom. */
    @Test
    void forwardsDownwardWheelAtBottom() {
        assertTrue(AiCommitSettingsConfigurable.shouldForwardWheel(1, 80, 20, 0, 100));
        assertFalse(AiCommitSettingsConfigurable.shouldForwardWheel(1, 70, 20, 0, 100));
    }
}
