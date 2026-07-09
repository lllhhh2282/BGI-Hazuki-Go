# BGI-Hazuki-Go

BGI-Hazuki-Go 是一个用于处理 **BGI（Buriko General Interpreter）** 引擎资源的命令行工具，使用 Go 语言编写。

它支持对 BGI 游戏常见的 ARC、CBG、DSC、BW 等格式进行解包、转换和文本处理，方便汉化、资源提取与二次开发。

## 功能特性

- **ARC 解包**：支持 `BURIKO ARC20`（v2）与旧版 `PackFile`（v1）两种格式，自动递归提取文件。
- **BSE 自动解密**：CBG、DSC 等条目外层的 `BSE 1.0` 加密会在解包/转换时自动剥离。
- **CBG 图片转换**：在 `CBG`（CompressedBG）与 `PNG` 之间双向转换。
- **DSC 脚本处理**：从 DSC 脚本或可执行编译脚本中提取可翻译文本，生成 `.hazuki.txt` 项目文件；也能将修改后的文本应用回脚本。如果 DSC 解密后是图片，会自动保存为 PNG。
- **BGI 音频提取**：从 `bw  ` 容器中提取裸 `OGG` 数据。
- **未知文件原样落盘**：解包时无法识别的文件会保持原样输出，而不是被丢弃。
- **资源探测**：通过 `probe` 命令快速识别文件类型并给出建议扩展名（支持 BSE 包裹文件）。
- **代码页支持**：默认使用 Shift-JIS（CP932）处理脚本文本，可通过命令行参数调整。

## 安装

需要 Go 1.26.3 或更高版本：

```bash
go install github.com/lllhhh2282/BGI-Hazuki-Go/cmd/hazuki@latest
```

也可以直接在 [Releases](https://github.com/lllhhh2282/BGI-Hazuki-Go/releases) 页面下载已编译好的跨平台二进制文件。

## 使用方法

### 查看帮助

```bash
hazuki --help
```

### 解包 ARC 档案

```bash
hazuki unpack game.arc -o ./output
```

会自动剥离 BSE 加密，将 CBG 转为 PNG、DSC 脚本提取为 `.hazuki.txt`（若 DSC 内嵌图片则保存为 PNG）、BW 音频提取为 OGG；无法识别的文件会原样保留。

### 图片转换

```bash
hazuki img image.cbg      # 解码为 image.png
hazuki img image.png      # 编码为 image.cbg
hazuki img ./cg           # 递归处理目录
```

### 音频提取

```bash
hazuki audio bgm.bw
```

### 脚本文本提取与应用

```bash
# 提取
hazuki text extract script output.hazuki.txt --decode-cp 932 --encode-cp 932

# 应用
hazuki text apply output.hazuki.txt script.patched --encode-cp 932
```

### 探测文件类型

```bash
hazuki probe file.cbg
hazuki probe ./assets
```

## 构建

项目根目录提供了 `build.sh`，可交叉编译为 Windows、Linux、macOS 的 amd64/arm64 二进制文件：

```bash
bash build.sh
```

编译结果会输出到 `dist/` 目录。

## 项目结构

```
cmd/hazuki/        # 程序入口
internal/
  binutil/         # 二进制读写、位流、VarInt 工具
  bse/             # BSE 1.0 解密
  codec/           # RLE、LZ、Huffman 编解码
pkg/
  arc/             # ARC20 档案解析
  asset/           # 文件类型探测
  audio/           # BGI 音频容器解析
  cbg/             # CBG 图片编解码
  cli/             # Cobra 命令行
  dsc/             # DSC 脚本文本处理
  pipeline/        # 一键解包+转换流程
dist/              # 预编译二进制文件
build.sh           # 交叉编译脚本
go.mod             # Go 模块定义
```

## 依赖

- [spf13/cobra](https://github.com/spf13/cobra)：命令行框架
- [golang.org/x/text](https://pkg.go.dev/golang.org/x/text)：Shift-JIS 编码支持

## 许可证

本项目采用 [Unlicense](LICENSE) 许可证。
