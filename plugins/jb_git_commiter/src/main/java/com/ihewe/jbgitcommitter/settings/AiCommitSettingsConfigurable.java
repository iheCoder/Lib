package com.ihewe.jbgitcommitter.settings;

import com.intellij.openapi.options.Configurable;
import com.intellij.openapi.options.ConfigurationException;
import com.intellij.ui.components.JBCheckBox;
import com.intellij.ui.components.JBLabel;
import com.intellij.ui.components.JBPasswordField;
import com.intellij.ui.components.JBTextArea;
import com.intellij.ui.components.JBTextField;
import com.intellij.util.ui.FormBuilder;
import org.jetbrains.annotations.Nls;
import org.jetbrains.annotations.Nullable;

import javax.swing.JComponent;
import javax.swing.JPanel;
import javax.swing.JScrollPane;
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

        panel = FormBuilder.createFormBuilder()
                .addLabeledComponent(new JBLabel("Chat Completions URL:"), endpointField, 1, false)
                .addLabeledComponent(new JBLabel("Model:"), modelField, 1, false)
                .addLabeledComponent(new JBLabel("API key (leave blank to keep saved key):"), apiKeyField, 1, false)
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

        AiCommitSettings settings = AiCommitSettings.getInstance();
        AiCommitSettings.SettingsState state = settings.getState();
        state.endpoint = endpoint;
        state.model = model;
        state.outputLanguage = language;
        state.additionalInstructions = instructionsArea.getText().trim();
        state.maxContextChars = maxContext;
        state.requestTimeoutSeconds = timeout;
        state.conventionalCommits = conventionalCheckBox.isSelected();
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
}
