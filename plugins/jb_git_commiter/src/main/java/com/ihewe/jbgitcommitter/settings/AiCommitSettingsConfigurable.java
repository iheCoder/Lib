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
import com.intellij.ui.table.JBTable;
import com.intellij.util.ui.FormBuilder;
import org.jetbrains.annotations.Nls;
import org.jetbrains.annotations.Nullable;

import javax.swing.JComponent;
import javax.swing.JButton;
import javax.swing.JComboBox;
import javax.swing.JPanel;
import javax.swing.JScrollBar;
import javax.swing.JScrollPane;
import javax.swing.SwingUtilities;
import javax.swing.table.DefaultTableModel;
import java.awt.BorderLayout;
import java.awt.Container;
import java.awt.FlowLayout;
import java.awt.event.MouseWheelEvent;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/** Builds the Settings page without reading secrets back onto the UI thread. */
public final class AiCommitSettingsConfigurable implements Configurable {
    private static final int MIN_CONTEXT_CHARS = 1_000;
    private static final int MAX_CONTEXT_CHARS = 500_000;
    private static final int MIN_TIMEOUT_SECONDS = 5;
    private static final int MAX_TIMEOUT_SECONDS = 600;
    private static final int MAX_MESSAGE_CHARACTERS = 2_000;

    private JPanel panel;
    private JBScrollPane settingsScrollPane;
    private JBTextField endpointField;
    private JBTextField modelField;
    private JComboBox<String> languageComboBox;
    private JBTextField messageMaxCharactersField;
    private JBTextField maxContextField;
    private JBTextField timeoutField;
    private JBTextArea defaultPromptArea;
    private JBTextArea customPromptArea;
    private JBTextArea generatedPatternsArea;
    private JBTable sourceGeneratedTable;
    private DefaultTableModel sourceGeneratedTableModel;
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
        settingsScrollPane = new JBScrollPane(panel);
        settingsScrollPane.setBorder(null);
        settingsScrollPane.getVerticalScrollBar().setUnitIncrement(20);
        return settingsScrollPane;
    }

    /** Creates all controls in one place so form composition remains a linear overview. */
    private void initializeFields() {
        endpointField = new JBTextField();
        modelField = new JBTextField();
        languageComboBox = new JComboBox<>(new String[]{"English", "中文", "日本語", "한국어"});
        languageComboBox.setEditable(true);
        messageMaxCharactersField = new JBTextField();
        maxContextField = new JBTextField();
        timeoutField = new JBTextField();
        defaultPromptArea = wrappingTextArea(9);
        defaultPromptArea.setText(AiCommitSettings.DEFAULT_PROMPT);
        defaultPromptArea.setEditable(false);
        customPromptArea = wrappingTextArea(9);
        // The default catalog fits without an inner vertical scroll, so ordinary page navigation
        // is not forced to traverse this editor before reaching the next Settings section.
        generatedPatternsArea = new JBTextArea(18, 40);
        initializeSourceGeneratedTable();
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
                .addLabeledComponent(new JBLabel("Commit message language:"), languageComboBox, 1, false)
                .addLabeledComponent(
                        new JBLabel("Maximum message characters (0 = unlimited):"),
                        messageMaxCharactersField,
                        1,
                        false
                )
                .addLabeledComponent(new JBLabel("Maximum context characters:"), maxContextField, 1, false)
                .addLabeledComponent(new JBLabel("Request timeout (seconds):"), timeoutField, 1, false)
                .addLabeledComponentFillVertically("Default prompt (read-only):", scrollable(defaultPromptArea))
                .addLabeledComponentFillVertically(
                        "Custom prompt override (blank uses Default Prompt):",
                        scrollable(customPromptArea)
                )
                .addLabeledComponentFillVertically(
                        "Generated file globs (one per line):",
                        scrollable(generatedPatternsArea)
                )
                .addLabeledComponentFillVertically("Source → Generated rules:", createSourceGeneratedTablePanel())
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

    /** Creates the two-column mapping model requested by the context-policy schema. */
    private void initializeSourceGeneratedTable() {
        sourceGeneratedTableModel = new DefaultTableModel(new Object[]{"Source glob", "Generated globs"}, 0);
        sourceGeneratedTable = new JBTable(sourceGeneratedTableModel);
        sourceGeneratedTable.setFillsViewportHeight(true);
        sourceGeneratedTable.putClientProperty("terminateEditOnFocusLost", Boolean.TRUE);
    }

    /** Adds explicit row controls while keeping the mapping relationship visually obvious. */
    private JPanel createSourceGeneratedTablePanel() {
        JBScrollPane tableScrollPane = scrollable(sourceGeneratedTable);
        tableScrollPane.setPreferredSize(new java.awt.Dimension(640, 150));
        JButton addButton = new JButton("Add mapping");
        addButton.addActionListener(event -> sourceGeneratedTableModel.addRow(new Object[]{"", ""}));
        JButton removeButton = new JButton("Remove selected");
        removeButton.addActionListener(event -> removeSelectedMappings());
        JPanel actions = new JPanel(new FlowLayout(FlowLayout.LEFT, 0, 4));
        actions.add(addButton);
        actions.add(new JBLabel("  "));
        actions.add(removeButton);
        JPanel tablePanel = new JPanel(new BorderLayout());
        tablePanel.add(tableScrollPane, BorderLayout.CENTER);
        tablePanel.add(actions, BorderLayout.SOUTH);
        return tablePanel;
    }

    /** Removes selected rows from bottom to top so model indices remain stable. */
    private void removeSelectedMappings() {
        int[] selectedRows = sourceGeneratedTable.getSelectedRows();
        for (int index = selectedRows.length - 1; index >= 0; index--) {
            sourceGeneratedTableModel.removeRow(selectedRows[index]);
        }
    }

    /** Forwards wheel input to the page only when the nested editor/table cannot scroll further. */
    private static <T extends java.awt.Component> JBScrollPane scrollable(T view) {
        JBScrollPane scrollPane = new JBScrollPane(view);
        scrollPane.addMouseWheelListener(event -> forwardWheelAtBoundary(scrollPane, event));
        return scrollPane;
    }

    /** Prevents nested scroll panes from trapping upward/downward page navigation at their edges. */
    private static void forwardWheelAtBoundary(JScrollPane inner, MouseWheelEvent event) {
        JScrollBar bar = inner.getVerticalScrollBar();
        JScrollPane outer = shouldForwardWheel(
                event.getWheelRotation(),
                bar.getValue(),
                bar.getVisibleAmount(),
                bar.getMinimum(),
                bar.getMaximum()
        ) ? findOuterScrollPane(inner) : null;
        if (outer == null) {
            return;
        }
        outer.dispatchEvent(SwingUtilities.convertMouseEvent(inner, event, outer));
        event.consume();
    }

    /** Pure boundary decision kept visible to regression tests for both scroll directions. */
    static boolean shouldForwardWheel(int rotation, int value, int visibleAmount, int minimum, int maximum) {
        boolean atTop = rotation < 0 && value <= minimum;
        boolean atBottom = rotation > 0 && value + visibleAmount >= maximum;
        return atTop || atBottom;
    }

    /** Finds the page scroller without assuming a particular Settings-dialog component hierarchy. */
    private static JScrollPane findOuterScrollPane(JScrollPane inner) {
        Container ancestor = inner.getParent();
        while (ancestor != null) {
            if (ancestor instanceof JScrollPane scrollPane) {
                return scrollPane;
            }
            ancestor = ancestor.getParent();
        }
        return null;
    }

    /** Compares every editable non-secret value and treats entered/cleared credentials as changes. */
    @Override
    public boolean isModified() {
        AiCommitSettings.SettingsState state = AiCommitSettings.getInstance().getState();
        return !endpointField.getText().trim().equals(state.endpoint)
                || !modelField.getText().trim().equals(state.model)
                || !selectedLanguage().equals(state.outputLanguage)
                || !messageMaxCharactersField.getText().trim().equals(String.valueOf(state.messageMaxCharacters))
                || !customPromptArea.getText().trim().equals(state.customPrompt)
                || !generatedPatternsArea.getText().trim().equals(state.generatedPatterns)
                || !currentMappingSchema(false).equals(normalizeMappingSchema(state.sourceGeneratedRules))
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
        languageComboBox.setSelectedItem(state.outputLanguage);
        messageMaxCharactersField.setText(String.valueOf(state.messageMaxCharacters));
        customPromptArea.setText(state.customPrompt);
        generatedPatternsArea.setText(state.generatedPatterns);
        loadMappingTable(state.sourceGeneratedRules);
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
        settingsScrollPane = null;
        endpointField = null;
        modelField = null;
        languageComboBox = null;
        messageMaxCharactersField = null;
        maxContextField = null;
        timeoutField = null;
        defaultPromptArea = null;
        customPromptArea = null;
        generatedPatternsArea = null;
        sourceGeneratedTable = null;
        sourceGeneratedTableModel = null;
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
        int messageMaxCharacters = parseBoundedInt(
                messageMaxCharactersField.getText(), "Maximum message characters", 0, MAX_MESSAGE_CHARACTERS
        );
        if (!endpoint.startsWith("http://") && !endpoint.startsWith("https://")) {
            throw new ConfigurationException("Chat Completions URL must start with http:// or https://");
        }
        if (model.isBlank()) {
            throw new ConfigurationException("Model cannot be empty");
        }
        String outputLanguage = selectedLanguage();
        if (outputLanguage.isBlank()) {
            throw new ConfigurationException("Commit message language cannot be empty");
        }
        String generatedPatterns = generatedPatternsArea.getText().trim();
        String sourceGeneratedRules;
        try {
            sourceGeneratedRules = currentMappingSchema(true);
        } catch (IllegalArgumentException exception) {
            throw new ConfigurationException(exception.getMessage());
        }
        validateFilePolicy(generatedPatterns, sourceGeneratedRules);
        return new FormValues(
                endpoint, model, outputLanguage, messageMaxCharacters, customPromptArea.getText().trim(),
                generatedPatterns, sourceGeneratedRules, maxContext, timeout, structuredOutputCheckBox.isSelected()
        );
    }

    /** Reads the editable combo box so predefined and user-entered languages follow one path. */
    private String selectedLanguage() {
        Object selected = languageComboBox.getEditor().getItem();
        return selected == null ? "" : selected.toString().trim();
    }

    /** Replaces all mapping rows from persisted schema without exposing its line-based encoding. */
    private void loadMappingTable(String schema) {
        sourceGeneratedTableModel.setRowCount(0);
        for (FileContextPolicy.SourceGeneratedMapping mapping : FileContextPolicy.parseMappings(schema)) {
            sourceGeneratedTableModel.addRow(new Object[]{mapping.sourceGlob(), mapping.generatedGlobs()});
        }
    }

    /** Converts current table cells to schema; strict mode surfaces half-filled rows on Apply/Test. */
    private String currentMappingSchema(boolean strict) {
        if (sourceGeneratedTable.isEditing()) {
            sourceGeneratedTable.getCellEditor().stopCellEditing();
        }
        List<FileContextPolicy.SourceGeneratedMapping> mappings = new ArrayList<>();
        for (int row = 0; row < sourceGeneratedTableModel.getRowCount(); row++) {
            mappings.add(new FileContextPolicy.SourceGeneratedMapping(
                    cellText(row, 0),
                    cellText(row, 1)
            ));
        }
        try {
            return FileContextPolicy.formatMappings(mappings);
        } catch (IllegalArgumentException exception) {
            if (strict) {
                throw exception;
            }
            return mappings.toString();
        }
    }

    /** Normalizes comments and whitespace before comparing persisted mappings with table rows. */
    private static String normalizeMappingSchema(String schema) {
        return FileContextPolicy.formatMappings(FileContextPolicy.parseMappings(schema));
    }

    /** Reads one table cell defensively because a newly added row initially contains empty values. */
    private String cellText(int row, int column) {
        Object value = sourceGeneratedTableModel.getValueAt(row, column);
        return value == null ? "" : value.toString();
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
            String outputLanguage,
            int messageMaxCharacters,
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
            state.outputLanguage = outputLanguage;
            state.messageMaxCharacters = messageMaxCharacters;
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
