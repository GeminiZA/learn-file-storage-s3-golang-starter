package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	outBytes, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var st struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	err = json.Unmarshal(outBytes, &st)
	if err != nil {
		return "", err
	}
	if len(st.Streams) < 1 {
		return "", fmt.Errorf("no streams in video output")
	}
	ratio := float64(st.Streams[0].Width) / float64(st.Streams[0].Height)
	out := "other"
	if math.Abs(ratio-16.0/9.0) < 0.01 {
		out = "16:9"
	} else if math.Abs(ratio-9.0/16.0) < 0.01 {
		out = "9:16"
	}
	return out, nil
}

func processVideoForFastStart(filePath string) (string, error) {
	procFilePath := fmt.Sprintf("%s.processing", filePath)
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", procFilePath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return procFilePath, nil
}
