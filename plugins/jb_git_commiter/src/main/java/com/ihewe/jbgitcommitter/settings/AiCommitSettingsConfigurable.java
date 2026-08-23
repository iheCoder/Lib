package com.ihewe.jbgitcommitter.settings;

import com.ihewe.jbgitcommitter.api.OpenAiCompatibleClient;
import com.ihewe.jbgitcommitter.context.FileContextPolicy;
import com.intellij.openapi.application.ApplicationManager;
import com.intellij.openapi.options.Configurable;
import com.intellij.openapi.options.ConfigurationException;
import com.intellij.ui.JBColor;
import com.intellij.ui.components.JBCheckBox;
import com.intellij.ui.components.JBLabel;
import com.intellij.ui.components.JBPasswordField;
import com.intellij.ui.components.JBScrollPane;
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
import java.util.List;

/** Builds the Settings page without reading secrets back onto the UI thread. */
public final class AiCommitSettingsConfigurable implements Configurable {
    private static final int MIN_CONTEXT_CHARS = 1_000;
    private static final int MAX_CONTEXT_CHARS = 500_000;
    private static final int MIN_TIMEOUT_SECONDS = 5;
    private static final int MAX_TIMEOUT_SECONDS = 600;

    private JPanel panel;
    private JBTextField endpointField;
    private JBTextField modelField;
    private JBTextField maxContextField;
    private JBTextField timeoutField;
    private JBTextArea defaultPromptArea;
    private JBTextArea customPromptArea;
    private JBTextArea generatedPatternsArea;
    private JBTextArea sourceGeneratedRulesArea;
    private JBPasswordField apiKeyField;
    private JBCheckBox structuredOutputCheckBox;
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
        initializeFields();
        panel = buildForm();
        reset();
        return new JBScrollPane(panel);
    }

    /** Creates all controls in one place so form composition remains a linear overview. */
    private void initializeFields() {
        endpointField = new JBTextField();
        modelField = new JBTextField();
        maxContextField = new JBTextField();
        timeoutField = new JBTextField();
        defaultPromptArea = wrappingTextArea(9);
        defaultPromptArea.setText(AiCommitSettings.DEFAULT_PROMPT);
        defaultPromptArea.setEditable(false);
        customPromptArea = wrappingTextArea(9);
        generatedPatternsArea = new JBTextArea(8, 40);
        sourceGeneratedRulesArea = new JBTextArea(6, 40);
        apiKeyField = new JBPasswordField();
        structuredOutputCheckBox = new JBCheckBox("Use strict JSON Schema output (disable for incompatible APIs)");
        clearApiKeyCheckBox = new JBCheckBox("Clear the saved API key");
        testApiButton = new JButton("Test API");
        testApiButton.addActionListener(event -> testApiConnection());
        testStatusLabel = new JBLabel("The test sends only 'Reply with OK', never repository content.");
    }

    /** Groups provider, output, prompt, and context-policy settings in their execution order. */
    private JPanel buildForm() {
        return FormBuilder.createFormBuilder()
                .addLabeledComponent(new JBLabel("Chat Completions URL:"), endpointField, 1, false)
                .addLabeledComponent(new JBLabel("Model:"), modelField, 1, false)
                .addLabeledComponent(new JBLabel("API key (leave blank to keep saved key):"), apiKeyField, 1, false)
                .addLabeledComponent(new JBLabel("Connection:"), createTestPanel(), 1, false)
                .addComponent(clearApiKeyCheckBox, 1)
                .addComponent(structuredOutputCheckBox, 1)
                .addLabeledComponent(new JBLabel("Maximum context characters:"), maxContextField, 1, false)
                .addLabeledComponent(new JBLabel("Request timeout (seconds):"), timeoutField, 1, false)
                .addLabeledComponentFillVertically("Default prompt (read-only):", new JScrollPane(defaultPromptArea))
                .addLabeledComponentFillVertically(
                        "Custom prompt override (blank uses Default Prompt):",
                        new JScrollPane(customPromptArea)
                )
                .addLabeledComponentFillVertically("Generated file globs (one per line):", new JScrollPane(generatedPatternsArea))
                .addLabeledComponentFillVertically("Source → Generated rules:", new JScrollPane(sourceGeneratedRulesArea))
                .addComponentFillVertically(new JPanel(), 0)
                .getPanel();
    }

    /** Applies consistent wrapping to prose prompts while leaving glob schemas line-oriented. */
    private static JBTextArea wrappingTextArea(int rows) {
        JBTextArea area = new JBTextArea(rows, 40);
        area.setLineWrap(true);
        area.setWrapStyleWord(true);
        return area;
    }

    /** Compares every editable non-secret value and treats entered/cleared credentials as changes. */
    @Override
    public boolean isModified() {
        AiCommitSettings.SettingsState state = AiCommitSettings.getInstance().getState();
        return !endpointField.getText().trim().equals(state.endpoint)
                || !modelField.getText().trim().equals(state.model)
                || !customPromptArea.getText().trim().equals(state.customPrompt)
                || !generatedPatternsArea.getText().trim().equals(state.generatedPatterns)
                || !sourceGeneratedRulesArea.getText().trim().equals(state.sourceGeneratedRules)
                || !maxContextField.getText().trim().equals(String.valueOf(state.maxContextChars))
                || !timeoutField.getText().trim().equals(String.valueOf(state.requestTimeoutSeconds))
                || structuredOutputCheckBox.isSelected() != state.structuredOutput
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
        customPromptArea.setText(state.customPrompt);
        generatedPatternsArea.setText(state.generatedPatterns);
        sourceGeneratedRulesArea.setText(state.sourceGeneratedRules);
        maxContextField.setText(String.valueOf(state.maxContextChars));
        timeoutField.setText(String.valueOf(state.requestTimeoutSeconds));
        structuredOutputCheckBox.setSelected(state.structuredOutput);
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
        maxContextField = null;
        timeoutField = null;
        defaultPromptArea = null;
        customPromptArea = null;
        generatedPatternsArea = null;
        sourceGeneratedRulesArea = null;
        apiKeyField = null;
        structuredOutputCheckBox = null;
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
        int maxContext = parseBoundedInt(
                maxContextField.getText(), "Maximum context", MIN_CONTEXT_CHARS, MAX_CONTEXT_CHARS
        );
        int timeout = parseBoundedInt(
                timeoutField.getText(), "Request timeout", MIN_TIMEOUT_SECONDS, MAX_TIMEOUT_SECONDS
        );
        if (!endpoint.startsWith("http://") && !endpoint.startsWith("https://")) {
            throw new ConfigurationException("Chat Completions URL must start with http:// or https://");
        }
        if (model.isBlank()) {
            throw new ConfigurationException("Model cannot be empty");
        }
        String generatedPatterns = generatedPatternsArea.getText().trim();
        String sourceGeneratedRules = sourceGeneratedRulesArea.getText().trim();
        validateFilePolicy(generatedPatterns, sourceGeneratedRules);
        return new FormValues(
                endpoint, model, customPromptArea.getText().trim(), generatedPatterns, sourceGeneratedRules,
                maxContext, timeout, structuredOutputCheckBox.isSelected()
        );
    }

    /** Reuses the production parser so malformed Source → Generated rules cannot be saved or tested. */
    private static void validateFilePolicy(String generatedPatterns, String sourceGeneratedRules)
            throws ConfigurationException {
        try {
            FileContextPolicy.select(List.of(), generatedPatterns, sourceGeneratedRules);
        } catch (IllegalArgumentException exception) {
            throw new ConfigurationException(exception.getMessage());
        }
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
            String customPrompt,
            String generatedPatterns,
            String sourceGeneratedRules,
            int maxContext,
            int timeout,
            boolean structuredOutput
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
            state.customPrompt = customPrompt;
            state.generatedPatterns = generatedPatterns;
            state.sourceGeneratedRules = sourceGeneratedRules;
            state.maxContextChars = maxContext;
            state.requestTimeoutSeconds = timeout;
            state.structuredOutput = structuredOutput;
            state.schemaVersion = AiCommitSettings.CURRENT_SCHEMA_VERSION;
        }
    }
}
