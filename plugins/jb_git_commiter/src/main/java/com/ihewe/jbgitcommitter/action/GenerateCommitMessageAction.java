package com.ihewe.jbgitcommitter.action;

import com.ihewe.jbgitcommitter.api.OpenAiCompatibleClient;
import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.ihewe.jbgitcommitter.prompt.CommitPromptBuilder;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import com.ihewe.jbgitcommitter.settings.AiCommitSettingsConfigurable;
import com.ihewe.jbgitcommitter.vcs.SelectedChangeCollector;
import com.intellij.notification.NotificationGroupManager;
import com.intellij.notification.NotificationType;
import com.intellij.openapi.actionSystem.ActionUpdateThread;
import com.intellij.openapi.actionSystem.AnAction;
import com.intellij.openapi.actionSystem.AnActionEvent;
import com.intellij.openapi.application.ApplicationManager;
import com.intellij.openapi.ide.CopyPasteManager;
import com.intellij.openapi.options.ShowSettingsUtil;
import com.intellij.openapi.progress.ProgressIndicator;
import com.intellij.openapi.progress.ProgressManager;
import com.intellij.openapi.progress.Task;
import com.intellij.openapi.project.DumbAware;
import com.intellij.openapi.project.Project;
import com.intellij.openapi.vcs.CommitMessageI;
import com.intellij.openapi.vcs.VcsDataKeys;
import org.jetbrains.annotations.NotNull;

import java.awt.datatransfer.StringSelection;
import java.util.List;

/** Entry point shared by the Commit toolbar and Project-view context menu. */
public final class GenerateCommitMessageAction extends AnAction implements DumbAware {
    private static final String NOTIFICATION_GROUP = "AI Git Committer";

    /** Keeps the toolbar button visible and enables it only when checked/selected changes exist. */
    @Override
    public void update(@NotNull AnActionEvent event) {
        boolean enabled = event.getProject() != null && SelectedChangeCollector.hasSelection(event);
        event.getPresentation().setVisible(event.getProject() != null);
        event.getPresentation().setEnabled(enabled);
    }

    /** Captures the current commit editor, then performs VCS, keychain, and network work in background. */
    @Override
    public void actionPerformed(@NotNull AnActionEvent event) {
        Project project = event.getProject();
        if (project == null) {
            return;
        }
        CommitMessageI commitMessageControl = event.getData(VcsDataKeys.COMMIT_MESSAGE_CONTROL);
        SelectedChangeCollector.Selection selection = SelectedChangeCollector.capture(event);
        ProgressManager.getInstance().run(new Task.Backgroundable(project, "Generating Commit Message", true) {
            @Override
            public void run(@NotNull ProgressIndicator indicator) {
                generateInBackground(project, selection, commitMessageControl, indicator);
            }
        });
    }

    /** Runs the linear generation pipeline and converts all failures into actionable notifications. */
    private static void generateInBackground(
            Project project,
            SelectedChangeCollector.Selection selection,
            CommitMessageI commitMessageControl,
            ProgressIndicator indicator
    ) {
        try {
            indicator.setText("Reading selected changes");
            List<FileChangeSnapshot> changes = SelectedChangeCollector.collect(selection, project);
            if (changes.isEmpty()) {
                notify(project, "No changed or unversioned text files were found in the selection", NotificationType.WARNING);
                return;
            }

            AiCommitSettings settingsService = AiCommitSettings.getInstance();
            AiCommitSettings.SettingsState settings = settingsService.getState();
            String apiKey = settingsService.loadApiKey();
            if (apiKey == null || apiKey.isBlank()) {
                openSettings(project, "Configure an API key before generating a commit message");
                return;
            }

            indicator.checkCanceled();
            indicator.setText("Calling " + settings.model);
            String systemPrompt = CommitPromptBuilder.systemPrompt(
                    settings.outputLanguage,
                    settings.conventionalCommits,
                    settings.additionalInstructions
            );
            String userPrompt = CommitPromptBuilder.userPrompt(changes, settings.maxContextChars);
            String commitMessage = new OpenAiCompatibleClient().generate(
                    settings,
                    apiKey,
                    systemPrompt,
                    userPrompt
            );
            deliverResult(project, commitMessageControl, commitMessage);
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            notify(project, "Commit message generation was interrupted", NotificationType.WARNING);
        } catch (Exception exception) {
            String detail = exception.getMessage() == null ? exception.getClass().getSimpleName() : exception.getMessage();
            notify(project, "Could not generate commit message: " + detail, NotificationType.ERROR);
        }
    }

    /** Writes directly into Commit UI when available; Project-view invocations copy a usable result. */
    private static void deliverResult(Project project, CommitMessageI control, String commitMessage) {
        ApplicationManager.getApplication().invokeLater(() -> {
            if (project.isDisposed()) {
                return;
            }
            if (control != null) {
                control.setCommitMessage(commitMessage);
                notify(project, "Commit message generated", NotificationType.INFORMATION);
            } else {
                CopyPasteManager.getInstance().setContents(new StringSelection(commitMessage));
                notify(project, "Commit message copied to the clipboard", NotificationType.INFORMATION);
            }
        });
    }

    /** Opens the plugin settings on the UI thread after explaining why generation cannot continue. */
    private static void openSettings(Project project, String message) {
        notify(project, message, NotificationType.WARNING);
        ApplicationManager.getApplication().invokeLater(() ->
                ShowSettingsUtil.getInstance().showSettingsDialog(project, AiCommitSettingsConfigurable.class)
        );
    }

    /** Centralizes user-facing reporting so background work never opens modal dialogs. */
    private static void notify(Project project, String content, NotificationType type) {
        NotificationGroupManager.getInstance()
                .getNotificationGroup(NOTIFICATION_GROUP)
                .createNotification(content, type)
                .notify(project);
    }

    /** CommitWorkflowUi is a Swing-backed data source and must be sampled on the UI thread. */
    @Override
    public @NotNull ActionUpdateThread getActionUpdateThread() {
        return ActionUpdateThread.EDT;
    }
}
