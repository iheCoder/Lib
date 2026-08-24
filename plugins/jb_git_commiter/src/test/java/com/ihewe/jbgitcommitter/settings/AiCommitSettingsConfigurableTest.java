package com.ihewe.jbgitcommitter.settings;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class AiCommitSettingsConfigurableTest {
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
