package com.ihewe.jbgitcommitter.context;

import com.ihewe.jbgitcommitter.model.FileChangeSnapshot;
import com.ihewe.jbgitcommitter.settings.AiCommitSettings;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class FileContextPolicyTest {
    /** Generated evidence is removed when a selected source file can explain the same change. */
    @Test
    void filtersGeneratedFilesWhenPrimaryChangesExist() {
        List<FileChangeSnapshot> selected = FileContextPolicy.select(
                List.of(change("api/order.proto"), change("api/order.pb.go"), change("service/order.go")),
                AiCommitSettings.DEFAULT_GENERATED_PATTERNS,
                AiCommitSettings.DEFAULT_SOURCE_GENERATED_RULES
        );

        assertEquals(List.of("api/order.proto", "service/order.go"), paths(selected));
    }

    /** An all-generated selection remains usable instead of producing an empty model context. */
    @Test
    void preservesGeneratedFilesWhenTheyAreTheOnlySelection() {
        List<FileChangeSnapshot> changes = List.of(change("api/order.pb.go"), change("client/user_pb2.py"));

        List<FileChangeSnapshot> selected = FileContextPolicy.select(
                changes,
                AiCommitSettings.DEFAULT_GENERATED_PATTERNS,
                AiCommitSettings.DEFAULT_SOURCE_GENERATED_RULES
        );

        assertEquals(changes, selected);
    }

    /** Custom mappings collapse project-specific derived names without hard-coding a language. */
    @Test
    void appliesCustomSourceToGeneratedRule() {
        List<FileChangeSnapshot> selected = FileContextPolicy.select(
                List.of(change("schema/domain.idl"), change("runtime/domain.client")),
                "",
                "**/*.idl => **/*.client"
        );

        assertEquals(List.of("schema/domain.idl"), paths(selected));
    }

    /** Invalid mapping syntax is rejected instead of being silently ignored. */
    @Test
    void rejectsMalformedRule() {
        assertThrows(IllegalArgumentException.class,
                () -> FileContextPolicy.select(List.of(), "", "**/*.proto -> **/*.pb.go"));
    }

    /** The Settings table model round-trips through the persisted line schema without ambiguity. */
    @Test
    void roundTripsMappingRows() {
        List<FileContextPolicy.SourceGeneratedMapping> mappings = List.of(
                new FileContextPolicy.SourceGeneratedMapping("**/*.proto", "**/*.pb.go, **/*_pb2.py"),
                new FileContextPolicy.SourceGeneratedMapping("**/*.graphql", "**/*.generated.*")
        );

        String schema = FileContextPolicy.formatMappings(mappings);

        assertEquals(mappings, FileContextPolicy.parseMappings(schema));
    }

    /** Keeps fixture creation focused on path classification rather than diff contents. */
    private static FileChangeSnapshot change(String path) {
        return new FileChangeSnapshot(path, "MODIFICATION", "before", "after");
    }

    /** Extracts paths for concise ordering and filtering assertions. */
    private static List<String> paths(List<FileChangeSnapshot> changes) {
        return changes.stream().map(FileChangeSnapshot::path).toList();
    }
}
