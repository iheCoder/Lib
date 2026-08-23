# AI Git Committer for GoLand

在 GoLand 的 Commit 工具窗口或 Project 视图选择若干变更文件，通过你自己的
OpenAI-compatible Chat Completions API 生成一条 commit message。

## 功能

- Commit 工具窗口：选择变更后点击 **Generate Commit Message with AI**，结果直接写入提交信息框。
- Commit 工具窗口的闪光勾选图标会始终显示；没有勾选文件时禁用，勾选后即可生成。
- Project 视图：多选文件后右键执行同一动作，结果会复制到剪贴板。
- 同时支持已跟踪变更、删除文件和未跟踪文本文件；目录选择会展开其下的变更。
- generated 文件默认从混合选择中移除；如果选中的全是 generated 文件，则保留全部作为回退上下文。
- generated glob 与 `Source → Generated` 折叠规则均可配置；映射规则通过 Source/Generated 两列表格维护。
- Custom Prompt 会完整覆盖 Default Prompt；插件不会追加隐藏规则，也不会对返回结果执行单行化或截断。
- 语言默认 English，可从常见语言中选择或直接输入其他语言；长度默认 `0`（不限制），也可设置正整数。
- 语言和可选长度会作为设置页中明确可见的 Output constraints 拼接到 Default/Custom Prompt 后。
- 每次生成都会使用 GoLand PSI 和引用索引附加 Limited Dependency Relations：changed symbols、package、项目内 dependencies、dependents、related tests 与相关路径切片。
- Settings 页面提供 **Test API**，使用当前未保存的 URL、模型和 API Key 发起最小请求并展示成功/失败原因。
- API Key 使用 JetBrains PasswordSafe（macOS 上通常落入 Keychain），不会写进插件 XML 配置。

## 本地开发

本项目使用 IntelliJ Platform Gradle Plugin 2.x。GoLand 2026.2 自带 JBR 25，执行构建时可直接复用：

```bash
export JAVA_HOME="/Applications/GoLand.app/Contents/jbr/Contents/Home"
./gradlew -PlocalIdePath="/Applications/GoLand.app" test buildPlugin
```

开发运行：

```bash
./gradlew -PlocalIdePath="/Applications/GoLand.app" runIde
```

安装包生成在 `build/distributions/`。在 GoLand 中打开
**Settings | Plugins | ⚙ | Install Plugin from Disk...**，选择该 ZIP。

## 使用

1. 打开 **Settings | Tools | AI Git Committer**。
2. 配置完整的 Chat Completions URL、模型和 API Key，点击 **Test API** 验证连接；成功后点击 Apply 保存。
3. 在 Commit 工具窗口勾选需要提交的文件，点击提交信息工具栏中的闪光勾选图标。
4. 检查生成结果后再手动提交；插件不会自动执行 `git commit`。

## API 协议

请求采用常见的 OpenAI Chat Completions 结构，并默认附带严格的 `response_format.json_schema`：

```json
{
  "model": "your-model",
  "temperature": 0.2,
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "response_format": {
    "type": "json_schema",
    "json_schema": {
      "name": "commit_message",
      "strict": true,
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

完整配置、glob 和 `Source → Generated` 语法见
[Configuration schema](docs/configuration-schema.md)。不支持 JSON Schema 的兼容接口可以关闭该选项；
插件只检查结果非空，不会在本地改写格式或截断。响应支持 `choices[0].message.content` 和顶层 `output_text`。
当 Maximum message characters 大于 `0` 时，同一数值会加入 prompt 和 schema 的 `maxLength`。

当前发送给模型的数据结构和完整示例见
[Model request example](docs/model-request-example.md)。关系分析固定执行，但最多包含 12 个 changed symbols、
每类 6 条关系，并最多扫描每个 symbol 的 60 个引用候选。

## 隐私边界

只有用户主动执行生成动作时，插件才会把所选文本变更的 before/after 内容发送到配置的
API URL。默认最多发送 60,000 个字符。二进制文件只发送路径和变更类型，不发送字节内容。

**Test API** 只发送固定的 `Reply with OK`，不会发送任何仓库文件或 diff。
