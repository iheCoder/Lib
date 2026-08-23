# ToolHub 中央注册协议

本文面向负责新增或修改工具的编码大模型与工程师。ToolHub 集成信息只属于
ToolHub；不要在被管理工具的目录中创建 `.toolhub.yaml`、SDK、探针或其他感知代码。

## 何时注册

当一个工具能够作为本地 Web 服务长期运行，或者能够作为有明确输入的一次性命令运行
时，在 `utils/toolhub/catalog/tools.yaml` 中增加记录。纯库、测试辅助程序和必须人工操作
交互式终端的程序不应注册。

## 注册步骤

1. 阅读工具自己的 README 和真实程序入口，不要根据目录名猜测命令。
2. 在普通终端中验证启动命令、工作目录、退出方式和依赖。
3. Web 工具选择 `kind: service`；执行后自然退出的工具选择 `kind: task`。
4. 只修改 ToolHub 的 `catalog/tools.yaml`，不要修改被管理工具来适配 ToolHub。
5. 命令与参数必须拆成 `command` 和 `args`，禁止 `sh -c`、管道、重定向和字符串拼接。
6. 运行下方校验命令，然后通过页面完成一次启动、异常日志查看与停止验证。
7. 对 Web 工具额外确认“外部先启动”时页面显示外部运行，且 ToolHub 不提供停止按钮。

## Service 必填信息

```yaml
- id: stable-lowercase-id
  name: 用户可读名称
  description: 一句话说明工具解决什么问题
  category: 媒体
  kind: service
  working_directory: ${REPO_ROOT}/utils/example
  command: go
  args: [run, .]
  environment: {}
  url: http://127.0.0.1:19000
  health_url: http://127.0.0.1:19000/
  startup_timeout: 20s
  stop_timeout: 5s
```

`url` 是用户点击“打开”后进入的页面。`health_url` 可以相同，也可以是专门的轻量
健康接口。两者必须使用 `http://127.0.0.1`；ToolHub 不负责管理暴露到网络的服务。

## Task 输入

任务的固定参数放在 `args`，用户输入由 `inputs` 按声明顺序追加。支持三类输入：

- `text`：可用 `position: true` 作为位置参数，或者用 `flag: --output-dir` 作为具名参数。
- `select`：必须声明 `options` 和 `flag`。
- `boolean`：选中时只追加 `flag`，未选中时不追加。

```yaml
- id: example-task
  name: 示例任务
  description: 处理一个本地文件
  category: 文档
  kind: task
  working_directory: ~/PycharmProjects/Example
  command: uv
  args: [run, python, -m, example, process]
  stop_timeout: 10s
  inputs:
    - id: input_file
      label: 输入文件路径
      type: text
      position: true
      required: true
    - id: format
      label: 输出格式
      type: select
      flag: --format
      default: pdf
      options: [pdf, html]
    - id: offline
      label: 离线运行
      type: boolean
      flag: --offline
      default: "true"
```

浏览器不会向本地网页暴露文件的绝对路径，因此文件输入使用明确标注的路径文本框。
不要用上传控件读取大型文件，也不要让文件内容经过 ToolHub。

## 验证清单

从仓库根目录执行：

```bash
go test ./utils/toolhub/...
go vet ./utils/toolhub/...
go run ./utils/toolhub -validate -catalog ./utils/toolhub/catalog/tools.yaml
go run ./utils/toolhub
```

然后检查：

- 注册项出现在首页，名称、分类和说明正确。
- 服务能从“未运行”进入“启动中”再进入“运行中”。
- “打开工具”在新标签页进入声明的 URL。
- 点击停止后端口释放，没有遗留子进程。
- 缺少命令、目录错误、端口冲突和进程异常退出时，页面给出可恢复的错误与日志。
- 工具在 ToolHub 之外启动时显示“外部运行”，可以打开但不能停止。

只有上述验证完成后，注册才算交付完成。
