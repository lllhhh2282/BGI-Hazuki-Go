// Package pipeline implements the full BGI ARC unpacking and conversion pipeline.
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/arc"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/asset"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/audio"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/cbg"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/dsc"
)

// Options configures the unpacking pipeline.
type Options struct {
	UnpackRoot     string
	DecodeCodepage uint32
	EncodeCodepage uint32
}

// Result reports the outcome of a pipeline run.
type Result struct {
	UnpackRoot     string
	ArchiveCount   int
	ExtractedCount int
	ProcessedCount int
	ConvertedCount int
	SkippedCount   int
}

// Progress describes the current pipeline progress.
type Progress struct {
	Completed   int
	Total       int
	CurrentItem string
}

// Callbacks receives pipeline events.
type Callbacks struct {
	OnLog      func(line string)
	OnProgress func(p Progress)
}

// RunFullUnpackPipeline collects ARC archives from inputs, extracts them and
// converts supported assets into easier-to-edit formats.
func RunFullUnpackPipeline(inputs []string, opts Options, cb Callbacks) (Result, error) {
	if len(inputs) == 0 {
		return Result{}, fmt.Errorf("pipeline: no inputs provided")
	}

	arcFiles, err := collectArcFiles(inputs)
	if err != nil {
		return Result{}, err
	}
	if len(arcFiles) == 0 {
		return Result{}, fmt.Errorf("pipeline: no valid .arc files were provided")
	}

	root := opts.UnpackRoot
	if root == "" {
		root = filepath.Join(".", "unpack")
	}
	root = filepath.Clean(root)

	emitLog(cb, "[ROOT] 输出目录: %s", root)
	emitLog(cb, "[SCAN] 发现可处理 ARC 文件: %d", len(arcFiles))

	if err := os.MkdirAll(root, 0o755); err != nil {
		return Result{}, fmt.Errorf("pipeline: failed to create root directory: %w", err)
	}

	archiveInfos := make([]*arc.ArchiveInfo, 0, len(arcFiles))
	totalEntries := 0
	for _, arcPath := range arcFiles {
		info, err := arc.ReadArcArchiveInfo(arcPath)
		if err != nil {
			return Result{}, fmt.Errorf("pipeline: failed to read %s: %w", arcPath, err)
		}
		totalEntries += len(info.Entries)
		emitLog(cb, "[ARC] %s : %d 个文件", arcPath, len(info.Entries))
		archiveInfos = append(archiveInfos, info)
	}

	totalWork := totalEntries * 2
	progressValue := 0
	emitProgress(cb, progressValue, totalWork, "等待开始")

	result := Result{
		UnpackRoot:   root,
		ArchiveCount: len(archiveInfos),
	}
	usedNames := make(map[string]struct{})

	for _, info := range archiveInfos {
		outDir := makeUniqueOutputDir(root, info.ArcPath, usedNames)
		if _, err := os.Stat(outDir); err == nil {
			_ = os.RemoveAll(outDir)
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return result, fmt.Errorf("pipeline: failed to create output directory: %w", err)
		}
		emitLog(cb, "[UNPACK] %s -> %s", info.ArcPath, outDir)

		extractedPaths, err := arc.ExtractArcArchive(info.ArcPath, outDir, func(entry arc.Entry, outputPath string, index, total int) {
			result.ExtractedCount++
			progressValue++
			emitProgress(cb, progressValue, totalWork, fmt.Sprintf("解包 %s", entry.RelativePath))
		})
		if err != nil {
			return result, fmt.Errorf("pipeline: failed to extract %s: %w", info.ArcPath, err)
		}

		for _, path := range extractedPaths {
			processExtractedFile(path, opts, &result, cb, &progressValue, &totalWork)
		}
	}

	emitLog(cb, "[DONE] ARC=%d, extracted=%d, processed=%d, converted=%d, skipped=%d",
		result.ArchiveCount, result.ExtractedCount, result.ProcessedCount, result.ConvertedCount, result.SkippedCount)
	emitProgress(cb, totalWork, totalWork, "完成")

	return result, nil
}

func collectArcFiles(inputs []string) ([]string, error) {
	var out []string
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			continue
		}
		if info.IsDir() {
			err := filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					return nil
				}
				if strings.EqualFold(filepath.Ext(path), ".arc") {
					if ok, _ := arc.IsBurikoArcFile(path); ok {
						out = append(out, path)
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		if strings.EqualFold(filepath.Ext(input), ".arc") {
			if ok, _ := arc.IsBurikoArcFile(input); ok {
				out = append(out, input)
			}
		}
	}

	// Sort and deduplicate by absolute path so the same archive is only processed once.
	absMap := make(map[string]string)
	for _, p := range out {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if _, ok := absMap[abs]; !ok {
			absMap[abs] = p
		}
	}
	unique := make([]string, 0, len(absMap))
	for _, p := range absMap {
		unique = append(unique, p)
	}
	sort.Strings(unique)
	return unique, nil
}

func makeUniqueOutputDir(root, arcPath string, used map[string]struct{}) string {
	base := filepath.Base(arcPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" {
		base = "archive"
	}
	candidate := base
	suffix := 2
	for {
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return filepath.Join(root, candidate)
		}
		candidate = fmt.Sprintf("%s_%d", base, suffix)
		suffix++
	}
}

func processExtractedFile(path string, opts Options, result *Result, cb Callbacks, progressValue, totalWork *int) {
	currentItem := fmt.Sprintf("处理 %s", filepath.Base(path))
	done := false
	defer func() {
		if !done {
			advanceProgress(cb, result, progressValue, totalWork, currentItem)
			done = true
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			result.SkippedCount++
			emitLog(cb, "[ERROR] %s -> 转换失败: %v", path, r)
		}
	}()

	data, info, err := asset.ReadAssetData(path)
	if err != nil {
		emitLog(cb, "[SKIP] %s -> probe error: %v", path, err)
		result.SkippedCount++
		return
	}

	switch info.Kind {
	case asset.CbgImage:
		emitLog(cb, "[CBG] 解析 %s (文件大小: %d 字节)", path, len(data))
		img, err := cbg.DecodeCbgBytes(data)
		if err != nil {
			emitLog(cb, "[ERROR] %s -> CBG 解码失败: %v", path, err)
			result.SkippedCount++
			return
		}
		outputPath := path + ".png"
		if err := cbg.SavePng(img, outputPath); err != nil {
			emitLog(cb, "[ERROR] %s -> PNG 保存失败: %v", path, err)
			result.SkippedCount++
			return
		}
		emitLog(cb, "[CBG->PNG] %s -> %s", path, outputPath)
		result.ConvertedCount++
		if err := os.Remove(path); err == nil {
			emitLog(cb, "[CLEAN] %s", path)
		}

	case asset.DscScript, asset.RawCompiledScript:
		compiled := data
		if info.Kind == asset.DscScript {
			compiled, err = dsc.DecodeDscToCompiled(data)
			if err != nil {
				emitLog(cb, "[ERROR] %s -> DSC 解密失败: %v", path, err)
				result.SkippedCount++
				return
			}
		}

		if dsc.IsDscImage(compiled) {
			if err := dsc.SaveDscImageAsPng(compiled, path); err != nil {
				emitLog(cb, "[ERROR] %s -> DSC 图片保存失败: %v", path, err)
				result.SkippedCount++
				return
			}
			emitLog(cb, "[DSC->PNG] %s -> %s.png", path, path)
			result.ConvertedCount++
			return
		}

		layout, err := dsc.LoadScriptLayoutFromCompiled(compiled)
		if err != nil {
			emitLog(cb, "[ERROR] %s -> 脚本布局解析失败: %v", path, err)
			result.SkippedCount++
			return
		}
		project, err := dsc.ExtractTextProjectFromLayout(layout, opts.DecodeCodepage, opts.EncodeCodepage)
		if err != nil {
			emitLog(cb, "[ERROR] %s -> 文本提取失败: %v", path, err)
			result.SkippedCount++
			return
		}
		outputPath := dsc.GetDefaultProjectPath(path)
		if err := dsc.SaveTextProject(project, path, outputPath); err != nil {
			emitLog(cb, "[ERROR] %s -> 项目保存失败: %v", path, err)
			result.SkippedCount++
			return
		}
		emitLog(cb, "[SCRIPT->TXT] %s -> %s", path, outputPath)
		result.ConvertedCount++

	case asset.BgiAudio:
		outputPath := path + ".ogg"
		if err := audio.ExtractBgiAudioBytesToOgg(data, outputPath); err != nil {
			emitLog(cb, "[ERROR] %s -> 音频提取失败: %v", path, err)
			result.SkippedCount++
			return
		}
		emitLog(cb, "[BW->OGG] %s -> %s", path, outputPath)
		result.ConvertedCount++

	default:
		// Unknown / unsupported: write the (possibly BSE-stripped) bytes as-is.
		if err := os.WriteFile(path, data, 0o644); err != nil {
			emitLog(cb, "[ERROR] %s -> 原样写入失败: %v", path, err)
			result.SkippedCount++
			return
		}
		emitLog(cb, "[RAW] %s -> 原样输出 (%d 字节)", path, len(data))
	}
}

func advanceProgress(cb Callbacks, result *Result, progressValue, totalWork *int, currentItem string) {
	result.ProcessedCount++
	(*progressValue)++
	emitProgress(cb, *progressValue, *totalWork, currentItem)
}

func emitLog(cb Callbacks, format string, args ...any) {
	if cb.OnLog != nil {
		cb.OnLog(fmt.Sprintf(format, args...))
	}
}

func emitProgress(cb Callbacks, completed, total int, currentItem string) {
	if cb.OnProgress != nil {
		cb.OnProgress(Progress{
			Completed:   completed,
			Total:       total,
			CurrentItem: currentItem,
		})
	}
}
