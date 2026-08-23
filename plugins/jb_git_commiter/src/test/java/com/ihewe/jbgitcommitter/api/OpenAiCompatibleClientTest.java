package com.ihewe.jbgitcommitter.api;

import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class OpenAiCompatibleClientTest {
    /** Verifies request JSON safely escapes arbitrary source text. */
    @Test
    void buildsValidEscapedRequest() {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
        settings.model = "model-a";
        String json = OpenAiCompatibleClient.buildRequestBody(settings, "system", "quote: \" and newline\n");

        assertTrue(json.contains("\\\""));
        assertTrue(json.contains("\\n"));
        assertTrue(json.contains("\"model\":\"model-a\""));
        assertTrue(json.contains("\"type\":\"json_schema\""));
        assertTrue(json.contains("\"maxLength\":50"));
    }

    /** Plain responses are preserved rather than forced into one line or stripped of formatting. */
    @Test
    void preservesPlainCommitMessage() throws IOException {
        String response = "{\"choices\":[{\"message\":{\"content\":\"```text\\nfeat(api): add retries\\n```\"}}]}";

        assertEquals("```text\nfeat(api): add retries\n```", OpenAiCompatibleClient.parseCommitMessage(response));
    }

    /** Verifies compatible gateways may return a top-level output_text value. */
    @Test
    void parsesOutputTextFallback() throws IOException {
        assertEquals("fix: handle timeout", OpenAiCompatibleClient.parseCommitMessage(
                "{\"output_text\":\"fix: handle timeout\"}"
        ));
    }

    /** Leading/trailing whitespace and a multi-line body remain under custom-prompt control. */
    @Test
    void preservesMultiLineFormattingVerbatim() throws IOException {
        String expected = "  title\n\nbody  ";

        assertEquals(expected, OpenAiCompatibleClient.parseCommitMessage(
                "{\"output_text\":\"  title\\n\\nbody  \"}"
        ));
    }

    /** Structured output is unwrapped without truncating a custom prompt's result. */
    @Test
    void parsesStructuredMessageWithoutPostProcessing() throws IOException {
        String content = "{\\\"message\\\":\\\"" + "修".repeat(55) + "\\\"}";
        String response = "{\"choices\":[{\"message\":{\"content\":\"" + content + "\"}}]}";

        String message = OpenAiCompatibleClient.parseCommitMessage(response, true);

        assertEquals(55, message.codePointCount(0, message.length()));
    }

    /** Custom prompt schema keeps the message shape but removes the default 50-character cap. */
    @Test
    void customPromptSchemaHasNoMaximumLength() {
        AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
        settings.customPrompt = "Write a detailed commit message.";

        String json = OpenAiCompatibleClient.buildRequestBody(settings, settings.customPrompt, "changes");

        assertTrue(json.contains("\"type\":\"json_schema\""));
        assertFalse(json.contains("maxLength"));
    }

    /** Verifies Test API performs an authenticated minimal request against the unsaved endpoint/model. */
    @Test
    void testsConnectionAgainstCompatibleEndpoint() throws Exception {
        AtomicReference<String> authorization = new AtomicReference<>();
        AtomicReference<String> requestBody = new AtomicReference<>();
        HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/chat/completions", exchange -> {
            authorization.set(exchange.getRequestHeaders().getFirst("Authorization"));
            requestBody.set(new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8));
            byte[] response = "{\"choices\":[{\"message\":{\"content\":\"{\\\"message\\\":\\\"OK\\\"}\"}}]}"
                    .getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(200, response.length);
            exchange.getResponseBody().write(response);
            exchange.close();
        });
        server.start();
        try {
            AiCommitSettings.SettingsState settings = new AiCommitSettings.SettingsState();
            settings.endpoint = "http://" + server.getAddress().getHostString() + ":" + server.getAddress().getPort()
                    + "/chat/completions";
            settings.model = "test-model";

            new OpenAiCompatibleClient().testConnection(settings, "test-secret");

            assertEquals("Bearer test-secret", authorization.get());
            assertTrue(requestBody.get().contains("\"model\":\"test-model\""));
            assertTrue(requestBody.get().contains("Reply with OK"));
            assertTrue(requestBody.get().contains("json_schema"));
        } finally {
            server.stop(0);
        }
    }

    /** Verifies incompatible gateways fail clearly instead of placing an empty message in the editor. */
    @Test
    void rejectsUnsupportedResponse() {
        IOException error = assertThrows(IOException.class,
                () -> OpenAiCompatibleClient.parseCommitMessage("{\"result\":\"missing\"}"));

        assertTrue(error.getMessage().contains("did not contain"));
    }
}
