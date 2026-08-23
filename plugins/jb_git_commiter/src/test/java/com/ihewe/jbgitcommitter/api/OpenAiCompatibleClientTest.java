package com.ihewe.jbgitcommitter.api;

import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class OpenAiCompatibleClientTest {
    /** Verifies request JSON safely escapes arbitrary source text. */
    @Test
    void buildsValidEscapedRequest() {
        String json = OpenAiCompatibleClient.buildRequestBody("model-a", "system", "quote: \" and newline\n");

        assertTrue(json.contains("\\\""));
        assertTrue(json.contains("\\n"));
        assertTrue(json.contains("\"model\":\"model-a\""));
    }

    /** Verifies the standard Chat Completions response and Markdown-fence cleanup. */
    @Test
    void parsesCommitMessage() throws IOException {
        String response = "{\"choices\":[{\"message\":{\"content\":\"```text\\nfeat(api): add retries\\n```\"}}]}";

        assertEquals("feat(api): add retries", OpenAiCompatibleClient.parseCommitMessage(response));
    }

    /** Verifies compatible gateways may return a top-level output_text value. */
    @Test
    void parsesOutputTextFallback() throws IOException {
        assertEquals("fix: handle timeout", OpenAiCompatibleClient.parseCommitMessage(
                "{\"output_text\":\"fix: handle timeout\"}"
        ));
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
            byte[] response = "{\"choices\":[{\"message\":{\"content\":\"OK\"}}]}"
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
