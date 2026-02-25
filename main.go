package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-go-utils/network"
	"github.com/EasterCompany/dex-tts-service/utils"
)

const (
	ServiceName = "dex-tts-service"
)

var (
	// Version info injected by build
	version   = "0.0.0"
	branch    = "unknown"
	commit    = "unknown"
	buildDate = "unknown"
	arch      = "unknown"

	mu       sync.Mutex
	isReady  = false
	noWarmup = false
)

type GenerateRequest struct {
	Text       string `json:"text"`
	Language   string `json:"language"`
	OutputPath string `json:"output_path"`
}

func main() {
	utils.SetVersion(version, branch, commit, buildDate, arch)

	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(utils.GetVersion().Str)
		os.Exit(0)
	}

	flag.BoolVar(&noWarmup, "no-warmup", false, "Skip model warmup on boot")
	flag.Parse()

	// Async setup
	go func() {
		home, _ := os.UserHomeDir()
		ttsBin := filepath.Join(home, "Dexter", "bin", "tts", "dex-net-tts")
		voicePath := filepath.Join(home, "Dexter", "models", "dex-net-tts.onnx")

		// Poll for assets (provisioned by dex-cli)
		for i := 0; i < 60; i++ {
			if _, err := os.Stat(ttsBin); err == nil {
				if _, err := os.Stat(voicePath); err == nil {
					mu.Lock()
					isReady = true
					mu.Unlock()
					log.Println("TTS Assets verified and ready.")
					return
				}
			}
			time.Sleep(2 * time.Second)
		}
		log.Println("Warning: TTS Assets not found after 2 minutes. Service will remain in initializing state.")
	}()

	http.HandleFunc("/generate", handleGenerate)
	http.HandleFunc("/hibernate", handleHibernate)
	http.HandleFunc("/wakeup", handleWakeup)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/service", handleService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "24402"
	}

	// Determine Binding Address
	bindAddr := network.GetBestBindingAddress()
	addr := fmt.Sprintf("%s:%s", bindAddr, port)

	log.Printf("Starting Dexter TTS Service on %s", addr)
	utils.SetHealthStatus("OK", "Service is running")
	if err := http.ListenAndServe(addr, network.AuthMiddleware(http.DefaultServeMux)); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	ready := isReady
	mu.Unlock()

	if !ready {
		http.Error(w, "TTS engine initializing (waiting for assets)", http.StatusServiceUnavailable)
		return
	}

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, "Dexter", "bin", "tts")
	ttsBin := filepath.Join(binDir, "dex-net-tts")
	voicePath := filepath.Join(home, "Dexter", "models", "dex-net-tts.onnx")

	if req.OutputPath == "" {
		req.OutputPath = fmt.Sprintf("/tmp/tts-%d.wav", time.Now().UnixNano())
	}

	log.Printf("TTS: [EXEC] Generating audio for: \"%.50s...\" (2m timeout)", req.Text)

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// dex-net-tts -m <model> -f <output>
	cmd := exec.CommandContext(ctx, ttsBin, "-m", voicePath, "-f", req.OutputPath)

	// Ensure engine can find its own libs in the same dir
	cmd.Env = append(os.Environ(), fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", binDir, os.Getenv("LD_LIBRARY_PATH")))

	cmd.Stdin = strings.NewReader(req.Text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("TTS: [ERROR] Timeout exceeded (2m)")
		} else {
			log.Printf("TTS: [ERROR] Neural TTS Kernel failed: %v, Stderr: %s", err, stderr.String())
		}
		http.Error(w, "Generation failed", http.StatusInternalServerError)
		return
	}

	log.Printf("TTS: [SUCCESS] Audio generated at %s", req.OutputPath)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"file_path": req.OutputPath,
	})
}

func handleHibernate(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"status":"ok","message":"process-idle"}`))
}

func handleWakeup(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"status":"ok","message":"ready"}`))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("OK"))
}

func handleService(w http.ResponseWriter, r *http.Request) {
	report := utils.ServiceReport{
		Version: utils.GetVersion(),
		Health:  utils.GetHealth(),
		Metrics: utils.GetMetrics().ToMap(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}
