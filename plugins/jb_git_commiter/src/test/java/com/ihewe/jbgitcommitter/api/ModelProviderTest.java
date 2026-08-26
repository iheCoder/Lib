package com.ihewe.jbgitcommitter.api;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ModelProviderTest {
    /** Official presets remain usable while exposing more than one model choice per provider. */
    @Test
    void exposesProviderDefaultsAndModels() {
        assertEquals("https://api.openai.com/v1/chat/completions", ModelProvider.OPENAI.defaultEndpoint());
        assertEquals("https://api.anthropic.com/v1/messages", ModelProvider.ANTHROPIC.defaultEndpoint());
        assertEquals("https://api.deepseek.com/chat/completions", ModelProvider.DEEPSEEK.defaultEndpoint());
        assertTrue(ModelProvider.OPENAI.models().size() > 1);
        assertTrue(ModelProvider.ANTHROPIC.models().size() > 1);
        assertTrue(ModelProvider.DEEPSEEK.models().contains("deepseek-v4-pro"));
    }

    /** Pre-v0.6 endpoint-only settings migrate without changing their provider semantics. */
    @Test
    void infersLegacyProviderFromEndpoint() {
        assertEquals(ModelProvider.ANTHROPIC,
                ModelProvider.inferFromEndpoint("https://api.anthropic.com/v1/messages"));
        assertEquals(ModelProvider.DEEPSEEK,
                ModelProvider.inferFromEndpoint("https://api.deepseek.com/chat/completions"));
        assertEquals(ModelProvider.OPENAI,
                ModelProvider.inferFromEndpoint("https://gateway.example.com/v1/chat/completions"));
    }
}
