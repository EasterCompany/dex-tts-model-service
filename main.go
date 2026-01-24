package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ServiceName    = "dex-tts-service"
	PiperUrl       = "https://github.com/rhasspy/piper/releases/download/2023.11.14-2/piper_linux_x86_64.tar.gz"
	VoiceModelUrl  = "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_GB/northern_english_male/medium/en_GB-northern_english_male-medium.onnx"
	VoiceConfigUrl = "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_GB/northern_english_male/medium/en_GB-northern_english_male-medium.onnx.json"
)

var (
	version   = "0.0.0"
	branch    = "unknown"
	commit    = "unknown"
	buildDate = "unknown"
	arch      = runtime.GOARCH
	startTime = time.Now()

	redisClient *redis.Client
	mu          sync.Mutex
	isReady     = false
)

type GenerateRequest struct {
	Text       string `json:"text"`
	OutputPath string `json:"output_path,omitempty"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("%s.%s.%s.%s.%s\n", version, branch, commit, buildDate, arch)
		os.Exit(0)
	}

	setupRedis()

	// Async setup to not block startup, but handleGenerate will wait if not ready
	go func() {
		if err := ensureAssets(); err != nil {
			log.Printf("Asset setup failed: %v", err)
		} else {
			mu.Lock()
			isReady = true
			mu.Unlock()
			log.Println("TTS Assets ready.")
		}
	}()

	http.HandleFunc("/generate", handleGenerate)
	http.HandleFunc("/hibernate", handleHibernate)
	http.HandleFunc("/wakeup", handleWakeup)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/service", handleService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8200"
	}

	log.Printf("Starting Dexter TTS Service (Piper-Go) on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func setupRedis() {
	redisClient = redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
}

func ensureAssets() error {
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, "Dexter", "bin")
	piperBin := filepath.Join(binDir, "piper", "piper")
	modelsDir := filepath.Join(home, "Dexter", "models", "piper")
	voicePath := filepath.Join(modelsDir, "en_GB-northern_english_male-medium.onnx")
	configPath := filepath.Join(modelsDir, "en_GB-northern_english_male-medium.onnx.json")

	// 1. Piper Binary
	if _, err := os.Stat(piperBin); os.IsNotExist(err) {
		log.Println("Downloading and installing Piper binary...")
		if err := downloadAndExtract(PiperUrl, binDir); err != nil {
			return err
		}
	}

	// 2. Voice Assets
	_ = os.MkdirAll(modelsDir, 0755)
	if _, err := os.Stat(voicePath); os.IsNotExist(err) {
		log.Println("Downloading Northern English male voice model...")
		if err := downloadFile(VoiceModelUrl, voicePath); err != nil {
			return err
		}
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := downloadFile(VoiceConfigUrl, configPath); err != nil {
			return err
		}
	}

	return nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, resp.Body)
	return err
}

func downloadAndExtract(url, destDir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	uncompressed, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer func() { _ = uncompressed.Close() }()

	archive := tar.NewReader(uncompressed)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, archive); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
		}
	}
	return nil
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	ready := isReady
	mu.Unlock()

	if !ready {
		http.Error(w, "TTS engine initializing", http.StatusServiceUnavailable)
		return
	}

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	hash := md5.Sum([]byte(req.Text))
	cacheKey := "tts:cache:" + hex.EncodeToString(hash[:])

	if redisClient != nil {
		if val, err := redisClient.Get(r.Context(), cacheKey).Bytes(); err == nil {
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(val)
			return
		}
	}

	home, _ := os.UserHomeDir()
	piperBin := filepath.Join(home, "Dexter", "bin", "piper", "piper")
	voicePath := filepath.Join(home, "Dexter", "models", "piper", "en_GB-northern_english_male-medium.onnx")

	cmd := exec.Command(piperBin, "--model", voicePath, "--output_file", "-")
	cmd.Stdin = strings.NewReader(req.Text)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("Piper Error: %v, Stderr: %s", err, stderr.String())
		http.Error(w, "Generation failed", http.StatusInternalServerError)
		return
	}

	if redisClient != nil {
		redisClient.Set(r.Context(), cacheKey, out.Bytes(), 48*time.Hour)
	}

	w.Header().Set("Content-Type", "audio/wav")
	_, _ = w.Write(out.Bytes())
}

func handleHibernate(w http.ResponseWriter, r *http.Request) {
	// Piper is process-based, no persistent VRAM usage when idle.
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","message":"process-idle"}`))
}

func handleWakeup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","message":"ready"}`))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	ready := isReady
	mu.Unlock()
	if !ready {
		http.Error(w, "initializing", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("OK"))
}

func handleService(w http.ResponseWriter, r *http.Request) {
	vParts := strings.Split(version, ".")
	major, minor, patch := "0", "0", "0"
	if len(vParts) >= 3 {
		major, minor, patch = vParts[0], vParts[1], vParts[2]
	}

	report := map[string]interface{}{
		"version": map[string]interface{}{
			"str": fmt.Sprintf("%s.%s.%s.%s.%s", version, branch, commit, buildDate, arch),
			"obj": map[string]interface{}{
				"major": major, "minor": minor, "patch": patch,
				"branch": branch, "commit": commit, "build_date": buildDate, "arch": arch,
			},
		},
		"health": map[string]interface{}{
			"status": "OK",
			"uptime": time.Since(startTime).String(),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}
