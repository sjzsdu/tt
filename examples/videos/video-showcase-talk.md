---
title: tt slide 视频生成演示
slides: ../slides/tt-slide-showcase.slide
width: 1280
height: 720
fps: 24
---

# 开场：tt slide 全能力示例集
slide: 1

大家好，这是一段由 tt video 自动生成的视频演示。

我们会用现有的 tt slide 示例文档，验证从幻灯片、讲稿、字幕到视频合成的完整流程。

# 能力总览
slide: 2

这份 deck 覆盖六类能力，包括文档规则、页面布局、Markdown 内容、Mermaid 图表、D2 图表，以及演示体验。

这也是视频生成命令的理想测试材料，因为它包含了多种真实页面结构。

# 内容和结构分离
slide: 3

.slide 文件只描述内容和结构，模板负责最终视觉呈现。

这个原则很重要，因为视频生成应该复用网页幻灯片已经具备的渲染能力，而不是重新实现一套排版系统。

# 最小文档骨架
slide: 4

一个最小的 slide 文档由 front matter、第一页内容、分页符和第二页内容组成。

视频脚本则在外部集中管理讲稿，并通过 slide 编号映射到具体页面。

# 当前规则
slide: 5

当前规则包括文件扩展名、分页方式、front matter、固定画布、资产路径和图表写法。

这些规则让 slide 文档可以稳定渲染，也让视频生成流程可以自动截图和合成。

# 页面指令
slide: 6

tt slide 支持 cover、center、hero、two-column、grid 和 cards 等页面指令。

在视频生成中，这些指令仍然由原来的网页渲染器处理，因此视频画面和浏览器演示保持一致。
