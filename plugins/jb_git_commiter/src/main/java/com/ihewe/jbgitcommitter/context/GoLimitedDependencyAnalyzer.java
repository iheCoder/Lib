package com.ihewe.jbgitcommitter.context;

import com.goide.GoFileType;
import com.goide.psi.GoFile;
import com.goide.psi.GoFunctionOrMethodDeclaration;
import com.goide.psi.GoNamedElement;
import com.goide.psi.GoReferenceExpression;
import com.goide.psi.GoTypeSpec;
import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.ihewe.jbgitcommitter.model.LimitedDependencyContext;
import com.ihewe.jbgitcommitter.model.LimitedDependencyContext.SymbolRelation;
import com.intellij.openapi.application.ReadAction;
import com.intellij.openapi.project.DumbService;
import com.intellij.openapi.project.Project;
import com.intellij.openapi.project.ProjectUtil;
import com.intellij.openapi.roots.GeneratedSourcesFilter;
import com.intellij.openapi.vfs.VirtualFile;
import com.intellij.psi.PsiElement;
import com.intellij.psi.PsiFile;
import com.intellij.psi.PsiFileFactory;
import com.intellij.psi.PsiManager;
import com.intellij.psi.PsiReference;
import com.intellij.psi.search.GlobalSearchScope;
import com.intellij.psi.search.searches.ReferencesSearch;
import com.intellij.psi.util.PsiTreeUtil;
import org.jetbrains.annotations.NotNull;

import java.util.ArrayList;
import java.util.Collection;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;

/** Builds a small, deterministic Go semantic neighborhood for every generation request. */
public final class GoLimitedDependencyAnalyzer {
    private static final int MAX_CHANGED_SYMBOLS = 12;
    private static final int MAX_RELATIONS_PER_KIND = 6;
    private static final int MAX_REFERENCE_CANDIDATES = 60;
    private static final int MAX_RELEVANT_PATHS = 30;

    private GoLimitedDependencyAnalyzer() {
    }

    /** Runs all PSI/index access under one read action and always returns a renderable context. */
    public static LimitedDependencyContext analyze(
            @NotNull Project project,
            @NotNull List<FileChangeSnapshot> changes
    ) {
        return ReadAction.computeCancellable(() -> analyzeInReadAction(project, changes));
    }

    /** Extracts changed declarations first, then enriches current declarations with bounded edges. */
    private static LimitedDependencyContext analyzeInReadAction(
            Project project,
            List<FileChangeSnapshot> changes
    ) {
        boolean indexesAvailable = !DumbService.isDumb(project);
        List<SymbolRelation> relations = new ArrayList<>();
        Set<String> relevantPaths = new LinkedHashSet<>();
        for (FileChangeSnapshot change : changes) {
            relevantPaths.add(change.path());
            if (!change.path().endsWith(".go") || relations.size() >= MAX_CHANGED_SYMBOLS) {
                continue;
            }
            analyzeGoChange(project, change, indexesAvailable, relations, relevantPaths);
        }
        String note = analysisNote(relations, indexesAvailable);
        return new LimitedDependencyContext(
                List.copyOf(relations),
                relevantPaths.stream().limit(MAX_RELEVANT_PATHS).toList(),
                note
        );
    }

    /** Compares declaration text across revisions without asking an LLM to infer changed symbols. */
    private static void analyzeGoChange(
            Project project,
            FileChangeSnapshot change,
            boolean indexesAvailable,
            List<SymbolRelation> destination,
            Set<String> relevantPaths
    ) {
        GoFile beforeFile = parseGoFile(project, change.path(), change.before());
        GoFile afterFile = parseGoFile(project, change.path(), change.after());
        Map<String, DeclarationSnapshot> before = declarations(beforeFile);
        Map<String, DeclarationSnapshot> after = declarations(afterFile);
        GoFile physicalFile = physicalGoFile(project, change.path());
        Map<String, GoNamedElement> physicalDeclarations = declarationElements(physicalFile);

        Set<String> keys = new LinkedHashSet<>(before.keySet());
        keys.addAll(after.keySet());
        for (String key : keys) {
            if (destination.size() >= MAX_CHANGED_SYMBOLS || Objects.equals(before.get(key), after.get(key))) {
                continue;
            }
            destination.add(buildRelation(
                    project, change.path(), before.get(key), after.get(key), physicalDeclarations.get(key),
                    indexesAvailable, relevantPaths
            ));
        }
    }

    /** Creates a synthetic Go PSI file for either VCS revision; absent/binary revisions stay null. */
    private static GoFile parseGoFile(Project project, String path, String content) {
        if (content == null) {
            return null;
        }
        String name = path.substring(path.lastIndexOf('/') + 1);
        PsiFile file = PsiFileFactory.getInstance(project).createFileFromText(name, GoFileType.INSTANCE, content);
        return file instanceof GoFile goFile ? goFile : null;
    }

    /** Resolves the current physical PSI file so references can use GoLand's project indexes. */
    private static GoFile physicalGoFile(Project project, String relativePath) {
        VirtualFile root = ProjectUtil.guessProjectDir(project);
        VirtualFile file = root == null ? null : root.findFileByRelativePath(relativePath);
        if (file == null || GeneratedSourcesFilter.isGeneratedSourceByAnyFilter(file, project)) {
            return null;
        }
        PsiFile psiFile = PsiManager.getInstance(project).findFile(file);
        return psiFile instanceof GoFile goFile ? goFile : null;
    }

    /** Captures declaration bodies by stable Go qualified name for before/after comparison. */
    private static Map<String, DeclarationSnapshot> declarations(GoFile file) {
        Map<String, DeclarationSnapshot> snapshots = new LinkedHashMap<>();
        for (GoNamedElement element : topLevelDeclarations(file)) {
            String key = declarationKey(element);
            snapshots.put(key, new DeclarationSnapshot(file.getPackageName(), key, element.getText()));
        }
        return snapshots;
    }

    /** Maps current declarations to their physical PSI elements for dependency searches. */
    private static Map<String, GoNamedElement> declarationElements(GoFile file) {
        Map<String, GoNamedElement> elements = new LinkedHashMap<>();
        for (GoNamedElement element : topLevelDeclarations(file)) {
            elements.put(declarationKey(element), element);
        }
        return elements;
    }

    /** Limits semantic units to functions, methods, and types—the useful commit-message level. */
    private static List<GoNamedElement> topLevelDeclarations(GoFile file) {
        if (file == null) {
            return List.of();
        }
        List<GoNamedElement> declarations = new ArrayList<>();
        declarations.addAll(file.getFunctions());
        declarations.addAll(file.getMethods());
        declarations.addAll(file.getTypes());
        declarations.sort(Comparator.comparingInt(PsiElement::getTextOffset));
        return declarations;
    }

    /** Qualified names distinguish methods with identical names on different receiver types. */
    private static String declarationKey(GoNamedElement element) {
        String qualifiedName = element.getQualifiedName();
        return qualifiedName == null || qualifiedName.isBlank() ? element.getName() : qualifiedName;
    }

    /** Enriches a changed declaration only with project-local, capped semantic relationships. */
    private static SymbolRelation buildRelation(
            Project project,
            String filePath,
            DeclarationSnapshot before,
            DeclarationSnapshot after,
            GoNamedElement current,
            boolean indexesAvailable,
            Set<String> relevantPaths
    ) {
        Set<String> dependencies = indexesAvailable ? dependencies(project, current, relevantPaths) : Set.of();
        ReferenceRelations references = indexesAvailable
                ? dependents(project, current, relevantPaths)
                : ReferenceRelations.EMPTY;
        DeclarationSnapshot visible = after == null ? before : after;
        return new SymbolRelation(
                visible.packageName(), filePath, visible.symbol(), changeKind(before, after),
                List.copyOf(dependencies), List.copyOf(references.production()), List.copyOf(references.tests())
        );
    }

    /** Resolves references used inside the changed declaration to project-local top-level symbols. */
    private static Set<String> dependencies(Project project, GoNamedElement current, Set<String> relevantPaths) {
        if (current == null) {
            return Set.of();
        }
        Set<String> dependencies = new LinkedHashSet<>();
        Collection<GoReferenceExpression> expressions = PsiTreeUtil.findChildrenOfType(
                current,
                GoReferenceExpression.class
        );
        for (GoReferenceExpression expression : expressions) {
            addResolvedRelation(project, current, expression.resolve(), dependencies, relevantPaths);
            if (dependencies.size() >= MAX_RELATIONS_PER_KIND) {
                break;
            }
        }
        return dependencies;
    }

    /** Searches direct callers/usages and splits tests from production dependents. */
    private static ReferenceRelations dependents(
            Project project,
            GoNamedElement current,
            Set<String> relevantPaths
    ) {
        if (current == null) {
            return ReferenceRelations.EMPTY;
        }
        Set<String> production = new LinkedHashSet<>();
        Set<String> tests = new LinkedHashSet<>();
        int[] visited = {0};
        ReferencesSearch.search(current, GlobalSearchScope.projectScope(project)).forEach(reference -> {
            visited[0]++;
            addDependent(project, reference, production, tests, relevantPaths);
            boolean needsMoreRelations = production.size() < MAX_RELATIONS_PER_KIND
                    || tests.size() < MAX_RELATIONS_PER_KIND;
            return visited[0] < MAX_REFERENCE_CANDIDATES && needsMoreRelations;
        });
        return new ReferenceRelations(production, tests);
    }

    /** Converts a resolved PSI element to the nearest useful project declaration. */
    private static void addResolvedRelation(
            Project project,
            GoNamedElement source,
            PsiElement resolved,
            Set<String> destination,
            Set<String> relevantPaths
    ) {
        GoNamedElement target = enclosingDeclaration(resolved);
        String path = target == null ? null : projectRelativePath(project, target.getContainingFile());
        if (target == null || target == source || path == null || isExcludedPath(path)
                || isGeneratedPsiFile(project, target.getContainingFile())) {
            return;
        }
        destination.add(declarationKey(target) + " @ " + path);
        relevantPaths.add(path);
    }

    /** Records the enclosing caller and classifies `_test.go` usages as related tests. */
    private static void addDependent(
            Project project,
            PsiReference reference,
            Set<String> production,
            Set<String> tests,
            Set<String> relevantPaths
    ) {
        PsiElement usage = reference.getElement();
        GoNamedElement caller = enclosingDeclaration(usage);
        String path = projectRelativePath(project, usage.getContainingFile());
        if (path == null || isExcludedPath(path) || isGeneratedPsiFile(project, usage.getContainingFile())) {
            return;
        }
        String label = (caller == null ? "file usage" : declarationKey(caller)) + " @ " + path;
        Set<String> destination = path.endsWith("_test.go") ? tests : production;
        if (destination.size() < MAX_RELATIONS_PER_KIND) {
            destination.add(label);
            relevantPaths.add(path);
        }
    }

    /** Walks from references/fields/local nodes to a function, method, or type declaration. */
    private static GoNamedElement enclosingDeclaration(PsiElement element) {
        if (element == null) {
            return null;
        }
        if (element instanceof GoFunctionOrMethodDeclaration function) {
            return function;
        }
        if (element instanceof GoTypeSpec type) {
            return type;
        }
        GoFunctionOrMethodDeclaration function = PsiTreeUtil.getParentOfType(
                element,
                GoFunctionOrMethodDeclaration.class,
                false
        );
        return function != null ? function : PsiTreeUtil.getParentOfType(element, GoTypeSpec.class, false);
    }

    /** Keeps references inside the repository and expresses them with repository-relative paths. */
    private static String projectRelativePath(Project project, PsiFile file) {
        VirtualFile root = ProjectUtil.guessProjectDir(project);
        VirtualFile virtualFile = file == null ? null : file.getVirtualFile();
        if (root == null || virtualFile == null) {
            return null;
        }
        String rootPath = root.getPath();
        String path = virtualFile.getPath();
        return path.startsWith(rootPath + "/") ? path.substring(rootPath.length() + 1) : null;
    }

    /** Reuses GoLand's language/build metadata before accepting a discovered relation edge. */
    private static boolean isGeneratedPsiFile(Project project, PsiFile file) {
        VirtualFile virtualFile = file == null ? null : file.getVirtualFile();
        return virtualFile != null && GeneratedSourcesFilter.isGeneratedSourceByAnyFilter(virtualFile, project);
    }

    /** Excludes tool metadata, dependency copies, and generated directories from semantic edges. */
    private static boolean isExcludedPath(String path) {
        String normalized = "/" + path.replace('\\', '/') + "/";
        return normalized.contains("/.git/")
                || normalized.contains("/.idea/")
                || normalized.contains("/.cursor/")
                || normalized.contains("/vendor/")
                || normalized.contains("/node_modules/")
                || normalized.contains("/generated/");
    }

    /** Makes add/remove/modify status explicit without leaking raw declaration bodies twice. */
    private static String changeKind(DeclarationSnapshot before, DeclarationSnapshot after) {
        if (before == null) {
            return "ADDED";
        }
        return after == null ? "REMOVED" : "MODIFIED";
    }

    /** Explains empty/degraded results honestly while keeping the section present in every request. */
    private static String analysisNote(List<SymbolRelation> relations, boolean indexesAvailable) {
        if (!indexesAvailable) {
            return "GoLand indexes were unavailable; package and changed-symbol analysis is included without reference edges.";
        }
        return relations.isEmpty()
                ? "No changed Go function, method, or type declaration was detected."
                : "Relations are project-local and capped at 12 symbols and 6 edges per relation kind.";
    }

    /** Declaration comparison value; text participates in equality but is never rendered directly. */
    private record DeclarationSnapshot(String packageName, String symbol, String text) {
    }

    /** Separates production dependents from test evidence in the final prompt. */
    private record ReferenceRelations(Set<String> production, Set<String> tests) {
        private static final ReferenceRelations EMPTY = new ReferenceRelations(Set.of(), Set.of());
    }
}
