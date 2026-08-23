# ToolHub

ToolHub 是本机工具的统一入口。它从自己的中央目录读取注册信息，负责启动命令、显示
运行状态、保留日志、发送等价于终端 `Ctrl+C` 的停止信号，并在 Web 服务就绪后提供
直接跳转链接。被管理工具不需要依赖 ToolHub，也不需要存放 ToolHub 配置。

## 直接运行

要求 Go 1.24 或更高版本。在 Lib 仓库根目录执行：

```bash
go run ./utils/toolhub
```

打开 <http://127.0.0.1:17840>。默认中央目录已经注册：

- IINA Resume
- 分享视频下载器
- 简历转换工作台
- AIEra 本地字幕生成器

前三项是 Web 服务，启动并通过健康检查后可以直接打开网页。字幕生成器是一次性任务，
页面根据中央声明生成参数表单，并显示完成、失败或取消状态。

## 安装为登录后常驻入口

```bash
cd utils/toolhub
./install.sh
```

安装脚本只安装 ToolHub 本身；它不会自动启动目录中的其他工具。安装或中央目录变化后
重新执行脚本即可更新。卸载入口：

```bash
cd utils/toolhub
./uninstall.sh
```

## 状态和边界

- **未运行**：健康地址不可访问，也没有 ToolHub 启动的进程。
- **外部运行**：健康地址已经可访问，但实例不是 ToolHub 启动的。页面允许打开，不允许停止。
- **启动中**：命令已经预留或启动，正在等待健康检查。
- **运行中**：ToolHub 拥有进程，服务健康或任务仍在执行。
- **运行异常**：进程仍存在，但连续健康检查失败。可以查看日志或停止。
- **失败**：命令无法启动、服务启动超时或进程意外退出；退出码和最近日志会保留。
- **已完成**：一次性任务以退出码 0 正常结束。

停止时先向独立进程组发送 `SIGINT`，让 Go、Node、Python 及其子进程按各自的
Ctrl+C 逻辑收尾。超过工具声明的停止时限后才发送强制终止信号。

如果 IINA Resume 已通过自己的 `install.sh` 注册为 LaunchAgent，ToolHub 会发现
`17845` 已有服务并显示“外部运行”。若希望由 ToolHub 启停它，先使用 IINA Resume
自己的 `uninstall.sh` 移除原 LaunchAgent。

## 集中注册新工具

所有注册都位于 [catalog/tools.yaml](catalog/tools.yaml)。不要在被管理项目中创建
`.toolhub.yaml`。面向编码大模型和工程师的字段协议、流程与交付清单见
[AGENT_REGISTRATION.md](AGENT_REGISTRATION.md)。

只校验中央目录，不启动服务器或工具：

```bash
go run ./utils/toolhub -validate -catalog ./utils/toolhub/catalog/tools.yaml
```

## 验证

```bash
go test ./utils/toolhub/...
go vet ./utils/toolhub/...
```

生命周期集成测试会在 `127.0.0.1` 随机端口启动测试子进程，覆盖健康就绪、外部实例
识别、异常退出、日志保留和 Ctrl+C 停止。
