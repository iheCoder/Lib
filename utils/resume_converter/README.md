# md-resume-pdf

将任意路径下的简历 Markdown 导出为可读、美观且经过校验的一页 A4 PDF。

工具会先使用舒适排版，再根据浏览器真实渲染高度调整留白、行距和字号。若内容在最低可读字号下仍无法放进一页，工具会拒绝生成 PDF，并指出占用最大的章节和建议精简的内容。

## 安装

```bash
cd utils/resume_converter
npm install
npx playwright install chromium
npm link
```

工具优先使用 Playwright 锁定版本的 Chromium；若尚未下载，则会尝试使用系统中已安装的 Google Chrome。

安装后可在任意目录运行：

```bash
md-resume-pdf "/path/to/中文 简历.md"
md-resume-pdf resume.md -o output/resume.pdf
```

## 网页工作台

```bash
npm start
```

然后打开 [http://127.0.0.1:4173](http://127.0.0.1:4173)。Guided Studio 提供 Markdown 拖放与编辑、结构检查、实时 A4 预览、版式预设、单页诊断和 PDF 下载；内容只在本机处理。

也可以继续使用兼容脚本：

```bash
./export_resume_one_page.sh resume.md output/resume.pdf
```

## 参数

```text
--theme classic
--accent "#17365d"
--margin 8mm
--min-font-size 9pt
--max-font-size 10.5pt
--debug
```

默认输出到 Markdown 同目录下的同名 PDF。`--debug` 会在 `<输出 PDF>.debug/` 中保留最终 HTML、页面截图和布局诊断 JSON。

## Markdown 结构建议

工具将第一个标题识别为姓名，将常见的中英文简历章节标题识别为主章节：

```markdown
# 张三

北京 | zhang@example.com

## 个人定位

...

## 工作经历

### 示例公司 | 后端工程师

- 可量化的工作成果
```

建议使用一级或二级标题表示主章节，使用更低级标题表示公司、项目或子模块。首版专门服务于简历，不承诺将文章、代码文档等任意 Markdown 压缩为一页。

## 自适应与失败策略

排版按以下顺序调整：

1. 在最大字号下尝试增加章节留白和行距，让短简历充分利用页面。
2. 内容溢出时先收紧段落、列表和章节间距。
3. 仍然溢出时，在配置范围内逐步降低字号。
4. 达到最低字号仍无法容纳时，返回诊断并删除可能存在的旧输出 PDF。

严格一页不会以无限缩小字号为代价。

## 测试

```bash
npm test
npm run test:integration
```
