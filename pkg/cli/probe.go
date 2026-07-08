package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/asset"
)

var probeCmd = &cobra.Command{
	Use:   "probe <file/dir>...",
	Short: "探测文件类型并给出建议扩展名",
	Long:  `递归遍历输入路径，对每个文件识别其 BGI 资源类型。`,
	Example: `  hazuki probe file.cbg
  hazuki probe ./assets`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, input := range args {
			if err := probePath(input); err != nil {
				return err
			}
		}
		return nil
	},
}

func probePath(input string) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				printProbe(path)
			}
			return nil
		})
	}
	printProbe(input)
	return nil
}

func printProbe(path string) {
	info, err := asset.ProbeAsset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s -> error: %v\n", path, err)
		return
	}
	fmt.Printf("%s -> %s (suggest %s)\n", path, info.Label, info.SuggestedExtension)
}
