package com.ihewe.jbgitcommitter.model;

import org.jetbrains.annotations.Nullable;

/** Immutable, API-independent representation of one selected text-file change. */
public record FileChangeSnapshot(String path, String changeType, @Nullable String before, @Nullable String after) {
}
