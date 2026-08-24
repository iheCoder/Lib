package com.ihewe.jbgitcommitter.context;

import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import org.jetbrains.annotations.NotNull;

import java.util.ArrayList;
import java.util.List;
import java.util.regex.Pattern;

/** Applies configurable, language-independent source/derived-file context rules. */
public final class FileContextPolicy {
    private FileContextPolicy() {
    }

    /**
     * Removes generated files when primary evidence exists, but preserves the original selection
     * when every selected file is generated so the model is never left without evidence.
     */
    public static List<FileChangeSnapshot> select(
            @NotNull List<FileChangeSnapshot> changes,
            String generatedPatternSchema,
            String sourceGeneratedRuleSchema
    ) {
        List<PathPattern> generatedPatterns = parsePatterns(generatedPatternSchema);
        List<SourceGeneratedRule> activeRules = compileRules(parseMappings(sourceGeneratedRuleSchema)).stream()
                .filter(rule -> changes.stream().anyMatch(change -> rule.source().matches(change.path())))
                .toList();
        List<FileChangeSnapshot> primaryChanges = changes.stream()
                .filter(change -> !isGenerated(change.path(), generatedPatterns, activeRules))
                .toList();
        return primaryChanges.isEmpty() ? List.copyOf(changes) : primaryChanges;
    }

    /** Treats global generated patterns and active source-to-generated targets uniformly. */
    private static boolean isGenerated(
            String path,
            List<PathPattern> generatedPatterns,
            List<SourceGeneratedRule> activeRules
    ) {
        if (generatedPatterns.stream().anyMatch(pattern -> pattern.matches(path))) {
            return true;
        }
        return activeRules.stream().flatMap(rule -> rule.generated().stream())
                .anyMatch(pattern -> pattern.matches(path));
    }

    /** Parses the one-glob-per-line schema while allowing comments and blank lines. */
    private static List<PathPattern> parsePatterns(String schema) {
        if (schema == null || schema.isBlank()) {
            return List.of();
        }
        return schema.lines()
                .map(String::trim)
                .filter(line -> !line.isBlank() && !line.startsWith("#"))
                .map(PathPattern::new)
                .toList();
    }

    /** Parses the persisted line schema into rows suitable for both validation and the Settings table. */
    public static List<SourceGeneratedMapping> parseMappings(String schema) {
        if (schema == null || schema.isBlank()) {
            return List.of();
        }
        List<SourceGeneratedMapping> mappings = new ArrayList<>();
        schema.lines().map(String::trim)
                .filter(line -> !line.isBlank() && !line.startsWith("#"))
                .forEach(line -> mappings.add(parseMapping(line)));
        return mappings;
    }

    /** Serializes table rows back to the stable `source => generated, generated` state format. */
    public static String formatMappings(List<SourceGeneratedMapping> mappings) {
        return mappings.stream()
                .filter(mapping -> !mapping.sourceGlob().isBlank() || !mapping.generatedGlobs().isBlank())
                .map(mapping -> validateMapping(mapping).sourceGlob() + " => " + mapping.generatedGlobs())
                .reduce((left, right) -> left + "\n" + right)
                .orElse("");
    }

    /** Requires one source and at least one generated target so configuration errors fail early. */
    private static SourceGeneratedMapping parseMapping(String line) {
        String[] sides = line.split("=>", 2);
        if (sides.length != 2 || sides[0].isBlank() || sides[1].isBlank()) {
            throw new IllegalArgumentException("Invalid Source → Generated rule: " + line);
        }
        return validateMapping(new SourceGeneratedMapping(sides[0].trim(), sides[1].trim()));
    }

    /** Compiles user-facing mapping rows once before selection classification. */
    private static List<SourceGeneratedRule> compileRules(List<SourceGeneratedMapping> mappings) {
        return mappings.stream().map(mapping -> {
            List<PathPattern> targets = List.of(mapping.generatedGlobs().split(",")).stream()
                .map(String::trim)
                .filter(target -> !target.isBlank())
                .map(PathPattern::new)
                .toList();
            return new SourceGeneratedRule(new PathPattern(mapping.sourceGlob()), targets);
        }).toList();
    }

    /** Validates table cells without changing their user-visible glob text. */
    private static SourceGeneratedMapping validateMapping(SourceGeneratedMapping mapping) {
        if (mapping.sourceGlob().isBlank() || mapping.generatedGlobs().isBlank()) {
            throw new IllegalArgumentException("Source and Generated globs are both required");
        }
        boolean hasTarget = List.of(mapping.generatedGlobs().split(",")).stream()
                .anyMatch(target -> !target.isBlank());
        if (!hasTarget) {
            throw new IllegalArgumentException("At least one Generated glob is required");
        }
        return mapping;
    }

    /** Converts the documented glob subset to a repository-path regular expression. */
    private static Pattern compileGlob(String glob) {
        String normalized = glob.replace('\\', '/');
        StringBuilder regex = new StringBuilder("^");
        for (int index = 0; index < normalized.length(); index++) {
            char current = normalized.charAt(index);
            if (current == '*' && index + 1 < normalized.length() && normalized.charAt(index + 1) == '*') {
                boolean followedBySlash = index + 2 < normalized.length() && normalized.charAt(index + 2) == '/';
                regex.append(followedBySlash ? "(?:.*/)?" : ".*");
                index += followedBySlash ? 2 : 1;
            } else if (current == '*') {
                regex.append("[^/]*");
            } else if (current == '?') {
                regex.append("[^/]");
            } else {
                regex.append(Pattern.quote(String.valueOf(current)));
            }
        }
        return Pattern.compile(regex.append('$').toString());
    }

    /** Compiled path predicate keeps matching logic independent of VFS and host separators. */
    private record PathPattern(Pattern pattern) {
        private PathPattern(String glob) {
            this(compileGlob(glob));
        }

        private boolean matches(String path) {
            return pattern.matcher(path.replace('\\', '/')).matches();
        }
    }

    /** One configured source pattern can activate several generated-target patterns. */
    private record SourceGeneratedRule(PathPattern source, List<PathPattern> generated) {
    }

    /** Editable Settings-table row; generated targets remain comma-separated in one cell. */
    public record SourceGeneratedMapping(String sourceGlob, String generatedGlobs) {
        public SourceGeneratedMapping {
            sourceGlob = sourceGlob == null ? "" : sourceGlob.trim();
            generatedGlobs = generatedGlobs == null ? "" : generatedGlobs.trim();
        }
    }
}
