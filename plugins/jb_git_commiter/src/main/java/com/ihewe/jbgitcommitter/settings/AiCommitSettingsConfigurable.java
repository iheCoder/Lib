package com.ihewe.jbgitcommitter.settings;

import com.ihewe.jbgitcommitter.api.OpenAiCompatibleClient;
import com.intellij.openapi.application.ApplicationManager;
import com.intellij.openapi.options.Configurable;
import com.intellij.openapi.options.ConfigurationException;
import com.intellij.ui.JBColor;
import com.intellij.ui.components.JBCheckBox;
import com.intellij.ui.components.JBLabel;
import com.intellij.ui.components.JBPasswordField;
import com.intellij.ui.components.JBTextArea;
import com.intellij.ui.components.JBTextField;
import com.intellij.util.ui.FormBuilder;
import org.jetbrains.annotations.Nls;
import org.jetbrains.annotations.Nullable;

import javax.swing.JComponent;
import javax.swing.JButton;
import javax.swing.JPanel;
import javax.swing.JScrollPane;
import java.awt.FlowLayout;
import java.util.Arrays;

/** Builds the Settings page without reading secrets back onto the UI thread. */
public final class AiCommitSettingsConfigurable implements Configurable {
    private static final int MIN_CONTEXT_CHARS = 1_000;
    private static final int MAX_CONTEXT_CHARS = 500_000;
    private static final int MIN_TIMEOUT_SECONDS = 5;
    private static final int MAX_TIMEOUT_SECONDS = 600;

    private JPanel panel;
    private JBTextField endpointField;
    private JBTextField modelField;
    private JBTextField languageField;
    private JBTextField maxContextField;
    private JBTextField timeoutField;
    private JBTextArea instructionsArea;
    private JBPasswordField apiKeyField;
    private JBCheckBox conventionalCheckBox;
    private JBCheckBox clearApiKeyCheckBox;
    private JButton testApiButton;
    private JBLabel testStatusLabel;

    /** Provides the searchable display name under Settings | Tools. */
    @Override
    public @Nls String getDisplayName() {
        return "AI Git Committer";
    }

    /** Creates a compact form; API-key input is write-only to avoid exposing stored credentials. */
    @Override
    public @Nullable JComponent createComponent() {
        endpointField = new JBTextField();
        modelField = new JBTextField();
        languageField = new JBTextField();
        maxContextField = new JBTextField();
        timeoutField = new JBTextField();
        instructionsArea = new JBTextArea(4, 40);
        instructionsArea.setLineWrap(true);
        instructionsArea.setWrapStyleWord(true);
        apiKeyField = new JBPasswordField();
        conventionalCheckBox = new JBCheckBox("Use Conventional Commits format");
        clearApiKeyCheckBox = new JBCheckBox("Clear the saved API key");
        testApiButton = new JButton("Test API");
        testApiButton.addActionListener(event -> testApiConnection());
        testStatusLabel = new JBLabel("The test sends only 'Reply with OK', never repository content.");

        panel = FormBuilder.createFormBuilder()
                .addLabeledComponent(new JBLabel("Chat Completions URL:"), endpointField, 1, false)
                .addLabeledComponent(new JBLabel("Model:"), modelField, 1, false)
                .addLabeledComponent(new JBLabel("API key (leave blank to keep saved key):"), apiKeyField, 1, false)
                .addLabeledComponent(new JBLabel("Connection:"), createTestPanel(), 1, false)
                .addComponent(clearApiKeyCheckBox, 1)
                .addLabeledComponent(new JBLabel("Output language:"), languageField, 1, false)
                .addComponent(conventionalCheckBox, 1)
                .addLabeledComponent(new JBLabel("Maximum context characters:"), maxContextField, 1, false)
                .addLabeledComponent(new JBLabel("Request timeout (seconds):"), timeoutField, 1, false)
                .addLabeledComponentFillVertically("Additional instructions:", new JScrollPane(instructionsArea))
                .addComponentFillVertically(new JPanel(), 0)
                .getPanel();
        reset();
        return panel;
    }

    /** Compares every editable non-secret value and treats entered/cleared credentials as changes. */
    @Override
    public boolean isModified() {
        AiCommitSettings.SettingsState state = AiCommitSettings.getInstance().getState();
        return !endpointField.getText().trim().equals(state.endpoint)
                || !modelField.getText().trim().equals(state.model)
                || !languageField.getText().trim().equals(state.outputLanguage)
                || !instructionsArea.getText().trim().equals(state.additionalInstructions)
                || !maxContextField.getText().trim().equals(String.valueOf(state.maxContextChars))
                || !timeoutField.getText().trim().equals(String.valueOf(state.requestTimeoutSeconds))
                || conventionalCheckBox.isSelected() != state.conventionalCommits
                || apiKeyField.getPassword().length > 0
                || clearApiKeyCheckBox.isSelected();
    }

    /** Validates bounds before mutating state, then schedules any keychain write off the UI thread. */
    @Override
    public void apply() throws ConfigurationException {
        FormValues values = readFormValues();
        AiCommitSettings settings = AiCommitSettings.getInstance();
        AiCommitSettings.SettingsState state = settings.getState();
        values.copyTo(state);
        applyCredentialChange(settings);
    }

    /** Restores non-secret values and intentionally leaves the password field empty. */
    @Override
    public void reset() {
        AiCommitSettings.SettingsState state = AiCommitSettings.getInstance().getState();
        endpointField.setText(state.endpoint);
        modelField.setText(state.model);
        languageField.setText(state.outputLanguage);
        instructionsArea.setText(state.additionalInstructions);
        maxContextField.setText(String.valueOf(state.maxContextChars));
        timeoutField.setText(String.valueOf(state.requestTimeoutSeconds));
        conventionalCheckBox.setSelected(state.conventionalCommits);
        apiKeyField.setText("");
        clearApiKeyCheckBox.setSelected(false);
        resetTestStatus();
    }

    /** Drops component references when the settings dialog is disposed. */
    @Override
    public void disposeUIResources() {
        panel = null;
        endpointField = null;
        modelField = null;
        languageField = null;
        maxContextField = null;
        timeoutField = null;
        instructionsArea = null;
        apiKeyField = null;
        conventionalCheckBox = null;
        clearApiKeyCheckBox = null;
        testApiButton = null;
        testStatusLabel = null;
    }

    /** Aligns the test button and status text without coupling form layout to networking state. */
    private JPanel createTestPanel() {
        JPanel testPanel = new JPanel(new FlowLayout(FlowLayout.LEFT, 0, 0));
        testPanel.add(testApiButton);
        testPanel.add(new JBLabel("  "));
        testPanel.add(testStatusLabel);
        return testPanel;
    }

    /** Validates unsaved form values and tests exactly those values on a pooled thread. */
    private void testApiConnection() {
        final FormValues values;
        try {
            values = readFormValues();
        } catch (ConfigurationException exception) {
            showTestResult(false, exception.getLocalizedMessage());
            return;
        }
        String enteredApiKey = readEnteredApiKey();
        if (clearApiKeyCheckBox.isSelected() && enteredApiKey.isBlank()) {
            showTestResult(false, "API key is marked for removal; enter a key to test.");
            return;
        }
        testApiButton.setEnabled(false);
        testStatusLabel.setForeground(JBColor.GRAY);
        testStatusLabel.setText("Testing connection...");
        ApplicationManager.getApplication().executeOnPooledThread(
                () -> runApiTest(values, enteredApiKey)
        );
    }

    /** Loads a saved key only when the write-only field is blank, then makes a minimal API request. */
    private void runApiTest(FormValues values, String enteredApiKey) {
        try {
            String apiKey = enteredApiKey.isBlank() ? AiCommitSettings.getInstance().loadApiKey() : enteredApiKey;
            if (apiKey == null || apiKey.isBlank()) {
                showTestResult(false, "Enter an API key or save one first.");
                return;
            }
            new OpenAiCompatibleClient().testConnection(values.toSettingsState(), apiKey);
            showTestResult(true, "Connection successful. Click Apply to save new values.");
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            showTestResult(false, "Connection test was interrupted.");
        } catch (Exception exception) {
            String message = exception.getMessage() == null ? exception.getClass().getSimpleName() : exception.getMessage();
            showTestResult(false, message);
        }
    }

    /** Applies the asynchronous test result only while this Settings page still exists. */
    private void showTestResult(boolean success, String message) {
        ApplicationManager.getApplication().invokeLater(() -> {
            if (testApiButton == null || testStatusLabel == null) {
                return;
            }
            testApiButton.setEnabled(true);
            testStatusLabel.setForeground(success ? JBColor.GREEN : JBColor.RED);
            testStatusLabel.setText((success ? "✓ " : "✗ ") + message);
        });
    }

    /** Restores the neutral privacy explanation whenever fields are reset. */
    private void resetTestStatus() {
        if (testStatusLabel != null) {
            testStatusLabel.setForeground(JBColor.GRAY);
            testStatusLabel.setText("The test sends only 'Reply with OK', never repository content.");
        }
    }

    /** Copies the write-only password field and immediately clears the temporary character array. */
    private String readEnteredApiKey() {
        char[] password = apiKeyField.getPassword();
        try {
            return new String(password);
        } finally {
            Arrays.fill(password, '\0');
        }
    }

    /** Parses and validates all fields once so Apply and Test API share exactly the same contract. */
    private FormValues readFormValues() throws ConfigurationException {
        String endpoint = endpointField.getText().trim();
        String model = modelField.getText().trim();
        String language = languageField.getText().trim();
        int maxContext = parseBoundedInt(
                maxContextField.getText(), "Maximum context", MIN_CONTEXT_CHARS, MAX_CONTEXT_CHARS
        );
        int timeout = parseBoundedInt(
                timeoutField.getText(), "Request timeout", MIN_TIMEOUT_SECONDS, MAX_TIMEOUT_SECONDS
        );
        if (!endpoint.startsWith("http://") && !endpoint.startsWith("https://")) {
            throw new ConfigurationException("Chat Completions URL must start with http:// or https://");
        }
        if (model.isBlank() || language.isBlank()) {
            throw new ConfigurationException("Model and output language cannot be empty");
        }
        return new FormValues(endpoint, model, language, instructionsArea.getText().trim(), maxContext, timeout,
                conventionalCheckBox.isSelected());
    }

    /** Parses a numeric setting with explicit operational bounds to prevent accidental huge requests. */
    private static int parseBoundedInt(String raw, String label, int minimum, int maximum)
            throws ConfigurationException {
        try {
            int value = Integer.parseInt(raw.trim());
            if (value < minimum || value > maximum) {
                throw new ConfigurationException(label + " must be between " + minimum + " and " + maximum);
            }
            return value;
        } catch (NumberFormatException exception) {
            throw new ConfigurationException(label + " must be an integer");
        }
    }

    /** Gives clear-key precedence and wipes the temporary password char array after copying it. */
    private void applyCredentialChange(AiCommitSettings settings) {
        char[] password = apiKeyField.getPassword();
        try {
            if (clearApiKeyCheckBox.isSelected()) {
                settings.clearApiKeyAsync();
            } else if (password.length > 0) {
                settings.saveApiKeyAsync(new String(password));
            }
        } finally {
            Arrays.fill(password, '\0');
            apiKeyField.setText("");
            clearApiKeyCheckBox.setSelected(false);
        }
    }

    /** Immutable validated form snapshot used across the UI/background-thread boundary. */
    private record FormValues(
            String endpoint,
            String model,
            String language,
            String instructions,
            int maxContext,
            int timeout,
            boolean conventionalCommits
    ) {
        /** Produces an isolated request configuration for Test API without mutating saved settings. */
        private AiCommitSettings.SettingsState toSettingsState() {
            AiCommitSettings.SettingsState state = new AiCommitSettings.SettingsState();
            copyTo(state);
            return state;
        }

        /** Copies every validated non-secret field to the target persistent or temporary state. */
        private void copyTo(AiCommitSettings.SettingsState state) {
            state.endpoint = endpoint;
            state.model = model;
            state.outputLanguage = language;
            state.additionalInstructions = instructions;
            state.maxContextChars = maxContext;
            state.requestTimeoutSeconds = timeout;
            state.conventionalCommits = conventionalCommits;
        }
    }
}
