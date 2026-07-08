// Package cli implements the hazuki command-line interface using cobra.
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

// rootCmd is the entry point for all hazuki subcommands.
var rootCmd = &cobra.Command{
	Use:   "hazuki",
	Short: "BGI-Hazuki-Go 资源处理工具",
	Long: `BGI-Hazuki-Go 是一个用于处理 BGI (Buriko General Interpreter) 引擎资源的命令行工具。
支持 ARC 解包、CBG 图片转换、DSC 脚本文本提取/应用以及 BGI 音频提取。`,
}

// Execute adds all child commands and runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(unpackCmd)
	rootCmd.AddCommand(probeCmd)
	rootCmd.AddCommand(imageCmd)
	rootCmd.AddCommand(textCmd)
	rootCmd.AddCommand(audioCmd)
	rootCmd.AddCommand(versionCmd)
}
