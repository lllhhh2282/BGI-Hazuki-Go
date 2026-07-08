package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/asset"
	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/dsc"
)

var textFlags struct {
	decodeCP uint32
	encodeCP uint32
}

var textCmd = &cobra.Command{
	Use:   "text",
	Short: "DSC 脚本文本提取与应用",
	Long:  `提取 DSC 脚本中的可翻译文本为 .hazuki.txt 项目，或将修改后的项目应用回脚本。`,
}

var textExtractCmd = &cobra.Command{
	Use:   "extract <script> [output.hazuki.txt]",
	Short: "从 DSC 脚本提取文本项目",
	Example: `  hazuki text extract script
  hazuki text extract script output.hazuki.txt --decode-cp 932 --encode-cp 932`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		scriptPath := args[0]
		outputPath := dsc.GetDefaultProjectPath(scriptPath)
		if len(args) >= 2 {
			outputPath = args[1]
		}

		project, err := extractTextProject(scriptPath)
		if err != nil {
			return fmt.Errorf("text extract: failed to extract %s: %w", scriptPath, err)
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return fmt.Errorf("text extract: failed to create output directory: %w", err)
		}
		if err := dsc.SaveTextProject(project, scriptPath, outputPath); err != nil {
			return fmt.Errorf("text extract: failed to save %s: %w", outputPath, err)
		}
		fmt.Printf("%s -> %s\n", scriptPath, outputPath)
		return nil
	},
}

var textApplyCmd = &cobra.Command{
	Use:   "apply <project.hazuki.txt> [output_script]",
	Short: "将文本项目应用回 DSC 脚本",
	Example: `  hazuki text apply project.hazuki.txt
  hazuki text apply project.hazuki.txt patched_script --encode-cp 932`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectPath := args[0]
		scriptPath := dsc.InferScriptPathFromProject(projectPath)
		if scriptPath == "" {
			return fmt.Errorf("text apply: cannot infer script path from %s", projectPath)
		}

		project, err := dsc.LoadTextProject(projectPath)
		if err != nil {
			return fmt.Errorf("text apply: failed to load %s: %w", projectPath, err)
		}

		outputPath := dsc.GetDefaultPatchedPath(scriptPath)
		if len(args) >= 2 {
			outputPath = args[1]
		}

		encodeCP := textFlags.encodeCP
		if encodeCP == 0 {
			encodeCP = project.EncodeCodepage
		}

		layout, err := loadScriptLayout(scriptPath)
		if err != nil {
			return fmt.Errorf("text apply: failed to load script %s: %w", scriptPath, err)
		}

		if err := dsc.ApplyTextProjectToLayout(project, layout, outputPath, encodeCP); err != nil {
			return fmt.Errorf("text apply: failed to apply project: %w", err)
		}
		fmt.Printf("%s -> %s\n", projectPath, outputPath)
		return nil
	},
}

// extractTextProject reads a script (stripping any BSE wrapper) and extracts
// a translation project. If the decrypted payload is a DSC-embedded image, an
// error is returned so the caller can save it as PNG instead.
func extractTextProject(scriptPath string) (*dsc.TextProject, error) {
	data, info, err := asset.ReadAssetData(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("read script: %w", err)
	}

	compiled := data
	containerKind := dsc.RawCompiled
	if info.Kind == asset.DscScript {
		compiled, err = dsc.DecodeDscToCompiled(data)
		if err != nil {
			return nil, fmt.Errorf("decompress DSC: %w", err)
		}
		containerKind = dsc.DscCompressed
	}

	if dsc.IsDscImage(compiled) {
		return nil, fmt.Errorf("该 DSC 解密后是图片，不是脚本文本；请通过 unpack 解包后保存为 PNG")
	}

	layout, err := dsc.LoadScriptLayoutFromCompiledWithKind(compiled, containerKind)
	if err != nil {
		return nil, fmt.Errorf("parse script layout: %w", err)
	}

	return dsc.ExtractTextProjectFromLayout(layout, textFlags.decodeCP, textFlags.encodeCP)
}

// loadScriptLayout loads a script file, transparently stripping any BSE wrapper.
func loadScriptLayout(scriptPath string) (*dsc.ScriptLayout, error) {
	data, info, err := asset.ReadAssetData(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("read script: %w", err)
	}

	compiled := data
	containerKind := dsc.RawCompiled
	if info.Kind == asset.DscScript {
		compiled, err = dsc.DecodeDscToCompiled(data)
		if err != nil {
			return nil, fmt.Errorf("decompress DSC: %w", err)
		}
		containerKind = dsc.DscCompressed
	}

	return dsc.LoadScriptLayoutFromCompiledWithKind(compiled, containerKind)
}

func init() {
	textCmd.AddCommand(textExtractCmd)
	textCmd.AddCommand(textApplyCmd)

	textExtractCmd.Flags().Uint32Var(&textFlags.decodeCP, "decode-cp", 932, "解码代码页")
	textExtractCmd.Flags().Uint32Var(&textFlags.encodeCP, "encode-cp", 932, "编码代码页")
	textApplyCmd.Flags().Uint32Var(&textFlags.encodeCP, "encode-cp", 0, "编码代码页（默认使用项目中的编码）")
}
