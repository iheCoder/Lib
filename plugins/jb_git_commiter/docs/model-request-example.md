# Model request example

Every generation sends two messages. The system message contains the selected Default/Custom
Prompt plus the explicit language and optional length settings. The user message always starts
with Limited Dependency Relations, followed by the selected files' complete before/after text.

The OpenAI example below is shortened only for documentation. The plugin applies the configured
global context-character budget to the real user message. DeepSeek uses the same message shape;
Anthropic moves the system prompt to its top-level `system` field.

```json
{
  "model": "gpt-5.6-terra",
  "messages": [
    {
      "role": "system",
      "content": "Generate a concise git commit message for the provided changes.\nCapture the primary intent or behavior change, not a list of individual edits.\nPrefer what the change accomplishes over how it was implemented.\nBe specific when the evidence supports it, but do not invent intent that is not present in the changes.\nIf several changes support the same goal, describe that shared goal rather than enumerating them.\nIf the changes contain multiple unrelated goals, summarize the most important ones concisely without forcing them into a single invented theme.\nReturn only the commit message. No explanation, prefix, quotation marks, or markdown.\n\nOutput constraints:\n- Write the commit message in English."
    },
    {
      "role": "user",
      "content": "Generate one commit message for these selected changes:\n\n## Limited Dependency Relations\nAnalysis: Relations are project-local and capped at 12 symbols and 6 edges per relation kind.\nRelevant project paths:\n- internal/order/service.go\n- internal/inventory/service.go\n- internal/order/handler.go\n- internal/order/service_test.go\nChanged symbols:\n- MODIFIED example.com/shop/internal/order.(*Service).CreateOrder (package: order, file: internal/order/service.go)\n  dependencies: example.com/shop/internal/inventory.(*Service).Check @ internal/inventory/service.go\n  dependents: example.com/shop/internal/order.(*Handler).CreateOrder @ internal/order/handler.go\n  related tests: example.com/shop/internal/order.TestCreateOrder @ internal/order/service_test.go\n\n=== internal/order/service.go [MODIFICATION] ===\n--- BEFORE ---\nfunc (s *Service) CreateOrder(...) error {\n    return s.repo.Create(...)\n}\n--- AFTER ---\nfunc (s *Service) CreateOrder(...) error {\n    if err := s.inventory.Check(...); err != nil {\n        return ErrInsufficientInventory\n    }\n    return s.repo.Create(...)\n}\n"
    }
  ],
  "response_format": {
    "type": "json_schema",
    "json_schema": {
      "name": "commit_message",
      "strict": true,
      "schema": {
        "type": "object",
        "properties": {
          "message": {"type": "string", "minLength": 1}
        },
        "required": ["message"],
        "additionalProperties": false
      }
    }
  }
}
```

The plugin deliberately does not send the complete repository tree. `Relevant project paths` is a
sparse project slice containing selected files and project-local files discovered through PSI
dependencies, dependents, and test references. Tool metadata, external dependencies, vendor,
node_modules, and generated directories are excluded from discovered relations.
