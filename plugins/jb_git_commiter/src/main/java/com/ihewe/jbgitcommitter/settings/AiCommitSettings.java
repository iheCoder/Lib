package com.ihewe.jbgitcommitter.settings;

import com.intellij.credentialStore.CredentialAttributes;
import com.intellij.credentialStore.CredentialAttributesKt;
import com.intellij.ide.passwordSafe.PasswordSafe;
import com.intellij.openapi.application.ApplicationManager;
import com.intellij.openapi.components.PersistentStateComponent;
import com.intellij.openapi.components.Service;
import com.intellij.openapi.components.State;
import com.intellij.openapi.components.Storage;
import org.jetbrains.annotations.NotNull;

/**
 * Owns non-secret model settings and delegates API-key persistence to the IDE password store.
 * The state object deliberately excludes the key so Settings Sync and XML exports cannot leak it.
 */
@Service(Service.Level.APP)
@State(name = "com.ihewe.jbgitcommitter.settings.AiCommitSettings", storages = @Storage("AiGitCommitter.xml"))
public final class AiCommitSettings implements PersistentStateComponent<AiCommitSettings.SettingsState> {
    public static final int CURRENT_SCHEMA_VERSION = 3;
    public static final String DEFAULT_ENDPOINT = "https://api.openai.com/v1/chat/completions";
    public static final String DEFAULT_MODEL = "gpt-4.1-mini";
    public static final int DEFAULT_MAX_CONTEXT_CHARS = 60_000;
    public static final String DEFAULT_PROMPT = """
            Generate a concise git commit message for the provided changes.
            Capture the primary intent or behavior change, not a list of individual edits.
            Prefer what the change accomplishes over how it was implemented.
            Be specific when the evidence supports it, but do not invent intent that is not present in the changes.
            If several changes support the same goal, describe that shared goal rather than enumerating them.
            If the changes contain multiple unrelated goals, summarize the most important ones concisely without forcing them into a single invented theme.
            Return only the commit message. No explanation, prefix, quotation marks, or markdown.
            """.strip();
    public static final String DEFAULT_GENERATED_PATTERNS = """
            # One language-independent glob per line.
            **/*.generated.*
            **/*.g.dart
            **/*.freezed.dart
            **/*.pb.go
            **/*_pb2.py
            **/*_pb2.pyi
            **/__generated__/**
            **/generated/**
            **/vendor/**
            **/*.min.js
            **/*.min.css
            **/package-lock.json
            **/yarn.lock
            **/pnpm-lock.yaml
            **/poetry.lock
            **/uv.lock
            **/go.sum
            """.strip();
    public static final String DEFAULT_SOURCE_GENERATED_RULES = """
            # source glob => generated glob, generated glob
            **/*.proto => **/*.pb.go, **/*_pb2.py, **/*_pb2.pyi
            **/*.graphql => **/*.generated.*, **/__generated__/**
            **/openapi*.yaml => **/generated/**
            **/openapi*.yml => **/generated/**
            **/openapi*.json => **/generated/**
            """.strip();
    private static final CredentialAttributes API_KEY_ATTRIBUTES = new CredentialAttributes(
            CredentialAttributesKt.generateServiceName("AI Git Committer", "OpenAI-compatible API key")
    );

    private SettingsState state = new SettingsState();
    private volatile String sessionApiKey;
    private volatile boolean apiKeyLoaded;

    /** Returns the application-level settings service shared by all projects. */
    public static AiCommitSettings getInstance() {
        return ApplicationManager.getApplication().getService(AiCommitSettings.class);
    }

    /** Exposes a stable state instance for callers and the persistence framework. */
    @Override
    public @NotNull SettingsState getState() {
        return state;
    }

    /** Replaces deserialized state atomically to avoid partially applied settings. */
    @Override
    public void loadState(@NotNull SettingsState loadedState) {
        // Older plugin XML may lack current fields. Restore only absent values while preserving
        // deliberate empty pattern/rule lists, which are the supported way to disable filtering.
        loadedState.customPrompt = loadedState.customPrompt == null ? "" : loadedState.customPrompt;
        if (loadedState.outputLanguage == null || loadedState.outputLanguage.isBlank()) {
            loadedState.outputLanguage = "English";
        }
        loadedState.messageMaxCharacters = Math.max(0, loadedState.messageMaxCharacters);
        if (loadedState.generatedPatterns == null) {
            loadedState.generatedPatterns = DEFAULT_GENERATED_PATTERNS;
        }
        if (loadedState.sourceGeneratedRules == null) {
            loadedState.sourceGeneratedRules = DEFAULT_SOURCE_GENERATED_RULES;
        }
        loadedState.schemaVersion = CURRENT_SCHEMA_VERSION;
        state = loadedState;
    }

    /**
     * Loads the API key on a background thread. PasswordSafe can consult the OS keychain and must
     * never be called from Swing's event-dispatch thread.
     */
    public String loadApiKey() {
        if (apiKeyLoaded) {
            return sessionApiKey;
        }
        sessionApiKey = PasswordSafe.getInstance().getPassword(API_KEY_ATTRIBUTES);
        apiKeyLoaded = true;
        return sessionApiKey;
    }

    /** Makes a newly entered key usable immediately, then persists it away from the UI thread. */
    public void saveApiKeyAsync(@NotNull String apiKey) {
        sessionApiKey = apiKey;
        apiKeyLoaded = true;
        ApplicationManager.getApplication().executeOnPooledThread(
                () -> PasswordSafe.getInstance().setPassword(API_KEY_ATTRIBUTES, apiKey)
        );
    }

    /** Clears both the in-memory key and its durable PasswordSafe entry asynchronously. */
    public void clearApiKeyAsync() {
        sessionApiKey = null;
        apiKeyLoaded = true;
        ApplicationManager.getApplication().executeOnPooledThread(
                () -> PasswordSafe.getInstance().setPassword(API_KEY_ATTRIBUTES, null)
        );
    }

    /** Serializable non-secret settings with conservative payload and timeout defaults. */
    public static final class SettingsState {
        public int schemaVersion = CURRENT_SCHEMA_VERSION;
        public String endpoint = DEFAULT_ENDPOINT;
        public String model = DEFAULT_MODEL;
        public String customPrompt = "";
        public String outputLanguage = "English";
        public int messageMaxCharacters = 0;
        public String generatedPatterns = DEFAULT_GENERATED_PATTERNS;
        public String sourceGeneratedRules = DEFAULT_SOURCE_GENERATED_RULES;
        public int maxContextChars = DEFAULT_MAX_CONTEXT_CHARS;
        public int requestTimeoutSeconds = 60;
        public boolean structuredOutput = true;

        /** Empty custom text means the immutable built-in prompt is the generation instruction. */
        public boolean usesDefaultPrompt() {
            return customPrompt == null || customPrompt.isBlank();
        }
    }
}
