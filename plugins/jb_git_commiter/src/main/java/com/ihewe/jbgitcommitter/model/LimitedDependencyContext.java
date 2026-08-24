package com.ihewe.jbgitcommitter.model;

import java.util.List;

/** Bounded semantic relationships derived locally from GoLand PSI and reference indexes. */
public record LimitedDependencyContext(
        List<SymbolRelation> symbols,
        List<String> relevantPaths,
        String analysisNote
) {
    /** One changed declaration and the project-local evidence immediately surrounding it. */
    public record SymbolRelation(
            String packageName,
            String filePath,
            String symbol,
            String changeKind,
            List<String> dependencies,
            List<String> dependents,
            List<String> relatedTests
    ) {
    }
}
