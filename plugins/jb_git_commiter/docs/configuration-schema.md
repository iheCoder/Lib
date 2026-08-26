# Configuration schema

AI Git Committer stores the following non-secret application settings in JetBrains persistent
state. API keys remain outside this schema in PasswordSafe.

```json
{
  "schemaVersion": 4,
  "provider": "openai",
  "endpoint": "https://api.openai.com/v1/chat/completions",
  "model": "gpt-5.6-terra",
  "customPrompt": "",
  "outputLanguage": "English",
  "messageMaxCharacters": 0,
  "structuredOutput": true,
  "maxContextChars": 60000,
  "requestTimeoutSeconds": 60,
  "generatedPatterns": "**/*.generated.*\n**/*.pb.go\n...",
  "sourceGeneratedRules": "**/*.proto => **/*.pb.go, **/*_pb2.py\n..."
}
```

`provider` is one of `openai`, `anthropic`, or `deepseek`. Selecting a provider in Settings fills
its official endpoint and curated model list. Both endpoint and model remain editable. PasswordSafe
uses a separate credential entry for each provider; pre-v0.6 OpenAI credentials are read through a
legacy fallback and are never copied to another provider.

## Generated file schema

`generatedPatterns` is a newline-separated list of repository-relative globs. Blank lines and
lines starting with `#` are ignored. The supported glob subset is:

- `**` matches across directories.
- `*` matches within one path segment.
- `?` matches one non-separator character.

When primary and generated files are selected together, generated files are omitted from model
context. When every selected file is generated, the complete generated selection is retained.

## Source → Generated schema

Each non-comment line has one source glob and one or more comma-separated generated globs:

```text
source glob => generated glob, generated glob
```

Example:

```text
**/*.proto => **/*.pb.go, **/*_pb2.py, **/*_pb2.pyi
**/*.graphql => **/*.generated.*, **/__generated__/**
```

A rule becomes active when at least one selected file matches its source glob. Its matching target
files are then treated as generated. Invalid lines are rejected by Apply and Test API.

## Model output schema

The immutable Default Prompt is:

```text
Generate a concise git commit message for the provided changes.
Capture the primary intent or behavior change, not a list of individual edits.
Prefer what the change accomplishes over how it was implemented.
Be specific when the evidence supports it, but do not invent intent that is not present in the changes.
If several changes support the same goal, describe that shared goal rather than enumerating them.
If the changes contain multiple unrelated goals, summarize the most important ones concisely without forcing them into a single invented theme.
Return only the commit message. No explanation, prefix, quotation marks, or markdown.
```

With `structuredOutput` enabled, OpenAI and DeepSeek requests contain this strict Chat Completions
`response_format` contract:

```json
{
  "type": "json_schema",
  "json_schema": {
    "name": "commit_message",
    "strict": true,
    "schema": {
      "type": "object",
      "properties": {
        "message": {
          "type": "string",
          "minLength": 1
        }
      },
      "required": ["message"],
      "additionalProperties": false
    }
  }
}
```

Anthropic receives the same inner schema through its native Messages contract:

```json
{
  "output_config": {
    "format": {
      "type": "json_schema",
      "schema": {
        "type": "object",
        "properties": {"message": {"type": "string", "minLength": 1}},
        "required": ["message"],
        "additionalProperties": false
      }
    }
  }
}
```

The built-in Default Prompt is used only when `customPrompt` is blank. A non-blank `customPrompt`
replaces it completely. The two visible output settings are then appended to either choice:

```text
Output constraints:
- Write the commit message in English.
- Keep the complete commit message within 80 Unicode characters.
```

Language defaults to `English` and may be selected or entered freely. `messageMaxCharacters`
defaults to `0`, meaning unlimited; the length line is omitted in that case. A positive value is
also added to the JSON Schema as `message.maxLength`. This keeps prompt and provider constraints
consistent without truncating the returned text locally.

Custom gateways without JSON Schema support can disable `structuredOutput`. After extracting `message`,
the plugin only rejects blank output. It does not force one line, strip Markdown, or truncate text.
