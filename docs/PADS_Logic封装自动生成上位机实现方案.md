# PADS Logic 封装自动生成上位机实现方案

## 1. 目标

开发一个运行在 PC 本地的上位机程序，用于根据芯片手册中的 pinout 截图自动生成 PADS Logic 所需的两类库文件：

```text
xxx_logic.c   PADS Logic SCH Decal / 原理图逻辑符号
xxx_pack.p    PADS Part Type / 器件属性、PCB decal 绑定、pin number/name 映射
```

用户拖入芯片引脚图截图，填写物料信息和 PCB decal 名称，软件调用可配置的大模型视觉 API 识别引脚表，经人工确认后生成 PADS Logic 库文件。

本工具只生成 PADS Logic 原理图符号和 Part Type 映射文件，不生成 PCB footprint 几何。PCB decal 名称必须与 PADS Layout 库中的实际 footprint 名称一致。

## 2. 总体架构

```text
网页前端 UI
  ↕ HTTP / WebSocket
Go 本地后台服务
  ├─ 大模型 API 客户端
  ├─ 图片上传与识别
  ├─ Pin 表人工校对数据模型
  ├─ PADS .c 生成器
  ├─ PADS .p 生成器
  └─ 本地文件导出
```

第一版采用本地 Web 服务形态：

```text
app.exe
打开 http://127.0.0.1:18080
```

后续可用 Wails 打包为桌面应用：

```text
Go 后台 + WebView 前端
```

## 3. 技术栈

后台建议：

```text
Go
Gin 或 Echo：HTTP API
Gorilla WebSocket：进度推送
Viper：配置文件管理
```

前端建议：

```text
React + TypeScript
Vite
TailwindCSS
shadcn/ui 或 Ant Design
```

配置文件建议：

```text
config.json
```

API Key 只保存在本机，不上传到除用户配置的大模型服务以外的第三方。

## 4. 功能模块

### 4.1 项目首页

功能：

- 拖拽上传芯片 pinout 截图
- 填写商品型号、商品编号、封装名称、PCB decal 名称
- 填写器件描述和输出文件名前缀
- 选择是否包含 exposed pad
- 一键调用大模型识别 pin 表
- 预览生成的 `.c` 和 `.p` 文本
- 导出文件

### 4.2 大模型 API 配置

支持 OpenAI API 或兼容 OpenAI API 的模型服务。

配置项：

```text
API Base URL
API Key
Model Name
Temperature
Timeout
```

典型用途：

- 识别 datasheet pinout 图片
- 输出结构化 JSON pin 表
- 对 pin name 做规范化建议

### 4.3 图片识别模块

用户上传芯片 pinout 图片后，后台将图片发送给大模型视觉接口。

要求模型返回结构化 JSON：

```json
{
  "part_name": "TPS7A8300",
  "mpn": "TPS7A8300RGRR",
  "package_name": "VQFN-20-EP(3.5x3.5)",
  "pins": [
    {"number": 1, "name": "OUT", "side": "left"},
    {"number": 2, "name": "SNS", "side": "left"},
    {"number": 3, "name": "FB", "side": "left"}
  ],
  "exposed_pad": {
    "number": 21,
    "name": "EP_GND"
  }
}
```

识别结果必须进入人工校对界面，不允许直接无确认生成库文件。

### 4.4 人工校对界面

校对字段：

- pin number
- pin name
- pin side：left / right / top / bottom / hidden
- 是否 exposed pad
- 是否参与 `.c` logic pin 绘制
- 是否参与 `.p` pin 映射

支持 pin 名规范化：

```text
/CS          → CS_N
/DRDY        → DRDY_N
DOUT/DRDY    → DOUT_DRDY
RESET/PWDN   → RESET_PWDN_N
NR/SS        → NR_SS
1.6V         → 1V6
Thermal Pad  → EP_GND
```

### 4.5 PADS Logic 生成器

基于项目中的样板文件：

```text
IC_pack.c
IC_pack.p
```

生成两类文件：

```text
xxx_logic.c
xxx_pack.p
```

## 5. PADS 文件生成规则

### 5.1 `.c` 文件规则

`.c` 文件是 PADS Logic 的 SCH Decal / 原理图逻辑符号。

文件头：

```text
*PADS-LIBRARY-SCH-DECALS-V9*
```

基本结构：

```text
器件名  32000 32000 97 10 97 10 4 1 0 引脚数 0
TIMESTAMP yyyy.mm.dd.hh.mm.ss
"Default Font"
"Default Font"
REF-DES 坐标定义
PART-TYPE 坐标定义
*
*
CLOSED ...  矩形外框
T... PIN    pin 端子文字位置
P...        pin 引脚线
*END*
```

规则：

- `.c` 定义原理图符号外观和 pin 端子数量/位置。
- `.c` 不负责真实 pin name 和 pin number 的绑定。
- pin name / pin number 由 `.p` 文件定义。
- `.c` 第一行中的引脚数必须与 `T... PIN` / `P...` 组合数量一致。
- 常规矩形 IC 默认采用左/右两列 pin 布局。

左侧 pin 样式参考：

```text
T-200  y  0 0 140 20 0 2 230 0 0 16 PIN
P-520  0  0 2 -80 0 0 2 0
```

右侧 pin 样式参考：

```text
T1600  y  0 2 140 20 0 2 230 0 0 16 PIN
P-520  0  0 2 -80 0 0 2 0
```

### 5.2 `.p` 文件规则

`.p` 文件是 PADS Part Type，负责器件属性、PCB decal 绑定和 pin 表。

文件头：

```text
*PADS-LIBRARY-PART-TYPES-V9*
```

基本结构：

```text
器件名 PCB封装名 I TTL 4 1 0 0 0
TIMESTAMP yyyy.mm.dd.hh.mm.ss
"Description" 具体型号/描述
"Geometry.Height"
"Part Number" 物料编号
"Value" 器件值
GATE 1 引脚数 0
逻辑封装名
pin_number 0 L pin_name
...
*END*
```

绑定关系：

```text
.p 第一行的器件名 = Part Type
.p 第一行的 PCB封装名 = PCB Decal / Footprint 名称
GATE 后面的逻辑封装名 = .c 里的 SCH Decal 名
pin_number / pin_name = 原理图 pin 显示和网表映射
```

规则：

- `.p` 的 `GATE 1 N 0` 必须与 `.c` 中的 pin 数一致。
- `.p` pin 表数量必须等于 N。
- PCB decal 名称必须在 PADS Layout 的 decal 库中真实存在。
- pin name 建议使用 ASCII 安全字符，避免 `/`、`.`、空格、括号等字符。
- exposed pad 建议作为额外 pin，例如 `EP_GND`。

## 6. 后台数据结构

建议 Go 数据结构：

```go
type Pin struct {
    Number int    `json:"number"`
    Name   string `json:"name"`
    Side   string `json:"side"` // left, right, top, bottom, hidden
    Hidden bool   `json:"hidden"`
}

type PartInfo struct {
    PartName     string `json:"part_name"`
    MPN          string `json:"mpn"`
    LCSCNumber   string `json:"lcsc_number"`
    PackageName  string `json:"package_name"`
    PCBDecalName string `json:"pcb_decal_name"`
    Description  string `json:"description"`
    Value        string `json:"value"`
    Pins         []Pin  `json:"pins"`
}

type ModelConfig struct {
    BaseURL     string  `json:"base_url"`
    APIKey      string  `json:"api_key"`
    Model       string  `json:"model"`
    Temperature float64 `json:"temperature"`
    TimeoutSec  int     `json:"timeout_sec"`
}
```

## 7. API 设计

```text
GET /api/config
读取本地大模型配置

POST /api/config
保存本地大模型配置

POST /api/parse-image
上传芯片 pinout 截图，调用大模型识别 pin 表

POST /api/normalize-pins
规范化 pin name

POST /api/generate
根据人工确认后的 pin 表生成 .c / .p

GET /api/download/:file
下载生成结果
```

生成接口请求示例：

```json
{
  "part_name": "TPS7A8300",
  "mpn": "TPS7A8300RGRR",
  "lcsc_number": "C544500",
  "package_name": "VQFN-20-EP(3.5x3.5)",
  "pcb_decal_name": "VQFN-20-EP_3.5X3.5_RGR",
  "description": "TPS7A8300RGRR low-noise LDO, VQFN-20-EP 3.5x3.5",
  "value": "TPS7A8300",
  "pins": [
    {"number": 1, "name": "OUT", "side": "left"},
    {"number": 2, "name": "SNS", "side": "left"},
    {"number": 21, "name": "EP_GND", "side": "hidden"}
  ]
}
```

## 8. 工作流程

```text
1. 用户打开本地网页
2. 配置大模型 API
3. 拖入芯片 pinout 截图
4. 填写商品型号、商品编号、封装、PCB decal 名称
5. 点击识别
6. 后台调用大模型视觉 API
7. 前端显示 pin 表
8. 用户人工校对 pin number / pin name / exposed pad
9. 点击生成
10. 后台生成 xxx_logic.c 和 xxx_pack.p
11. 前端预览文本并导出文件
```

## 9. 第一版范围

第一版实现：

- 单芯片 pinout 图片上传
- 大模型识别 pin 表
- 人工编辑 pin 表
- pin name 自动规范化
- 生成 `.c` 和 `.p`
- 预览与导出
- 支持 exposed pad 作为额外 pin

第一版暂不实现：

- PCB footprint 几何生成
- 直接写入 PADS 库数据库
- 多 gate 器件拆分
- 运放多单元符号
- 自动校验 PADS Layout 中是否存在 PCB decal

## 10. 风险与约束

- 大模型 OCR/视觉识别存在误读风险，必须人工确认。
- datasheet top view / bottom view 方向必须人工核对。
- exposed pad 是否作为额外 pin，需按公司 PADS 库规范统一。
- `.p` 中 PCB decal 名称必须和 PADS Layout 库一致，否则网表导入会失败。
- pin name 中不建议保留 `/`、`.`、空格等字符，统一转为 PADS 兼容 ASCII 名称。
- 该工具生成的是 Logic 原理图库，不替代 PCB footprint 尺寸校核。

## 11. 后续升级

可扩展功能：

- 批量生成多个器件
- 读取 datasheet PDF 并自动截取 pinout 区域
- 集成本地 OCR 作为大模型前处理
- 支持多单元器件 gate 拆分
- 支持公司封装命名规范模板
- 支持导入现有 `.c/.p` 作为样式模板
- 增加 pin 类型：input / output / power / passive / nc
- 增加封装一致性检查报告
