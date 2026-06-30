# tt slide 与 .slide 语义规范

> 最后更新：2026-06-29

`tt slide` 是面向 `.slide` 文件的本地演示服务。它使用 Go 启动本地 HTTP 服务，前端基于 Reveal.js 渲染幻灯片，并支持 live reload、文件列表切换、模板美化、Mermaid / D2 图表和演示 overview。

## 核心原则：.slide 是语义文档，template 只负责美化

`.slide` 文档的主要职责是定义演示内容和语义结构。它必须尽量保持模板无关，不能出现只有某一个 template 才能理解或才能正确显示的写法。

规则：

- `.slide` 里写标题、正文、列表、表格、图片、代码块、Mermaid / D2，以及通用布局语义。
- `.slide` 可以使用语义指令，例如 `.center`、`.cover`、`.split`、`.two-column`、`.brand`、`.end`。
- `.slide` 不应该写 `template: magicloud` / `template: dark` 这类模板选择。模板选择属于运行时配置，用 `tt slide --template ...` 或 URL query 完成。
- `.slide` 不应该依赖某个模板的品牌、颜色、logo、字体、背景图、CSS class 或视觉细节。
- template 的职责是读取这些通用语义，然后决定如何美化。例如同一个 `.end` 在 MagiCloud 模板中可以渲染为 MagiCloud 封底，在其他模板中可以渲染为该模板自己的结束页。

换句话说：先写一份可被任何模板解释的 `.slide`，再让 template 负责把它变漂亮。

## 快速开始

```bash
tt slide
# 或打开指定 .slide 文件
tt slide path/to/demo.slide
# 或打开某个目录下的 .slide 文件列表
tt slide slides/
```

`tt slide` 现在只支持 `.slide` 文件，不再把 `.md` / `.markdown` 当作 slide deck 扫描或打开。幻灯片之间用 `---` 分隔：

```markdown
# 第一页标题

正文内容

---

# 第二页标题

- 要点 A
- 要点 B
```

## 模板选择

默认模板是 `magicloud`，但这是 `tt slide` 的运行时默认值，不是 `.slide` 文档的一部分。需要换模板时用命令行：

```bash
tt slide demo.slide --template dark
tt slide demo.slide --template light
```

不要在 `.slide` front matter 中写 `template:`。当前渲染器会忽略 `.slide` 内的 `template:`，以避免文档和模板耦合。

## MagiCloud 模板

默认模板是 `magicloud`，风格贴近 `MC PPT Template.pptx`：

- 主色：MagiCloud 绿系，核心色包括 `#00643C`、`#00633B`、`#008D55`。
- 正文灰：偏温和的 `#535E59` / `#595959`。
- 背景：白底内容页、绿黑渐变封面和尾页、蜂窝 mesh 背景。
- 字体：优先使用 `Aptos` / `Aptos Display`，再 fallback 到 `Segoe UI`、`Arial`、`PingFang SC`、`Noto Sans SC`。
- 普通内容页标题区下移，正文行距更舒展，适合中英文混排。

## 页面指令：只能表达通用语义

可以在每页开头写一个点号指令来控制版式。指令行不会出现在页面内容中。

```markdown
---

.center

# 居中页

一段居中的说明。
```

常用语义指令：

| 指令 | 作用 |
| --- | --- |
| `.center` | 居中内容页 |
| `.cover` | 封面语义，仍使用第一页封面样式优先级 |
| `.split` | 左右分栏 / 图文页 |
| `.two-column` / `.columns` | 两列内容页 |
| `.brand` / `.logo` | 品牌页语义，由当前模板决定 logo / 品牌呈现方式 |
| `.end` / `.closing` / `.final` | 结束页 / 封底页语义，由当前模板决定视觉效果 |

## 结束页 / 封底页

`.end` 表示“这是演示结束页”。它不是 MagiCloud 专用写法。不同模板可以把它渲染成各自的结束页。可以用下面任意一种语义指令：

```markdown
---

.end
```

也支持：

```markdown
---

.closing
```

或：

```markdown
---

.final
```

在 MagiCloud 模板中，`.end` 的效果按 `MC PPT Template.pptx` 第 15 页制作：

- 绿黑渐变背景铺满全区域。
- 蜂窝 mesh 背景覆盖整个页面。
- 中间显示大号白色 `FLEXCOMPUTE | MagiCloud` logo。
- 普通播放时背景在固定 viewport 层淡入淡出，避免上下切换时出现背景切边。
- ESC overview 缩略图模式仍保留缩略页自身背景，方便识别尾页。

推荐把尾页放在 deck 最后：

```markdown
# 最后一页正文

谢谢观看。

---

.end
```

## 切换与位置记录

- 左下角文件按钮默认隐藏，鼠标移到左下角热区才显示。
- 切换不同 `.slide` 文件时，当前位置互不影响。
- 每个 `.slide` 文件会用浏览器 `localStorage` 独立记录上次位置。
- 第一次打开某个文件默认从第一页开始。

## ESC overview

按 `Esc` 会进入 Reveal overview。当前实现将横向缩略图固定在 viewport 中，并隔离横向滚动，避免 overview 宽度撑破或影响下面的正常页面。
