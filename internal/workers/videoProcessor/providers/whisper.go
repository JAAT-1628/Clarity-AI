package providers

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func TranscribeWithLocalWhisper(ctx context.Context, audioFile string) (string, error) {
	log.Printf("🎙️ Starting local Whisper transcription for: %s", audioFile)
	if _, err := exec.LookPath("whisper"); err != nil {
		return "", fmt.Errorf("whisper CLI not found")
	}

	outputDir := "/tmp/whisper-output"
	os.MkdirAll(outputDir, 0755)

	cmd := exec.CommandContext(ctx,
		"whisper",
		audioFile,
		"--output_format", "txt",
		"--output_dir", outputDir,
		"--model", "tiny",
		"--language", "en",
		"--fp16", "False",
		"--no_speech_threshold", "0.1",
		"--condition_on_previous_text", "False",
	)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper command failed: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(audioFile), filepath.Ext(audioFile))
	transcriptPath := filepath.Join(outputDir, baseName+".txt")

	transcriptBytes, err := os.ReadFile(transcriptPath)
	if err != nil {
		return "", fmt.Errorf("failed to read transcript file: %w", err)
	}

	transcript := strings.TrimSpace(string(transcriptBytes))
	os.Remove(transcriptPath)

	return transcript, nil
}
