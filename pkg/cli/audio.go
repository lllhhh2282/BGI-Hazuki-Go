package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lllhhh2282/BGI-Hazuki-Go/pkg/audio"
)

var audioCmd = &cobra.Command{
	Use:   "audio <bw-file>...",
	Short: "从 BGI 音频容器提取 OGG",
	Long:  `将 BGI "bw  " 容器中的裸 OGG 数据提取为 .ogg 文件。`,
	Example: `  hazuki audio bgm.bw
  hazuki audio ./bgm`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, input := range args {
			if err := processAudio(input); err != nil {
				return err
			}
		}
		return nil
	},
}

func processAudio(input string) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return extractAudio(path)
		})
	}
	return extractAudio(input)
}

func extractAudio(path string) error {
	ok, err := audio.IsBgiAudioFile(path)
	if err != nil {
		return fmt.Errorf("audio: failed to probe %s: %w", path, err)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "[SKIP] %s -> not a BGI audio file\n", path)
		return nil
	}
	outputPath := path + ".ogg"
	if err := audio.ExtractBgiAudioToOgg(path, outputPath); err != nil {
		return fmt.Errorf("audio: failed to extract %s: %w", path, err)
	}
	fmt.Printf("%s -> %s\n", path, outputPath)
	return nil
}
