# PADS Logic Library Generator

一个本地 Windows 上位机工具，用于根据芯片 pinout 截图和物料信息生成 PADS Logic 库文件。

它可以生成两类 PADS Logic 文件：

```text
xxx_logic.c   PADS Logic SCH Decal / 原理图逻辑符号
xxx_pack.p    PADS Part Type / 器件属性、PCB Decal 绑定、pin number/name 映射
```

## 功能

- 本地 WebView2 桌面窗口，双击 exe 即可打开
- 可配置 OpenAI-compatible 大模型 API
- 支持 Text Model / Vision Model 分开配置
- 支持物料截图识别：MPN、商品编号、封装等
- 支持 pinout 截图识别：pin number、pin name、side、exposed pad
- 支持人工校对 pin 表
- 支持 pin name 规范化
- 支持生成并下载 `.c` / `.p`
- 前端资源嵌入 exe，可构建为单文件 Windows 程序

## 运行源码

环境要求：

- Windows
- Go 1.23+
- Microsoft Edge WebView2 Runtime

运行：

```powershell
go run .
```

程序会启动本地服务并打开 WebView2 窗口。也可以访问：

```text
http://127.0.0.1:18080
```

## 直接运行 exe

仓库中已提供编译好的 Windows 单文件版本：

```text
release/padslogic.exe
```

适用环境：

```text
操作系统：Windows 10 / Windows 11，64-bit
运行时：Microsoft Edge WebView2 Runtime
网络：需要能访问你配置的大模型 API 地址
PADS：本工具只生成 .c/.p 文件，不要求本机安装 PADS；导入库文件时才需要 PADS Logic / PADS Layout
```

运行方式：

```text
双击 release/padslogic.exe
```

启动后会打开一个本地桌面窗口。程序不会把 API Key 上传到本项目服务器；API Key 只保存在 exe 同目录下自动生成的：

```text
config.json
```

运行时还会生成：

```text
output/   生成的 .c / .p 文件
logs/     调试日志
```

如果双击后窗口无法打开，请先安装 Microsoft Edge WebView2 Runtime，或确认系统中 Microsoft Edge 可正常运行。

## 构建

普通构建：

```powershell
.\build.ps1
```

输出：

```text
dist/padslogic.exe
```

单文件构建：

```powershell
.\build-single.ps1
```

输出：

```text
out_single/padslogic.exe
```

`out_single/padslogic.exe` 已嵌入前端资源，可以单独复制给别人使用。运行时会在 exe 所在目录自动生成：

```text
config.json
output/
logs/
```

## API 配置

界面中配置：

```text
Base URL      OpenAI-compatible API 地址，例如 https://api.example.com/v1
API Key       只填 key 本身，不要带 Bearer
Text Model    用于“测试接口”
Vision Model  用于图片识别；为空时使用 Text Model
Timeout(s)    图片识别建议 60-180 秒
```

`config.json` 只保存在本机，不建议提交到 GitHub。

## 使用流程

1. 配置大模型 API。
2. 点击“测试接口”确认文本模型可用。
3. 点击“测试视觉”确认图片模型可用。
4. 手工填写物料信息，或拖入物料信息截图识别。
5. 拖入 datasheet pinout 截图。
6. 点击“调用大模型识别”。
7. 人工校对 pin 表。
8. 点击“生成 .c / .p”。
9. 下载生成文件。

## PADS 生成规则

`.c` 文件：

```text
*PADS-LIBRARY-SCH-DECALS-V9*
```

用于定义 PADS Logic 原理图符号外观和 pin 端子位置。

`.p` 文件：

```text
*PADS-LIBRARY-PART-TYPES-V9*
```

用于定义器件属性、PCB Decal 名称和 pin number/name 映射。

注意：

- `.p` 中的 PCB Decal 名称必须和 PADS Layout 库里的实际 footprint 名称一致。
- exposed pad 建议作为额外 pin，例如 `EP_GND`。
- 模型识别结果必须人工校对。
- datasheet 的 top view / bottom view 必须人工确认。

## 示例

示例输入：

```text
samples/tps7a8300.json
```

可以作为 API `/api/generate` 的输入样例，也可以参考其中字段手工录入。

## 文档

详细实现方案见：

```text
docs/PADS_Logic封装自动生成上位机实现方案.md
```

## 不会上传的本地文件

以下文件已在 `.gitignore` 中排除：

```text
config.json
logs/
output/
dist/
out/
out_single/
single_test/
```

## 🍻 请我喝杯咖啡 / Buy me a coffee

如果这个项目对你有帮助，或者单纯想鼓励一下我，可以请我喝杯咖啡。
If this project helped you, feel free to buy me a coffee.

<p align="center">
  <img src="docs/reward.jpg" alt="WeChat Reward QR" width="300">
  <br>
  <sub>微信扫码赞赏 / Scan with WeChat</sub>
</p>

## License

MIT
