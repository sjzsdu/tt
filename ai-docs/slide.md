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
tt slide examples/slides/
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
tt slide --list-templates
tt slide demo.slide --template dark
tt slide demo.slide --template light
```

`tt slide --list-templates` 会列出内置模板、项目 `.tt/slide/templates/` 模板和全局 `~/.tt/slide/templates/` 模板，并显示来源路径。

不要在 `.slide` front matter 中写 `template:`。当前渲染器会忽略 `.slide` 内的 `template:`，以避免文档和模板耦合。

## `.tt` 中的自定义模板约定

当前支持项目自定义模板放在项目 `.tt` 下，而不是写进 `.slide` 文件：

```text
.tt/
└── slide/
    └── templates/
        └── my-template/
            ├── template.json
            ├── template.css
            └── assets/
                ├── cover-bg.png
                ├── logo-dark.png
                └── logo-white.svg
```

`template.json` 只描述模板元数据和 Reveal 默认值：

```json
{
  "name": "my-template",
  "revealTheme": "white",
  "css": "template.css",
  "defaults": {
    "theme": "light",
    "transition": "fade",
    "center": false,
    "margin": 0
  },
  "vars": {
    "slide-padding": "96px 112px 72px",
    "cover-background": "url(\"./assets/cover-bg.png\") center / cover no-repeat",
    "logo-width": "280px"
  }
}
```

`vars` 会被注入为 CSS 变量，例如 `slide-padding` 变成 `--slide-padding`。上下左右边距、背景图、logo 尺寸、品牌颜色等全局视觉参数优先写在 `vars` 中；细节布局仍写在 `template.css`。

`template.css` 写视觉实现。资源放在同模板目录的 `assets/` 下，并用相对路径引用：

```css
:root {
  --slide-bg: #ffffff;
  --slide-fg: #1f2329;
  --slide-accent: #008d55;
}

.reveal .slides section {
  padding: var(--slide-padding, 96px 112px 72px);
  background: var(--slide-bg);
  color: var(--slide-fg);
}

.reveal:has(.slides section:first-child.present)::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: var(--cover-background);
}

.reveal .slides section:not(:first-child)::before {
  content: "";
  position: absolute;
  top: 48px;
  left: 64px;
  width: var(--logo-width, 280px);
  height: 32px;
  background: url("./assets/logo-dark.png") left center / contain no-repeat;
}
```

资源规则：

- 背景、logo、纹理等 template 专属资源放在 `.tt/slide/templates/<name>/assets/`。
- `.slide` 文档里的业务图片仍然放在文档附近，用正常 Markdown 图片引用。
- template CSS 只能引用自己目录下的相对资源，避免依赖用户机器上的绝对路径。
- 不要把二进制资源写进 `.slide`，也不要把 template asset 路径写进 `.slide`。
- 如果同名模板同时存在于项目 `.tt` 和全局 `~/.tt`，项目模板优先。

启动时仍然应该通过运行时配置选择模板，例如：

```bash
tt slide deck.slide --template my-template
```

这样 `.slide` 继续保持模板无关，`.tt/slide/templates/my-template/` 只负责视觉美化和 assets 打包。

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
| `.three-column` / `.three-columns` | 三列内容页，适合三个并列观点、阶段、方案或案例；每列内容必须短 |
| `.four-column` / `.four-columns` | 四列内容页，适合四个短标签、步骤或对比项；避免长段落 |
| `.brand` / `.logo` | 品牌页语义，由当前模板决定 logo / 品牌呈现方式 |
| `.full-bleed` / `.bleed` | 满屏铺展页，移除 section padding，单张图片/视频/iframe/canvas/svg 会按 `object-fit: cover` 铺满 16:9 舞台，适合封面大图和沉浸式视觉页 |
| `.no-padding` | 仅移除 section padding，单张媒体按 `object-fit: contain` 显示，不裁剪，适合需要完整显示的图表、截图或设计稿 |
| `.absolute` / `.freeform` | 自由布局页，移除 padding，并允许页面内 `.abs` / `.abs-center` / `.abs-fill` 元素按 CSS 变量绝对定位到 1600×900 舞台任意位置 |
| `.no-panzoom` / `.no-zoom` / `.no-drag` / `.static-diagram` | 禁用本页 Mermaid / D2 图表的滚轮缩放、拖拽平移和图表 toolbar，适合静态演示或避免误触 |
| `.end` / `.closing` / `.final` | 结束页 / 封底页语义，由当前模板决定视觉效果 |

满屏视觉页示例：

```markdown
---

.full-bleed

![背景图](./hero.png)
```

完整截图页示例：

```markdown
---

.no-padding

![产品截图](./screenshot.png)
```

也可以直接使用 Reveal 原生 section class：

```html
<section class="full-bleed" data-transition="fade">
  <img src="./hero.png" alt="">
</section>
```

自由定位页示例。舞台固定为 1600×900，`--x` / `--y` / `--w` / `--h` 可使用 px、%、vw 等 CSS 单位：

```markdown
---

.absolute

<div class="abs" style="--x:96px; --y:80px; --w:720px; --z:2">
  <h1>核心判断</h1>
  <p>一页只承载一个明确观点。</p>
</div>

<div class="abs media-box media-cover" style="--x:900px; --y:0; --w:700px; --h:900px">
  <img src="./hero.png" alt="">
</div>
```

常用定位类：

- `.abs`：按 `--x`、`--y`、`--right`、`--bottom`、`--w`、`--h` 精确定位。
- `.abs-center`：默认居中，可用 `--x`、`--y` 改中心点。
- `.abs-fill`：默认铺满页面，可用 `--inset` 设置内缩。
- 可选变量：`--z` 控制层级，`--rotate` 控制旋转，`--scale` 控制缩放，`--tx` / `--ty` 做微调。

媒体尺寸控制示例：

```html
<div class="media-box media-contain" style="--w:980px; --h:560px; --radius:18px">
  <img src="./screenshot.png" alt="产品截图">
</div>
```

常用媒体类：

- `.media-box` / `.slide-media-box`：媒体容器，用 `--w`、`--h`、`--aspect`、`--radius` 控制尺寸和圆角。
- `.media-cover`：裁剪铺满，等价于 `object-fit: cover`。
- `.media-contain`：完整显示，等价于 `object-fit: contain`。
- `.media-fill` / `.media-stretch`：拉伸填满。
- `.media-none`：原始尺寸显示。
- `.media-16x9` / `.media-4x3` / `.media-1x1`：快速设置常见比例。

禁用图表拖拽缩放示例：

````markdown
---

.no-panzoom

```mermaid
graph LR
  A[需求] --> B[设计]
  B --> C[交付]
```
````

`.no-panzoom` 只影响 Mermaid / D2 图表 viewport，不会禁用 Reveal 翻页。

三列 / 四列内容页示例，继续使用 `:::columns` 分隔每一列：

```markdown
---

.three-column

# 三种路径

:::columns
## 路径 A
- 轻量
- 快速
:::

:::columns
## 路径 B
- 稳定
- 可扩展
:::

:::columns
## 路径 C
- 深度定制
- 成本更高
:::
```

```markdown
---

.four-column

# 四步流程

:::columns
## 发现
一句话说明。
:::

:::columns
## 判断
一句话说明。
:::

:::columns
## 行动
一句话说明。
:::

:::columns
## 复盘
一句话说明。
:::
```

三列和四列会缩小正文密度。若每列超过 2 到 3 个短点，优先拆页，避免内容超出 1600×900 边界。

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

## Reveal 进阶节奏与动画

`tt slide` 的播放器基于 Reveal.js，因此可以利用 Reveal 的节奏控制能力。使用时要遵守一个边界：**普通 `.slide` 内容优先保持语义化和模板无关；只有在确实需要演示节奏或动态效果时，才使用 Reveal class / data 属性或少量 raw HTML。**

可以把进阶用法分成三层：

1. **slide 之间的切换动画**：由 Reveal 配置或单页 `data-transition` 控制。
2. **slide 内元素的分步进出**：由 `.fragment`、`data-fragment-index`、`.current-visible`、`.highlight-*` 等 class 控制。
3. **高级状态动画**：由 `data-auto-animate`、模板 CSS、或 Reveal 事件监听驱动 SVG、图表、视频等内容。

### 页间切换

全局切换效果优先放在模板 `template.json` 的 `defaults.transition` 中，而不是写进 `.slide`：

```json
{
  "defaults": {
    "transition": "fade",
    "backgroundTransition": "slide"
  }
}
```

如果某一页确实需要特殊转场，可以用 raw HTML section 属性，但这会让该页更接近 Reveal 原生写法，应该少用：

```html
<section data-transition="zoom">

# 关键架构变化

这一页需要更强的视觉强调。

</section>
```

推荐原则：默认用模板控制全局转场；单页 `data-transition` 只用于少数强调页。

### 页内分步显示：fragment

Reveal 的 fragment 适合控制讲述节奏。Markdown 原生列表没有专用语法时，可以用少量 HTML 包裹需要逐步出现的内容：

```html
<ul>
  <li class="fragment" data-fragment-index="1">先说明现状</li>
  <li class="fragment" data-fragment-index="2">再指出瓶颈</li>
  <li class="fragment highlight-green" data-fragment-index="3">最后给出结论</li>
</ul>
```

常用 fragment class：

| class | 作用 |
| --- | --- |
| `fragment` | 按步骤出现 |
| `fade-in` / `fade-out` | 淡入 / 淡出 |
| `current-visible` | 只在当前 fragment 步骤可见 |
| `highlight-red` / `highlight-green` / `highlight-blue` | 当前步骤高亮 |

使用规则：

- 只给真正需要讲述节奏的 2 到 5 个元素加 fragment。
- 不要把整页所有 bullet 都做成 fragment，除非是培训或逐步推导页。
- `data-fragment-index` 用于明确顺序，避免复杂布局中出现顺序误判。
- 如果只是希望页面更美观，不要用 fragment，让模板负责视觉布局。

### 跨页连续动画：auto-animate

`data-auto-animate` 适合展示“同一个对象从一个状态变到另一个状态”，例如架构演进、流程扩展、指标变化。Reveal 会根据相邻 slide 中相同元素的结构或 `data-id` 做连续过渡。

```html
<section data-auto-animate>

# 当前流程

<div data-id="pipeline">用户 → API → Worker</div>

</section>

---

<section data-auto-animate>

# 引入调度层

<div data-id="pipeline">用户 → API → Scheduler → Worker</div>

</section>
```

推荐用法：

- 相邻两页都加 `data-auto-animate`。
- 给需要连续变化的核心元素加稳定 `data-id`。
- 每次只表达一个结构变化，避免多个对象同时飞来飞去。
- 如果只是普通翻页，不要使用 auto-animate。

### CSS 与 JS 扩展边界

模板 CSS 可以定制 fragment 的可见态和当前态，例如：

```css
.reveal .fragment.soft-dim {
  opacity: 0.25;
}

.reveal .fragment.soft-dim.visible {
  opacity: 1;
}

.reveal .current-fragment.callout {
  outline: 4px solid var(--slide-accent);
}
```

更高级的联动可以通过 Reveal 事件完成，例如 `slidechanged`、`fragmentshown`、`fragmenthidden`。这类逻辑应放在播放器或模板扩展中，而不是散落在普通 `.slide` 文档里。适合使用 JS 的场景包括：

- fragment 出现时启动 SVG 路径动画。
- 切到某页时播放或暂停视频。
- 根据当前 fragment 更新图表高亮状态。

原则：Reveal 负责演示节奏，CSS 负责动画表现，JS 负责少数高级联动。普通内容页仍应优先使用 `.center`、`.split`、`.two-column`、`.cards` 等语义指令。

## ESC overview

按 `Esc` 会进入 Reveal overview。当前实现将横向缩略图固定在 viewport 中，并隔离横向滚动，避免 overview 宽度撑破或影响下面的正常页面。
