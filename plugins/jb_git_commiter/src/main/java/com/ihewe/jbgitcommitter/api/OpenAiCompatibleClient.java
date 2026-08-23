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

    /** Sends the selected code context and returns a normalized plain-text commit message. */
    public String generate(
            @NotNull AiCommitSettings.SettingsState settings,
            @NotNull String apiKey,
            @NotNull String systemPrompt,
            @NotNull String userPrompt
    ) throws IOException, InterruptedException {
        String requestBody = buildRequestBody(settings.model, systemPrompt, userPrompt);
        HttpRequest request = HttpRequest.newBuilder(URI.create(settings.endpoint))
                .timeout(Duration.ofSeconds(settings.requestTimeoutSeconds))
                .header("Authorization", "Bearer " + apiKey)
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(requestBody))
                .build();
        HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() < 200 || response.statusCode() >= 300) {
            throw new IOException("Model API returned HTTP " + response.statusCode() + ": " + boundedError(response.body()));
        }
        return parseCommitMessage(response.body());
    }

    /** Performs a minimal authenticated request without transmitting repository content. */
    public void testConnection(@NotNull AiCommitSettings.SettingsState settings, @NotNull String apiKey)
            throws IOException, InterruptedException {
        generate(
                settings,
                apiKey,
                "You are an API connectivity tester. Return only OK.",
                "Reply with OK."
        );
    }

    /** Builds the broadly supported Chat Completions request shape. */
    static String buildRequestBody(String model, String systemPrompt, String userPrompt) {
        JsonObject root = new JsonObject();
        root.addProperty("model", model);
        root.addProperty("temperature", GENERATION_TEMPERATURE);
        JsonArray messages = new JsonArray();
        messages.add(message("system", systemPrompt));
        messages.add(message("user", userPrompt));
        root.add("messages", messages);
        return root.toString();
    }

    /** Accepts standard choices and a common output_text fallback used by compatible gateways. */
    static String parseCommitMessage(String responseBody) throws IOException {
        try {
            JsonObject root = JsonParser.parseString(responseBody).getAsJsonObject();
            JsonArray choices = root.getAsJsonArray("choices");
            if (choices != null && !choices.isEmpty()) {
                JsonObject message = choices.get(0).getAsJsonObject().getAsJsonObject("message");
                JsonElement content = message == null ? null : message.get("content");
                if (content != null && content.isJsonPrimitive()) {
                    return normalize(content.getAsString());
                }
            }
            JsonElement outputText = root.get("output_text");
            if (outputText != null && outputText.isJsonPrimitive()) {
                return normalize(outputText.getAsString());
            }
            throw new IOException("Model API response did not contain choices[0].message.content");
        } catch (JsonParseException | IllegalStateException | IndexOutOfBoundsException | NullPointerException exception) {
            throw new IOException("Model API returned an unsupported JSON response", exception);
        }
    }

    /** Creates one role/content message without hand-escaping source text. */
    private static JsonObject message(String role, String content) {
        JsonObject message = new JsonObject();
        message.addProperty("role", role);
        message.addProperty("content", content);
        return message;
    }

    /** Removes presentation fences while preserving intentional multi-line commit bodies. */
    private static String normalize(String raw) throws IOException {
        String message = raw.trim();
        if (message.startsWith("```")) {
            int firstLineEnd = message.indexOf('\n');
            int closingFence = message.lastIndexOf("```");
            if (firstLineEnd >= 0 && closingFence > firstLineEnd) {
                message = message.substring(firstLineEnd + 1, closingFence).trim();
            }
        }
        if (message.isBlank()) {
            throw new IOException("Model API returned an empty commit message");
        }
        return message;
    }

    /** Prevents large or secret-bearing provider error pages from flooding IDE notifications. */
    private static String boundedError(String body) {
        String flattened = body == null ? "empty response" : body.replaceAll("\\s+", " ").trim();
        return flattened.length() <= MAX_ERROR_CHARACTERS
                ? flattened
                : flattened.substring(0, MAX_ERROR_CHARACTERS) + "...";
    }
}
