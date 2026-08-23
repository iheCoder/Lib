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
    public static final String DEFAULT_ENDPOINT = "https://api.openai.com/v1/chat/completions";
    public static final String DEFAULT_MODEL = "gpt-4.1-mini";
    public static final int DEFAULT_MAX_CONTEXT_CHARS = 60_000;
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
        public String endpoint = DEFAULT_ENDPOINT;
        public String model = DEFAULT_MODEL;
        public String outputLanguage = "中文";
        public String additionalInstructions = "";
        public int maxContextChars = DEFAULT_MAX_CONTEXT_CHARS;
        public int requestTimeoutSeconds = 60;
        public boolean conventionalCommits = true;
    }
}
