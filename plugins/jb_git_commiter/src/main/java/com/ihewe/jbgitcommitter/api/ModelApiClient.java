package com.ihewe.jbgitcommitter.api;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParseException;
import com.google.gson.JsonParser;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import com.intellij.util.net.HttpConnectionUtils;
import org.jetbrains.annotations.NotNull;

import javax.net.ssl.SSLException;
import java.io.IOException;
import java.io.InputStream;
import java.net.ConnectException;
import java.net.HttpURLConnection;
import java.net.SocketTimeoutException;
import java.net.UnknownHostException;
import java.nio.charset.StandardCharsets;

/**
 * Sends commit prompts through provider-specific protocols while delegating proxy selection to the
 * IntelliJ Platform. The client intentionally has no vendor SDK dependency so the plugin stays small.
 */
public final class ModelApiClient {
    private static final int ANTHROPIC_MAX_OUTPUT_TOKENS = 1_024;
    private static final int MAX_ERROR_CHARACTERS = 500;
    private static final String ANTHROPIC_VERSION = "2023-06-01";

    /** Sends selected context and returns the model message without changing its formatting. */
    public String generate(
            @NotNull AiCommitSettings.SettingsState settings,
            @NotNull String apiKey,
            @NotNull String systemPrompt,
            @NotNull String userPrompt
    ) throws IOException, InterruptedException {
        String requestBody = buildRequestBody(settings, systemPrompt, userPrompt);
        ProviderResponse response = execute(settings, apiKey, requestBody);
        return parseProviderResponse(settings, response.body());
    }

    /** Performs a minimal authenticated request without transmitting repository content. */
    public void testConnection(@NotNull AiCommitSettings.SettingsState settings, @NotNull String apiKey)
            throws IOException, InterruptedException {
        String requestBody = buildRequestBody(
                settings,
                "You test API connectivity. Return OK using the supplied output contract.",
                "Reply with OK."
        );
        ProviderResponse response = execute(settings, apiKey, requestBody);
        parseProviderResponse(settings, response.body());
    }

    /** Selects the wire contract from the persisted provider rather than guessing from the URL. */
    static String buildRequestBody(
            AiCommitSettings.SettingsState settings,
            String systemPrompt,
            String userPrompt
    ) {
        return switch (settings.provider().protocol()) {
            case OPENAI_CHAT_COMPLETIONS -> openAiRequest(settings, systemPrompt, userPrompt);
            case ANTHROPIC_MESSAGES -> anthropicRequest(settings, systemPrompt, userPrompt);
        };
    }

    /** Builds the shared OpenAI/DeepSeek Chat Completions request. */
    private static String openAiRequest(
            AiCommitSettings.SettingsState settings,
            String systemPrompt,
            String userPrompt
    ) {
        JsonObject root = new JsonObject();
        root.addProperty("model", settings.model);
        JsonArray messages = new JsonArray();
        messages.add(message("system", systemPrompt));
        messages.add(message("user", userPrompt));
        root.add("messages", messages);
        if (settings.structuredOutput) {
            root.add("response_format", openAiResponseFormat(settings.messageMaxCharacters));
        }
        return root.toString();
    }

    /** Uses Anthropic's top-level system field and required max_tokens value. */
    private static String anthropicRequest(
            AiCommitSettings.SettingsState settings,
            String systemPrompt,
            String userPrompt
    ) {
        JsonObject root = new JsonObject();
        root.addProperty("model", settings.model);
        root.addProperty("max_tokens", ANTHROPIC_MAX_OUTPUT_TOKENS);
        root.addProperty("system", systemPrompt);
        JsonArray messages = new JsonArray();
        messages.add(message("user", userPrompt));
        root.add("messages", messages);
        if (settings.structuredOutput) {
            root.add("output_config", anthropicOutputConfig(settings.messageMaxCharacters));
        }
        return root.toString();
    }

    /** Executes one POST through GoLand's proxy-aware connection factory. */
    private static ProviderResponse execute(
            AiCommitSettings.SettingsState settings,
            String apiKey,
            String requestBody
    ) throws IOException {
        HttpURLConnection connection = null;
        try {
            connection = createConnection(settings, apiKey);
            connection.connect();
            connection.getOutputStream().write(requestBody.getBytes(StandardCharsets.UTF_8));
            int statusCode = connection.getResponseCode();
            String responseBody = readResponseBody(connection, statusCode);
            ensureSuccess(settings.provider(), statusCode, responseBody);
            return new ProviderResponse(statusCode, responseBody);
        } catch (IOException exception) {
            throw explainNetworkFailure(exception, settings.requestTimeoutSeconds);
        } finally {
            if (connection != null) {
                connection.disconnect();
            }
        }
    }

    /** Applies timeouts, redirects, and provider authentication before the connection is opened. */
    private static HttpURLConnection createConnection(
            AiCommitSettings.SettingsState settings,
            String apiKey
    ) throws IOException {
        HttpURLConnection connection = HttpConnectionUtils.openHttpConnection(settings.endpoint);
        int timeoutMillis = Math.multiplyExact(settings.requestTimeoutSeconds, 1_000);
        connection.setConnectTimeout(timeoutMillis);
        connection.setReadTimeout(timeoutMillis);
        connection.setInstanceFollowRedirects(true);
        connection.setRequestMethod("POST");
        connection.setDoOutput(true);
        connection.setRequestProperty("Content-Type", "application/json");
        connection.setRequestProperty("Accept", "application/json");
        applyAuthentication(connection, settings.provider(), apiKey);
        return connection;
    }

    /** Anthropic uses dedicated headers; OpenAI-compatible providers use a Bearer token. */
    private static void applyAuthentication(
            HttpURLConnection connection,
            ModelProvider provider,
            String apiKey
    ) {
        if (provider.protocol() == ModelProvider.ApiProtocol.ANTHROPIC_MESSAGES) {
            connection.setRequestProperty("x-api-key", apiKey);
            connection.setRequestProperty("anthropic-version", ANTHROPIC_VERSION);
            return;
        }
        connection.setRequestProperty("Authorization", "Bearer " + apiKey);
    }

    /** Reads provider error streams as well as ordinary successful response streams. */
    private static String readResponseBody(HttpURLConnection connection, int statusCode) throws IOException {
        InputStream stream = statusCode >= 200 && statusCode < 300
                ? connection.getInputStream()
                : connection.getErrorStream();
        if (stream == null) {
            return "";
        }
        try (stream) {
            return new String(stream.readAllBytes(), StandardCharsets.UTF_8);
        }
    }

    /** Separates credential failures from endpoint/model/provider errors before parsing any response. */
    private static void ensureSuccess(ModelProvider provider, int statusCode, String responseBody) throws IOException {
        if (statusCode >= 200 && statusCode < 300) {
            return;
        }
        if (statusCode == HttpURLConnection.HTTP_UNAUTHORIZED || statusCode == HttpURLConnection.HTTP_FORBIDDEN) {
            throw new IOException(provider.displayName() + " authentication failed (HTTP " + statusCode
                    + "). Check the API key saved for this provider.");
        }
        throw new IOException(provider.displayName() + " API returned HTTP " + statusCode + ": "
                + boundedError(responseBody));
    }

    /** Turns low-level network failures into instructions that match GoLand's network settings. */
    static IOException explainNetworkFailure(IOException failure, int timeoutSeconds) {
        if (failure instanceof SocketTimeoutException) {
            return new IOException("Connection timed out after " + timeoutSeconds
                    + " seconds. Check GoLand HTTP Proxy settings and whether the API host is reachable.", failure);
        }
        if (failure instanceof UnknownHostException) {
            return new IOException("Could not resolve the API host. Check DNS and GoLand HTTP Proxy settings.", failure);
        }
        if (failure instanceof ConnectException) {
            return new IOException("Could not connect to the API host. Check the URL, firewall, and GoLand HTTP Proxy settings.", failure);
        }
        if (failure instanceof SSLException) {
            return new IOException("TLS connection failed. Check certificates, HTTPS interception, and GoLand proxy settings.", failure);
        }
        return failure;
    }

    /** Dispatches response extraction through the same protocol selected for request construction. */
    static String parseProviderResponse(AiCommitSettings.SettingsState settings, String responseBody)
            throws IOException {
        String content = switch (settings.provider().protocol()) {
            case OPENAI_CHAT_COMPLETIONS -> openAiContent(responseBody);
            case ANTHROPIC_MESSAGES -> anthropicContent(responseBody);
        };
        return validateMessage(extractMessage(content, settings.structuredOutput));
    }

    /** Accepts standard choices and the common top-level output_text compatibility fallback. */
    static String parseOpenAiResponse(String responseBody, boolean structuredOutput) throws IOException {
        return validateMessage(extractMessage(openAiContent(responseBody), structuredOutput));
    }

    /** Finds the first Chat Completions message without accepting unrelated JSON as success. */
    private static String openAiContent(String responseBody) throws IOException {
        try {
            JsonObject root = JsonParser.parseString(responseBody).getAsJsonObject();
            JsonArray choices = root.getAsJsonArray("choices");
            if (choices != null && !choices.isEmpty()) {
                JsonObject message = choices.get(0).getAsJsonObject().getAsJsonObject("message");
                JsonElement content = message == null ? null : message.get("content");
                if (content != null && content.isJsonPrimitive()) {
                    return content.getAsString();
                }
            }
            JsonElement outputText = root.get("output_text");
            if (outputText != null && outputText.isJsonPrimitive()) {
                return outputText.getAsString();
            }
            throw new IOException("Model API response did not contain choices[0].message.content");
        } catch (JsonParseException | IllegalStateException | IndexOutOfBoundsException | NullPointerException exception) {
            throw new IOException("Model API returned an unsupported OpenAI-compatible JSON response", exception);
        }
    }

    /** Concatenates Anthropic text blocks while ignoring thinking/tool blocks. */
    private static String anthropicContent(String responseBody) throws IOException {
        try {
            JsonArray content = JsonParser.parseString(responseBody).getAsJsonObject().getAsJsonArray("content");
            StringBuilder text = new StringBuilder();
            if (content != null) {
                for (JsonElement element : content) {
                    JsonObject block = element.getAsJsonObject();
                    if ("text".equals(block.has("type") ? block.get("type").getAsString() : null)
                            && block.has("text")) {
                        text.append(block.get("text").getAsString());
                    }
                }
            }
            if (!text.isEmpty()) {
                return text.toString();
            }
            throw new IOException("Anthropic response did not contain a text content block");
        } catch (JsonParseException | IllegalStateException | NullPointerException exception) {
            throw new IOException("Anthropic API returned an unsupported JSON response", exception);
        }
    }

    /** Creates the strict, single-property schema used by every supported provider. */
    private static JsonObject messageSchema(int maxCharacters) {
        JsonObject messageProperty = new JsonObject();
        messageProperty.addProperty("type", "string");
        messageProperty.addProperty("minLength", 1);
        if (maxCharacters > 0) {
            messageProperty.addProperty("maxLength", maxCharacters);
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
        return schema;
    }

    /** Wraps the shared schema in the Chat Completions response_format contract. */
    private static JsonObject openAiResponseFormat(int maxCharacters) {
        JsonObject jsonSchema = new JsonObject();
        jsonSchema.addProperty("name", "commit_message");
        jsonSchema.addProperty("strict", true);
        jsonSchema.add("schema", messageSchema(maxCharacters));
        JsonObject responseFormat = new JsonObject();
        responseFormat.addProperty("type", "json_schema");
        responseFormat.add("json_schema", jsonSchema);
        return responseFormat;
    }

    /** Wraps the same schema in Anthropic's output_config.format contract. */
    private static JsonObject anthropicOutputConfig(int maxCharacters) {
        JsonObject format = new JsonObject();
        format.addProperty("type", "json_schema");
        format.add("schema", messageSchema(maxCharacters));
        JsonObject outputConfig = new JsonObject();
        outputConfig.add("format", format);
        return outputConfig;
    }

    /** Reads schema JSON only when the user enabled structured output. */
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

    /** Rejects empty output but preserves custom-prompt formatting and length verbatim. */
    private static String validateMessage(String raw) throws IOException {
        if (raw.isBlank()) {
            throw new IOException("Model API returned an empty commit message");
        }
        return raw;
    }

    /** Prevents large or secret-bearing provider error pages from flooding IDE notifications. */
    private static String boundedError(String body) {
        String flattened = body == null ? "empty response" : body.replaceAll("\\s+", " ").trim();
        return flattened.length() <= MAX_ERROR_CHARACTERS
                ? flattened
                : flattened.substring(0, MAX_ERROR_CHARACTERS) + "...";
    }

    /** Small immutable transport result keeps protocol parsing independent from connections. */
    private record ProviderResponse(int statusCode, String body) {
    }
}
