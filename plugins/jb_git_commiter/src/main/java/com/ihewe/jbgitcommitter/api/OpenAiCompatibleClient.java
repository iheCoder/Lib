package com.ihewe.jbgitcommitter.api;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParseException;
import com.google.gson.JsonParser;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import org.jetbrains.annotations.NotNull;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;

/** Minimal OpenAI Chat Completions client with no provider-specific SDK dependency. */
public final class OpenAiCompatibleClient {
    private static final double GENERATION_TEMPERATURE = 0.2;
    private static final int MAX_ERROR_CHARACTERS = 500;
    private final HttpClient httpClient;

    /** Uses the JDK client so proxy/TLS behavior follows the running IDE's JVM. */
    public OpenAiCompatibleClient() {
        httpClient = HttpClient.newBuilder()
                .followRedirects(HttpClient.Redirect.NORMAL)
                .build();
    }

    /** Sends selected context and returns the schema message without altering its formatting. */
    public String generate(
            @NotNull AiCommitSettings.SettingsState settings,
            @NotNull String apiKey,
            @NotNull String systemPrompt,
            @NotNull String userPrompt
    ) throws IOException, InterruptedException {
        String requestBody = buildRequestBody(settings, systemPrompt, userPrompt);
        HttpRequest request = createRequest(settings, apiKey, requestBody);
        HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() < 200 || response.statusCode() >= 300) {
            throw new IOException("Model API returned HTTP " + response.statusCode() + ": " + boundedError(response.body()));
        }
        return parseCommitMessage(response.body(), settings.structuredOutput);
    }

    /** Performs a minimal authenticated request without transmitting repository content. */
    public void testConnection(@NotNull AiCommitSettings.SettingsState settings, @NotNull String apiKey)
            throws IOException, InterruptedException {
        String requestBody = buildRequestBody(
                settings,
                "You test API connectivity. Return OK using the supplied output contract.",
                "Reply with OK."
        );
        HttpRequest request = createRequest(settings, apiKey, requestBody);
        HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() < 200 || response.statusCode() >= 300) {
            throw new IOException("Model API returned HTTP " + response.statusCode() + ": " + boundedError(response.body()));
        }
        parseCommitMessage(response.body(), settings.structuredOutput);
    }

    /** Builds Chat Completions JSON and optionally constrains the result with strict JSON Schema. */
    static String buildRequestBody(
            AiCommitSettings.SettingsState settings,
            String systemPrompt,
            String userPrompt
    ) {
        JsonObject root = new JsonObject();
        root.addProperty("model", settings.model);
        root.addProperty("temperature", GENERATION_TEMPERATURE);
        JsonArray messages = new JsonArray();
        messages.add(message("system", systemPrompt));
        messages.add(message("user", userPrompt));
        root.add("messages", messages);
        if (settings.structuredOutput) {
            root.add("response_format", responseFormat(settings.usesDefaultPrompt()));
        }
        return root.toString();
    }

    /** Accepts standard choices and a common output_text fallback used by compatible gateways. */
    static String parseCommitMessage(String responseBody) throws IOException {
        return parseCommitMessage(responseBody, false);
    }

    /** Extracts provider text and unwraps schema JSON without rewriting the model's message. */
    static String parseCommitMessage(String responseBody, boolean structuredOutput) throws IOException {
        try {
            JsonObject root = JsonParser.parseString(responseBody).getAsJsonObject();
            JsonArray choices = root.getAsJsonArray("choices");
            if (choices != null && !choices.isEmpty()) {
                JsonObject message = choices.get(0).getAsJsonObject().getAsJsonObject("message");
                JsonElement content = message == null ? null : message.get("content");
                if (content != null && content.isJsonPrimitive()) {
                    return validateMessage(extractMessage(content.getAsString(), structuredOutput));
                }
            }
            JsonElement outputText = root.get("output_text");
            if (outputText != null && outputText.isJsonPrimitive()) {
                return validateMessage(extractMessage(outputText.getAsString(), structuredOutput));
            }
            throw new IOException("Model API response did not contain choices[0].message.content");
        } catch (JsonParseException | IllegalStateException | IndexOutOfBoundsException | NullPointerException exception) {
            throw new IOException("Model API returned an unsupported JSON response", exception);
        }
    }

    /** Creates the strict, single-property schema understood by OpenAI-compatible providers. */
    private static JsonObject responseFormat(boolean enforceDefaultLimit) {
        JsonObject messageProperty = new JsonObject();
        messageProperty.addProperty("type", "string");
        messageProperty.addProperty("minLength", 1);
        if (enforceDefaultLimit) {
            messageProperty.addProperty("maxLength", AiCommitSettings.DEFAULT_MESSAGE_MAX_CHARACTERS);
        }
        JsonObject properties = new JsonObject();
        properties.add("message", messageProperty);
        JsonArray required = new JsonArray();
        required.add("message");
        JsonObject schema = new JsonObject();
        schema.addProperty("type", "object");
        schema.add("properties", properties);
        schema.add("required", required);
        schema.addProperty("additionalProperties", false);
        JsonObject jsonSchema = new JsonObject();
        jsonSchema.addProperty("name", "commit_message");
        jsonSchema.addProperty("strict", true);
        jsonSchema.add("schema", schema);
        JsonObject responseFormat = new JsonObject();
        responseFormat.addProperty("type", "json_schema");
        responseFormat.add("json_schema", jsonSchema);
        return responseFormat;
    }

    /** Reads the single schema field while keeping plain-text compatibility configurable. */
    private static String extractMessage(String content, boolean structuredOutput) throws IOException {
        if (!structuredOutput) {
            return content;
        }
        try {
            JsonElement message = JsonParser.parseString(content).getAsJsonObject().get("message");
            if (message == null || !message.isJsonPrimitive()) {
                throw new IOException("Structured response did not contain a string message field");
            }
            return message.getAsString();
        } catch (JsonParseException | IllegalStateException exception) {
            throw new IOException("Model API did not return the configured commit-message schema", exception);
        }
    }

    /** Creates one role/content message without hand-escaping source text. */
    private static JsonObject message(String role, String content) {
        JsonObject message = new JsonObject();
        message.addProperty("role", role);
        message.addProperty("content", content);
        return message;
    }

    /** Rejects empty output but otherwise preserves custom-prompt formatting and length verbatim. */
    private static String validateMessage(String raw) throws IOException {
        if (raw.isBlank()) {
            throw new IOException("Model API returned an empty commit message");
        }
        return raw;
    }

    /** Centralizes endpoint, timeout, authentication, and content headers for both API operations. */
    private static HttpRequest createRequest(
            AiCommitSettings.SettingsState settings,
            String apiKey,
            String requestBody
    ) {
        return HttpRequest.newBuilder(URI.create(settings.endpoint))
                .timeout(Duration.ofSeconds(settings.requestTimeoutSeconds))
                .header("Authorization", "Bearer " + apiKey)
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(requestBody))
                .build();
    }

    /** Prevents large or secret-bearing provider error pages from flooding IDE notifications. */
    private static String boundedError(String body) {
        String flattened = body == null ? "empty response" : body.replaceAll("\\s+", " ").trim();
        return flattened.length() <= MAX_ERROR_CHARACTERS
                ? flattened
                : flattened.substring(0, MAX_ERROR_CHARACTERS) + "...";
    }
}
