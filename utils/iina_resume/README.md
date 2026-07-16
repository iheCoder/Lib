# IINA Resume

一个仅在本机运行的小工具：打开网页即可看到 IINA 最近关闭的多窗口播放会话，并在独立窗口中一键恢复该会话的全部视频。

## 会话是怎样恢复的

IINA 没有直接保存“上次关闭时所有播放器窗口”的列表，但它已经提供了重建会话所需的两部分数据：

- `history.plist` 保存媒体路径与 mpv watch-later MD5 的对应关系；
- `watch_later/` 保存每个媒体的断点，并在 IINA 退出、批量关闭播放器时集中更新。

本工具将同一小段时间内更新的有效断点识别为一个关闭会话，再通过 MD5 从历史索引反查全部媒体。mpv 创建的 `# redirect entry` 标记会被排除。页面保留最近 10 个可恢复批次，因此从旧版单视频工具升级后，仍可选择紧邻的、更早的多窗口会话。

恢复时使用 IINA 自带的：

```text
iina-cli --no-stdin --separate-windows <视频1> <视频2> ...
```

路径作为独立进程参数传递，不经过 shell。实际播放断点仍由 IINA/mpv 自己的 watch-later 数据控制。

## 安装或升级

要求：macOS、安装在 `/Applications/IINA.app` 的 IINA，以及用于首次构建的 Go 1.23 或更高版本。

```bash
cd ~/Lib/utils/iina_resume
./install.sh
```

重复运行安装脚本即可升级。脚本会：

1. 编译一个无第三方依赖的本地服务；
2. 安装为当前用户的 `launchd` 后台任务，登录后自动运行；
3. 打开 <http://127.0.0.1:17845>。

建议将页面加入浏览器书签。服务只监听 `127.0.0.1`，不会暴露给局域网；恢复接口拒绝跨站请求，并且只接受后台生成的会话 ID，不接受页面传入文件路径。

## IINA 设置要求

多窗口恢复依赖 IINA 的播放历史与断点文件。请保持以下能力启用：

- IINA 的播放历史记录；
- mpv/IINA 的退出时保存播放位置。

如果历史文件缺失或相关设置被关闭，工具会兼容回退到 IINA 的 `iinaLastPlayedFilePath`，但这种回退只能恢复一个视频。

## 开发与验证

```bash
go test ./utils/iina_resume
go run ./utils/iina_resume
```

打开 <http://127.0.0.1:17845>。如果部分视频位于未连接的移动硬盘，页面会保留这些文件的信息，但只恢复当前可用的视频。

## 卸载

```bash
cd /Users/ihewe/GolandProjects/Lib/utils/iina_resume
./uninstall.sh
```

卸载会保留日志，方便排查问题；日志目录为 `~/Library/Logs/IINA Resume`。
