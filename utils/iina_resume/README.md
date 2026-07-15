# IINA Resume

一个仅在本机运行的小工具：打开网页即可看到 IINA 上次关闭的视频，并一键交给 IINA 继续播放。

## 为什么不需要监听脚本

IINA 已经在 macOS 偏好设置中维护：

- `iinaLastPlayedFilePath`：最近播放的文件或 URL
- `iinaLastPlayedFilePosition`：关闭时的播放位置

本工具在页面加载和点击恢复时直接读取这两个值，避免维护第二份容易过期的历史记录。实际断点仍由 IINA 自己的 `watch_later` 数据恢复；本工具只负责找到并打开最后一个视频。

## 安装

要求：macOS、IINA，以及用于首次构建的 Go 1.23 或更高版本。

```bash
cd /Users/ihewe/GolandProjects/Lib/utils/iina_resume
./install.sh
```

安装脚本会：

1. 编译一个无第三方依赖的本地服务；
2. 安装为当前用户的 `launchd` 后台任务，登录后自动运行；
3. 打开 <http://127.0.0.1:17845>。

建议将页面加入浏览器书签。服务只监听 `127.0.0.1`，不会暴露给局域网；恢复接口也拒绝跨站请求。

## 开发与验证

```bash
go test ./utils/iina_resume
go run ./utils/iina_resume
```

打开 <http://127.0.0.1:17845>。如果上次视频位于移动硬盘，硬盘未连接时页面会保留文件信息但禁用恢复按钮。

## 卸载

```bash
cd /Users/ihewe/GolandProjects/Lib/utils/iina_resume
./uninstall.sh
```

卸载会保留日志，方便排查问题；日志目录为 `~/Library/Logs/IINA Resume`。
