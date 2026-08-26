package com.ihewe.jbgitcommitter.settings;

import com.ihewe.jbgitcommitter.api.ModelProvider;
import com.intellij.credentialStore.CredentialAttributes;
import com.intellij.credentialStore.CredentialAttributesKt;
import com.intellij.ide.passwordSafe.PasswordSafe;
import com.intellij.openapi.application.ApplicationManager;
import com.intellij.openapi.components.PersistentStateComponent;
import com.intellij.openapi.components.Service;
import com.intellij.openapi.components.State;
import com.intellij.openapi.components.Storage;
import org.jetbrains.annotations.NotNull;

import java.util.EnumMap;
import java.util.EnumSet;
import java.util.Map;
import java.util.Set;

/**
 * Owns non-secret model settings and delegates API-key persistence to the IDE password store.
 * The state object deliberately excludes the key so Settings Sync and XML exports cannot leak it.
 */
@Service(Service.Level.APP)
@State(name = "com.ihewe.jbgitcommitter.settings.AiCommitSettings", storages = @Storage("AiGitCommitter.xml"))
public final class AiCommitSettings implements PersistentStateComponent<AiCommitSettings.SettingsState> {
    public static final int CURRENT_SCHEMA_VERSION = 4;
    public static final String DEFAULT_ENDPOINT = ModelProvider.OPENAI.defaultEndpoint();
    public static final String DEFAULT_MODEL = ModelProvider.OPENAI.defaultModel();
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
    private static final CredentialAttributes LEGACY_API_KEY_ATTRIBUTES = new CredentialAttributes(
            CredentialAttributesKt.generateServiceName("AI Git Committer", "OpenAI-compatible API key")
    );

    private SettingsState state = new SettingsState();
    private final Map<ModelProvider, String> sessionApiKeys = new EnumMap<>(ModelProvider.class);
    private final Set<ModelProvider> loadedApiKeys = EnumSet.noneOf(ModelProvider.class);
    private volatile ModelProvider legacyCredentialOwner = ModelProvider.OPENAI;

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
    public synchronized void loadState(@NotNull SettingsState loadedState) {
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
        if (loadedState.provider == null || loadedState.provider.isBlank()) {
            loadedState.provider = ModelProvider.inferFromEndpoint(loadedState.endpoint).id();
        }
        ModelProvider provider = ModelProvider.fromId(loadedState.provider);
        legacyCredentialOwner = provider;
        loadedState.provider = provider.id();
        if (loadedState.endpoint == null || loadedState.endpoint.isBlank()) {
            loadedState.endpoint = provider.defaultEndpoint();
        }
        if (loadedState.model == null || loadedState.model.isBlank()) {
            loadedState.model = provider.defaultModel();
        }
        loadedState.schemaVersion = CURRENT_SCHEMA_VERSION;
        state = loadedState;
        sessionApiKeys.clear();
        loadedApiKeys.clear();
    }

    /**
     * Loads the API key on a background thread. PasswordSafe can consult the OS keychain and must
     * never be called from Swing's event-dispatch thread.
     */
    public synchronized String loadApiKey() {
        return loadApiKey(state.provider());
    }

    /** Keeps credentials isolated by provider so switching never sends one vendor another vendor's key. */
    public synchronized String loadApiKey(@NotNull ModelProvider provider) {
        if (loadedApiKeys.contains(provider)) {
            return sessionApiKeys.get(provider);
        }
        String apiKey = PasswordSafe.getInstance().getPassword(apiKeyAttributes(provider));
        // The old credential had no provider ID. Its owner is frozen when persisted state loads,
        // so a later provider switch cannot accidentally claim and transmit somebody else's key.
        if ((apiKey == null || apiKey.isBlank()) && provider == legacyCredentialOwner) {
            apiKey = PasswordSafe.getInstance().getPassword(LEGACY_API_KEY_ATTRIBUTES);
            if (apiKey != null && !apiKey.isBlank()) {
                migrateLegacyApiKey(provider, apiKey);
            }
        }
        sessionApiKeys.put(provider, apiKey);
        loadedApiKeys.add(provider);
        return apiKey;
    }

    /** Makes a newly entered key usable immediately, then persists it away from the UI thread. */
    public void saveApiKeyAsync(@NotNull ModelProvider provider, @NotNull String apiKey) {
        synchronized (this) {
            sessionApiKeys.put(provider, apiKey);
            loadedApiKeys.add(provider);
        }
        ApplicationManager.getApplication().executeOnPooledThread(
                () -> persistApiKey(provider, apiKey)
        );
    }

    /** Clears both the in-memory key and its durable PasswordSafe entry asynchronously. */
    public void clearApiKeyAsync(@NotNull ModelProvider provider) {
        synchronized (this) {
            sessionApiKeys.remove(provider);
            loadedApiKeys.add(provider);
        }
        ApplicationManager.getApplication().executeOnPooledThread(
                () -> clearPersistedApiKey(provider)
        );
    }

    /** Builds one PasswordSafe key per vendor while retaining the pre-v0.6 shared key for migration. */
    private static CredentialAttributes apiKeyAttributes(ModelProvider provider) {
        return new CredentialAttributes(CredentialAttributesKt.generateServiceName(
                "AI Git Committer",
                provider.displayName() + " API key"
        ));
    }

    /** Moves the single legacy key into its inferred provider slot during the first background read. */
    private static void migrateLegacyApiKey(ModelProvider provider, String apiKey) {
        PasswordSafe.getInstance().setPassword(apiKeyAttributes(provider), apiKey);
        PasswordSafe.getInstance().setPassword(LEGACY_API_KEY_ATTRIBUTES, null);
    }

    /** Replacing the inferred legacy owner also retires the obsolete shared credential. */
    private void persistApiKey(ModelProvider provider, String apiKey) {
        PasswordSafe.getInstance().setPassword(apiKeyAttributes(provider), apiKey);
        if (provider == legacyCredentialOwner) {
            PasswordSafe.getInstance().setPassword(LEGACY_API_KEY_ATTRIBUTES, null);
        }
    }

    /** Clearing the inferred owner removes both its provider slot and any not-yet-migrated key. */
    private void clearPersistedApiKey(ModelProvider provider) {
        PasswordSafe.getInstance().setPassword(apiKeyAttributes(provider), null);
        if (provider == legacyCredentialOwner) {
            PasswordSafe.getInstance().setPassword(LEGACY_API_KEY_ATTRIBUTES, null);
        }
    }

    /** Serializable non-secret settings with conservative payload and timeout defaults. */
    public static final class SettingsState {
        public int schemaVersion = CURRENT_SCHEMA_VERSION;
        public String provider = ModelProvider.OPENAI.id();
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

        /** Converts the persisted stable ID at the boundary instead of leaking string comparisons. */
        public ModelProvider provider() {
            return ModelProvider.fromId(provider);
        }

        /** Empty custom text means the immutable built-in prompt is the generation instruction. */
        public boolean usesDefaultPrompt() {
            return customPrompt == null || customPrompt.isBlank();
        }
    }
}
