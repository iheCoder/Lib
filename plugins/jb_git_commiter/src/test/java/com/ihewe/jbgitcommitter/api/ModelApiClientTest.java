package com.ihewe.jbgitcommitter.api;

import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.SocketTimeoutException;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ModelApiClientTest {
    /** OpenAI keeps the Chat Completions message and structured-output contract. */
    @Test
    void buildsOpenAiRequest() {
        AiCommitSettings.SettingsState settings = settings(ModelProvider.OPENAI, "model-a");

        String json = ModelApiClient.buildRequestBody(settings, "system", "quote: \" and newline\n");

        assertTrue(json.contains("\\\""));
        assertTrue(json.contains("\\n"));
        assertTrue(json.contains("\"model\":\"model-a\""));
        assertTrue(json.contains("\"response_format\""));
        assertFalse(json.contains("maxLength"));
    }

    /** Anthropic uses a top-level system prompt, required max_tokens, and output_config.format. */
    @Test
    void buildsAnthropicMessagesRequest() {
        AiCommitSettings.SettingsState settings = settings(ModelProvider.ANTHROPIC, "claude-test");
        settings.messageMaxCharacters = 80;

        String json = ModelApiClient.buildRequestBody(settings, "system", "changes");

        assertTrue(json.contains("\"system\":\"system\""));
        assertTrue(json.contains("\"max_tokens\":1024"));
        assertTrue(json.contains("\"output_config\""));
        assertTrue(json.contains("\"maxLength\":80"));
        assertFalse(json.contains("\"role\":\"system\""));
    }

    /** DeepSeek deliberately shares the OpenAI-compatible Chat Completions wire shape. */
    @Test
    void buildsDeepSeekCompatibleRequest() {
        AiCommitSettings.SettingsState settings = settings(ModelProvider.DEEPSEEK, "deepseek-v4-pro");

        String json = ModelApiClient.buildRequestBody(settings, "system", "changes");

        assertTrue(json.contains("\"model\":\"deepseek-v4-pro\""));
        assertTrue(json.contains("\"role\":\"system\""));
        assertTrue(json.contains("\"response_format\""));
    }

    /** Plain OpenAI responses remain under custom-prompt control without post-processing. */
    @Test
    void preservesPlainOpenAiMessage() throws IOException {
        String response = "{\"choices\":[{\"message\":{\"content\":\"```text\\nfeat(api): add retries\\n```\"}}]}";

        assertEquals("```text\nfeat(api): add retries\n```", ModelApiClient.parseOpenAiResponse(response, false));
    }

    /** Anthropic text blocks are parsed and structured JSON is unwrapped identically. */
    @Test
    void parsesStructuredAnthropicMessage() throws IOException {
        AiCommitSettings.SettingsState settings = settings(ModelProvider.ANTHROPIC, "claude-test");
        String response = "{\"content\":[{\"type\":\"thinking\",\"thinking\":\"hidden\"},"
                + "{\"type\":\"text\",\"text\":\"{\\\"message\\\":\\\"fix proxy support\\\"}\"}]}";

        assertEquals("fix proxy support", ModelApiClient.parseProviderResponse(settings, response));
    }

    /** OpenAI transport sends Bearer authentication through the provider-aware client. */
    @Test
    void testsOpenAiConnection() throws Exception {
        CapturedRequest captured = new CapturedRequest();
        HttpServer server = startServer(exchange -> respondOpenAi(exchange, captured));
        try {
            AiCommitSettings.SettingsState settings = localSettings(server, ModelProvider.OPENAI);

            new ModelApiClient().testConnection(settings, "openai-secret");

            assertEquals("Bearer openai-secret", captured.authorization.get());
            assertTrue(captured.body.get().contains("Reply with OK"));
        } finally {
            server.stop(0);
        }
    }

    /** Anthropic transport sends vendor headers and parses content[] instead of choices[]. */
    @Test
    void testsAnthropicConnection() throws Exception {
        CapturedRequest captured = new CapturedRequest();
        HttpServer server = startServer(exchange -> respondAnthropic(exchange, captured));
        try {
            AiCommitSettings.SettingsState settings = localSettings(server, ModelProvider.ANTHROPIC);

            new ModelApiClient().testConnection(settings, "anthropic-secret");

            assertEquals("anthropic-secret", captured.apiKey.get());
            assertEquals("2023-06-01", captured.anthropicVersion.get());
            assertFalse(captured.body.get().contains("\"role\":\"system\""));
        } finally {
            server.stop(0);
        }
    }

    /** HTTP authentication failures remain distinct from connection failures. */
    @Test
    void explainsProviderAuthenticationFailure() throws Exception {
        HttpServer server = startServer(exchange -> respond(exchange, 401, "{\"error\":\"bad key\"}"));
        try {
            AiCommitSettings.SettingsState settings = localSettings(server, ModelProvider.DEEPSEEK);

            IOException error = assertThrows(IOException.class,
                    () -> new ModelApiClient().testConnection(settings, "wrong"));

            assertTrue(error.getMessage().contains("DeepSeek authentication failed (HTTP 401)"));
        } finally {
            server.stop(0);
        }
    }

    /** Timeout diagnostics direct users to the IDE proxy instead of blaming their API key. */
    @Test
    void explainsTimeoutWithProxyGuidance() {
        IOException error = ModelApiClient.explainNetworkFailure(new SocketTimeoutException(), 60);

        assertTrue(error.getMessage().contains("timed out after 60 seconds"));
        assertTrue(error.getMessage().contains("GoLand HTTP Proxy"));
    }

    /** Produces a minimal settings snapshot for protocol-focused tests. */
    private static AiCommitSettings.SettingsState settings(ModelProvider provider, String model) {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
        settings.provider = provider.id();
        settings.model = model;
        return settings;
    }

    /** Points a provider configuration at the same local endpoint while preserving its protocol. */
    private static AiCommitSettings.SettingsState localSettings(HttpServer server, ModelProvider provider) {
        AiCommitSettings.SettingsState settings = settings(provider, "test-model");
        settings.endpoint = "http://" + server.getAddress().getHostString() + ":" + server.getAddress().getPort()
                + "/model";
        return settings;
    }

    /** Starts a local component-test server without relying on an external model provider. */
    private static HttpServer startServer(ExchangeHandler handler) throws IOException {
        HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/model", exchange -> handler.handle(exchange));
        server.start();
        return server;
    }

    /** Captures OpenAI request evidence and returns its structured response shape. */
    private static void respondOpenAi(HttpExchange exchange, CapturedRequest captured) throws IOException {
        captured.authorization.set(exchange.getRequestHeaders().getFirst("Authorization"));
        captured.body.set(readBody(exchange));
        respond(exchange, 200, "{\"choices\":[{\"message\":{\"content\":\"{\\\"message\\\":\\\"OK\\\"}\"}}]}");
    }

    /** Captures Anthropic-specific headers and returns a text content block. */
    private static void respondAnthropic(HttpExchange exchange, CapturedRequest captured) throws IOException {
        captured.apiKey.set(exchange.getRequestHeaders().getFirst("x-api-key"));
        captured.anthropicVersion.set(exchange.getRequestHeaders().getFirst("anthropic-version"));
        captured.body.set(readBody(exchange));
        respond(exchange, 200, "{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"message\\\":\\\"OK\\\"}\"}]}");
    }

    /** Reads UTF-8 request JSON before the exchange is closed. */
    private static String readBody(HttpExchange exchange) throws IOException {
        return new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
    }

    /** Writes one deterministic JSON response for protocol tests. */
    private static void respond(HttpExchange exchange, int status, String body) throws IOException {
        byte[] response = body.getBytes(StandardCharsets.UTF_8);
        exchange.sendResponseHeaders(status, response.length);
        exchange.getResponseBody().write(response);
        exchange.close();
    }

    /** Mutable capture belongs to the test server thread and is observed after the synchronous call. */
    private static final class CapturedRequest {
        private final AtomicReference<String> authorization = new AtomicReference<>();
        private final AtomicReference<String> apiKey = new AtomicReference<>();
        private final AtomicReference<String> anthropicVersion = new AtomicReference<>();
        private final AtomicReference<String> body = new AtomicReference<>();
    }

    @FunctionalInterface
    private interface ExchangeHandler {
        void handle(HttpExchange exchange) throws IOException;
    }
}
