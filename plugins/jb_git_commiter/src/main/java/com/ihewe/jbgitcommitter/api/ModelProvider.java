package com.ihewe.jbgitcommitter.api;

import java.util.List;
import java.util.Locale;

/**
 * Defines the small provider catalog shown in Settings and the wire protocol behind each entry.
 * Endpoint and model values remain editable because provider catalogs evolve faster than plugins.
 */
public enum ModelProvider {
    OPENAI(
            "openai",
            "OpenAI",
            "https://api.openai.com/v1/chat/completions",
            List.of("gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.6", "gpt-5-mini", "gpt-4.1-mini"),
            ApiProtocol.OPENAI_CHAT_COMPLETIONS
    ),
    ANTHROPIC(
            "anthropic",
            "Anthropic",
            "https://api.anthropic.com/v1/messages",
            List.of("claude-sonnet-5", "claude-opus-5", "claude-fable-5", "claude-haiku-4-5-20251001"),
            ApiProtocol.ANTHROPIC_MESSAGES
    ),
    DEEPSEEK(
            "deepseek",
            "DeepSeek",
            "https://api.deepseek.com/chat/completions",
            List.of("deepseek-v4-pro", "deepseek-v4-flash"),
            ApiProtocol.OPENAI_CHAT_COMPLETIONS
    );

    private final String id;
    private final String displayName;
    private final String defaultEndpoint;
    private final List<String> models;
    private final ApiProtocol protocol;

    ModelProvider(
            String id,
            String displayName,
            String defaultEndpoint,
            List<String> models,
            ApiProtocol protocol
    ) {
        this.id = id;
        this.displayName = displayName;
        this.defaultEndpoint = defaultEndpoint;
        this.models = List.copyOf(models);
        this.protocol = protocol;
    }

    /** Resolves persisted values conservatively so unknown future IDs do not break Settings. */
    public static ModelProvider fromId(String id) {
        if (id != null) {
            for (ModelProvider provider : values()) {
                if (provider.id.equalsIgnoreCase(id.trim())) {
                    return provider;
                }
            }
        }
        return OPENAI;
    }

    /** Migrates pre-provider settings by recognizing official endpoint host names. */
    public static ModelProvider inferFromEndpoint(String endpoint) {
        String normalized = endpoint == null ? "" : endpoint.toLowerCase(Locale.ROOT);
        if (normalized.contains("anthropic.com")) {
            return ANTHROPIC;
        }
        return normalized.contains("deepseek.com") ? DEEPSEEK : OPENAI;
    }

    public String id() {
        return id;
    }

    public String displayName() {
        return displayName;
    }

    public String defaultEndpoint() {
        return defaultEndpoint;
    }

    public String defaultModel() {
        return models.getFirst();
    }

    public List<String> models() {
        return models;
    }

    public ApiProtocol protocol() {
        return protocol;
    }

    /** Swing combo boxes use this value while persistence continues to use the stable lowercase ID. */
    @Override
    public String toString() {
        return displayName;
    }

    /** Only Anthropic diverges from the OpenAI-compatible request and response contract. */
    public enum ApiProtocol {
        OPENAI_CHAT_COMPLETIONS,
        ANTHROPIC_MESSAGES
    }
}
