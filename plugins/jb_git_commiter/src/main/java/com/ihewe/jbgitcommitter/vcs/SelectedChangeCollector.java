package com.ihewe.jbgitcommitter.vcs;

import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.intellij.openapi.actionSystem.AnActionEvent;
import com.intellij.openapi.actionSystem.CommonDataKeys;
import com.intellij.openapi.project.Project;
import com.intellij.openapi.project.ProjectUtil;
import com.intellij.openapi.vcs.FilePath;
import com.intellij.openapi.vcs.VcsDataKeys;
import com.intellij.openapi.vcs.VcsException;
import com.intellij.openapi.vcs.changes.Change;
import com.intellij.openapi.vcs.changes.ChangeListManager;
import com.intellij.openapi.vcs.changes.ContentRevision;
import com.intellij.openapi.vfs.VfsUtilCore;
import com.intellij.openapi.vfs.VirtualFile;
import org.jetbrains.annotations.NotNull;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collection;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

/** Resolves either Commit-tool-window changes or Project-view files into text snapshots. */
public final class SelectedChangeCollector {
    private SelectedChangeCollector() {
    }

    /**
     * Prefers explicitly selected VCS changes. Project-view selections are resolved through the
     * change-list manager, including changed descendants when a directory is selected.
     */
    public static List<FileChangeSnapshot> collect(@NotNull Selection selection, @NotNull Project project)
            throws VcsException, IOException {
        Set<Change> changes = new LinkedHashSet<>();
        List<VirtualFile> unversionedFiles = new ArrayList<>();
        if (selection.changes().length > 0) {
            changes.addAll(List.of(selection.changes()));
        } else {
            collectFromVirtualFiles(selection.files(), project, changes, unversionedFiles);
        }

        List<FileChangeSnapshot> snapshots = new ArrayList<>(changes.size() + unversionedFiles.size());
        for (Change change : changes) {
            snapshots.add(toSnapshot(project, change));
        }
        for (VirtualFile file : unversionedFiles) {
            snapshots.add(new FileChangeSnapshot(relativePath(project, file.getPath()), "NEW", null, readText(file)));
        }
        return snapshots;
    }

    /** Performs a cheap data-context check used to hide the action when nothing can be analyzed. */
    public static boolean hasSelection(@NotNull AnActionEvent event) {
        return !capture(event).isEmpty();
    }

    /** Copies action data immediately so a short-lived DataContext is never accessed by a worker thread. */
    public static Selection capture(@NotNull AnActionEvent event) {
        Change[] selected = firstNonEmpty(
                event.getData(VcsDataKeys.SELECTED_CHANGES),
                event.getData(VcsDataKeys.CHANGES)
        );
        VirtualFile[] files = event.getData(CommonDataKeys.VIRTUAL_FILE_ARRAY);
        return new Selection(
                selected == null ? new Change[0] : selected.clone(),
                files == null ? new VirtualFile[0] : files.clone()
        );
    }

    /** Expands selected directories and keeps unversioned files as complete after-images. */
    private static void collectFromVirtualFiles(
            VirtualFile[] files,
            Project project,
            Set<Change> changes,
            List<VirtualFile> unversionedFiles
    ) {
        ChangeListManager manager = ChangeListManager.getInstance(project);
        for (VirtualFile file : files) {
            if (file.isDirectory()) {
                changes.addAll(manager.getChangesIn(file));
                collectUnversionedDescendants(manager.getUnversionedFilesPaths(), file, unversionedFiles);
                continue;
            }
            Change change = manager.getChange(file);
            if (change != null) {
                changes.add(change);
            } else if (manager.isUnversioned(file)) {
                unversionedFiles.add(file);
            }
        }
    }

    /** Includes only unversioned descendants of the explicitly selected directory. */
    private static void collectUnversionedDescendants(
            Collection<FilePath> allUnversioned,
            VirtualFile directory,
            List<VirtualFile> destination
    ) {
        for (FilePath path : allUnversioned) {
            VirtualFile file = path.getVirtualFile();
            if (file != null && VfsUtilCore.isAncestor(directory, file, false)) {
                destination.add(file);
            }
        }
    }

    /** Reads before/after revisions; null content intentionally represents deletion or binary data. */
    private static FileChangeSnapshot toSnapshot(Project project, Change change) throws VcsException {
        ContentRevision beforeRevision = change.getBeforeRevision();
        ContentRevision afterRevision = change.getAfterRevision();
        String path = afterRevision != null
                ? afterRevision.getFile().getPath()
                : beforeRevision != null ? beforeRevision.getFile().getPath() : "unknown";
        return new FileChangeSnapshot(
                relativePath(project, path),
                change.getType().name(),
                contentOf(beforeRevision),
                contentOf(afterRevision)
        );
    }

    /** ContentRevision returns null for binary revisions, which the prompt marks without decoding bytes. */
    private static String contentOf(ContentRevision revision) throws VcsException {
        return revision == null ? null : revision.getContent();
    }

    /** Refuses to decode binary-looking files and uses the VFS charset for ordinary source files. */
    private static String readText(VirtualFile file) throws IOException {
        if (file.getFileType().isBinary()) {
            return null;
        }
        return new String(file.contentsToByteArray(), file.getCharset() == null ? StandardCharsets.UTF_8 : file.getCharset());
    }

    /** Produces repository-relative paths when a project base directory is available. */
    private static String relativePath(Project project, String absolutePath) {
        VirtualFile baseDir = ProjectUtil.guessProjectDir(project);
        if (baseDir == null) {
            return absolutePath;
        }
        String basePath = baseDir.getPath();
        return absolutePath.startsWith(basePath + "/")
                ? absolutePath.substring(basePath.length() + 1)
                : absolutePath;
    }

    /** Keeps selection precedence readable and avoids treating empty data-key arrays as a selection. */
    private static Change[] firstNonEmpty(Change[] primary, Change[] fallback) {
        if (primary != null && primary.length > 0) {
            return primary;
        }
        return fallback != null && fallback.length > 0 ? fallback : null;
    }

    /** Immutable copy of the user selection that is safe to pass to a background task. */
    public record Selection(Change[] changes, VirtualFile[] files) {
        public boolean isEmpty() {
            return changes.length == 0 && files.length == 0;
        }
    }
}
