package com.ihewe.jbgitcommitter.api;

import org.junit.jupiter.api.Test;

import java.io.IOException;

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

    /** Verifies incompatible gateways fail clearly instead of placing an empty message in the editor. */
    @Test
    void rejectsUnsupportedResponse() {
        IOException error = assertThrows(IOException.class,
                () -> OpenAiCompatibleClient.parseCommitMessage("{\"result\":\"missing\"}"));

        assertTrue(error.getMessage().contains("did not contain"));
    }
}
