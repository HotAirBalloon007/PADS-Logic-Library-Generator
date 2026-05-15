package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	webview2 "github.com/jchv/go-webview2"
)

//go:embed web/*
var webFS embed.FS

const (
	configFile = "config.json"
	outputDir  = "output"
	webDir     = "web"
	logDir     = "logs"
)

type Pin struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	Side   string `json:"side"`
	Hidden bool   `json:"hidden"`
}

type PartInfo struct {
	PartName     string `json:"part_name"`
	MPN          string `json:"mpn"`
	LCSCNumber   string `json:"lcsc_number"`
	PackageName  string `json:"package_name"`
	PCBDecalName string `json:"pcb_decal_name"`
	Description  string `json:"description"`
	Value        string `json:"value"`
	Pins         []Pin  `json:"pins"`
}

type ModelConfig struct {
	BaseURL     string  `json:"base_url"`
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	VisionModel string  `json:"vision_model"`
	Temperature float64 `json:"temperature"`
	TimeoutSec  int     `json:"timeout_sec"`
}

type GenerateResponse struct {
	LogicFile   string `json:"logic_file"`
	PartFile    string `json:"part_file"`
	LogicText   string `json:"logic_text"`
	PartText    string `json:"part_text"`
	GeneratedAt string `json:"generated_at"`
}

type ParseResponse struct {
	Part PartInfo `json:"part"`
	Raw  string   `json:"raw"`
}

type ModelListResponse struct {
	Models []string `json:"models"`
}

func init() {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SetProcessDpiAwarenessContext")
	ret, _, _ := proc.Call(uintptr(^uintptr(3)))
	if ret == 0 {
		proc2 := user32.NewProc("SetProcessDPIAware")
		proc2.Call()
	}
}

func main() {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := initLogger(); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/models", handleModels)
	mux.HandleFunc("/api/test-model", handleTestModel)
	mux.HandleFunc("/api/test-vision", handleTestVision)
	mux.HandleFunc("/api/normalize-pins", handleNormalizePins)
	mux.HandleFunc("/api/generate", handleGenerate)
	mux.HandleFunc("/api/parse-image", handleParseImage)
	mux.HandleFunc("/api/parse-material", handleParseMaterial)
	mux.HandleFunc("/api/download/", handleDownload)
	mux.Handle("/web/", http.FileServer(http.FS(webFS)))

	addr := "127.0.0.1:18080"
	url := "http://" + addr
	if isServerAlreadyRunning(url) {
		if os.Getenv("PADSLOGIC_NO_WINDOW") != "1" {
			_ = runWebViewWindow(url)
		}
		return
	}
	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("PADS Logic generator listening on %s", url)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	time.Sleep(500 * time.Millisecond)
	select {
	case err := <-errCh:
		log.Fatal(err)
	default:
	}
	if !waitForServer(url, 5*time.Second) {
		log.Printf("server did not become ready before launching window")
	}

	if os.Getenv("PADSLOGIC_NO_WINDOW") != "1" {
		if err := runWebViewWindow(url); err != nil {
			log.Printf("webview window failed: %v", err)
			_ = server.Close()
			log.Fatal(err)
		}
		_ = server.Close()
		return
	}

	if err := <-errCh; err != nil {
		log.Fatal(err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func isServerAlreadyRunning(url string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(url + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func waitForServer(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isServerAlreadyRunning(url) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func runWebViewWindow(url string) error {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "PADS Logic 封装生成器",
			Width:  1280,
			Height: 900,
			Center: true,
		},
	})
	if w == nil {
		return errors.New("创建窗口失败，请确认已安装 Microsoft Edge WebView2 Runtime")
	}
	defer w.Destroy()
	w.SetSize(1280, 900, webview2.HintNone)
	w.Navigate(url)
	w.Run()
	return nil
}

func initLogger() error {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, "app.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	return nil
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := loadConfig()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if cfg.TimeoutSec == 0 {
			cfg.TimeoutSec = 60
		}
		writeJSON(w, cfg)
	case http.MethodPost:
		var cfg ModelConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if cfg.TimeoutSec <= 0 {
			cfg.TimeoutSec = 60
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		if err := os.WriteFile(configFile, data, 0600); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var cfg ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 30
	}
	models, err := fetchModelList(cfg)
	if err != nil {
		log.Printf("fetch models failed base_url=%s err=%v", cfg.BaseURL, err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("fetch models ok base_url=%s count=%d", cfg.BaseURL, len(models))
	writeJSON(w, ModelListResponse{Models: models})
}

func handleTestModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var cfg ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 30
	}
	start := time.Now()
	reply, err := callTextModel(cfg, "Reply with exactly: PADS_OK")
	if err != nil {
		log.Printf("test-model failed model=%s elapsed=%s err=%v", cfg.Model, time.Since(start), err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	elapsed := time.Since(start)
	log.Printf("test-model ok model=%s elapsed=%s", cfg.Model, elapsed)
	writeJSON(w, map[string]string{
		"status":     "ok",
		"reply":      strings.TrimSpace(reply),
		"elapsed_ms": strconv.FormatInt(elapsed.Milliseconds(), 10),
	})
}

func handleTestVision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var cfg ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 60
	}
	img, err := makeTestVisionPNG()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	visionCfg := cfg.forVision()
	start := time.Now()
	raw, err := callImageTextModel(visionCfg, "image/png", img, "If the image input is accepted, reply with exactly: VISION_OK")
	if err != nil {
		log.Printf("test-vision failed model=%s elapsed=%s err=%v", visionCfg.Model, time.Since(start), err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	elapsed := time.Since(start)
	log.Printf("test-vision ok model=%s elapsed=%s", visionCfg.Model, elapsed)
	writeJSON(w, map[string]string{
		"status":     "ok",
		"reply":      strings.TrimSpace(raw),
		"elapsed_ms": strconv.FormatInt(elapsed.Milliseconds(), 10),
	})
}

func makeTestVisionPNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	bg := color.RGBA{R: 248, G: 250, B: 252, A: 255}
	blue := color.RGBA{R: 14, G: 116, B: 144, A: 255}
	orange := color.RGBA{R: 234, G: 88, B: 12, A: 255}
	dark := color.RGBA{R: 30, G: 41, B: 59, A: 255}
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, bg)
		}
	}
	for y := 30; y < 75; y++ {
		for x := 30; x < 290; x++ {
			img.Set(x, y, blue)
		}
	}
	for y := 105; y < 150; y++ {
		for x := 30; x < 160; x++ {
			img.Set(x, y, orange)
		}
		for x := 190; x < 290; x++ {
			img.Set(x, y, dark)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func handleNormalizePins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var part PartInfo
	if err := json.NewDecoder(r.Body).Decode(&part); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	normalizePart(&part)
	writeJSON(w, part)
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var part PartInfo
	if err := json.NewDecoder(r.Body).Decode(&part); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	normalizePart(&part)
	if err := validatePart(part); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	logicText := generateLogicDecal(part)
	partText := generatePartType(part)

	prefix := safeIdent(part.PartName)
	if prefix == "" {
		prefix = "PART"
	}
	logicFile := prefix + "_logic.c"
	partFile := prefix + "_pack.p"

	if err := os.WriteFile(filepath.Join(outputDir, logicFile), []byte(logicText), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(filepath.Join(outputDir, partFile), []byte(partText), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, GenerateResponse{
		LogicFile:   logicFile,
		PartFile:    partFile,
		LogicText:   logicText,
		PartText:    partText,
		GeneratedAt: time.Now().Format(time.RFC3339),
	})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/download/")
	name = filepath.Base(name)
	if name == "." || name == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(outputDir, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	http.ServeFile(w, r, path)
}

func handleParseImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(24 << 20); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := configFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	img, err := readMultipartFile(file, header, 20<<20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	seed := PartInfo{
		PartName:     r.FormValue("part_name"),
		MPN:          r.FormValue("mpn"),
		LCSCNumber:   r.FormValue("lcsc_number"),
		PackageName:  r.FormValue("package_name"),
		PCBDecalName: r.FormValue("pcb_decal_name"),
		Description:  r.FormValue("description"),
		Value:        r.FormValue("value"),
	}
	visionCfg := cfg.forVision()
	start := time.Now()
	part, raw, err := callVisionModel(visionCfg, img.mime, img.data, seed)
	if err != nil {
		log.Printf("parse-image failed model=%s size=%d elapsed=%s err=%v", visionCfg.Model, len(img.data), time.Since(start), err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("parse-image ok model=%s size=%d pins=%d elapsed=%s", visionCfg.Model, len(img.data), len(part.Pins), time.Since(start))
	mergeSeed(&part, seed)
	normalizePart(&part)
	writeJSON(w, ParseResponse{Part: part, Raw: raw})
}

func handleParseMaterial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := configFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	img, err := readMultipartFile(file, header, 10<<20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	seed := PartInfo{
		PartName:     r.FormValue("part_name"),
		MPN:          r.FormValue("mpn"),
		LCSCNumber:   r.FormValue("lcsc_number"),
		PackageName:  r.FormValue("package_name"),
		PCBDecalName: r.FormValue("pcb_decal_name"),
		Description:  r.FormValue("description"),
		Value:        r.FormValue("value"),
	}
	visionCfg := cfg.forVision()
	start := time.Now()
	part, raw, err := callMaterialModel(visionCfg, img.mime, img.data, seed)
	if err != nil {
		log.Printf("parse-material failed model=%s size=%d elapsed=%s err=%v", visionCfg.Model, len(img.data), time.Since(start), err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	log.Printf("parse-material ok model=%s size=%d elapsed=%s", visionCfg.Model, len(img.data), time.Since(start))
	mergeSeed(&part, seed)
	normalizePart(&part)
	writeJSON(w, ParseResponse{Part: part, Raw: raw})
}

func configFromRequest(r *http.Request) (ModelConfig, error) {
	cfg := ModelConfig{
		BaseURL:     strings.TrimSpace(r.FormValue("base_url")),
		APIKey:      strings.TrimSpace(r.FormValue("api_key")),
		Model:       strings.TrimSpace(r.FormValue("model")),
		VisionModel: strings.TrimSpace(r.FormValue("vision_model")),
		Temperature: parseFloat(r.FormValue("temperature")),
		TimeoutSec:  parseInt(r.FormValue("timeout_sec")),
	}
	if cfg.BaseURL == "" && cfg.APIKey == "" && cfg.Model == "" {
		saved, err := loadConfig()
		if err != nil {
			return cfg, fmt.Errorf("load model config: %w", err)
		}
		cfg = saved
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 60
	}
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return cfg, errors.New("model config requires base_url, api_key and model")
	}
	return cfg, nil
}

func (cfg ModelConfig) forVision() ModelConfig {
	cfg.VisionModel = strings.TrimSpace(cfg.VisionModel)
	if cfg.VisionModel != "" {
		cfg.Model = cfg.VisionModel
	}
	return cfg
}

type uploadedImage struct {
	mime string
	data []byte
}

func readMultipartFile(file multipart.File, header *multipart.FileHeader, limit int64) (uploadedImage, error) {
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return uploadedImage{}, err
	}
	if int64(len(data)) > limit {
		return uploadedImage{}, errors.New("image is too large")
	}
	mime := header.Header.Get("Content-Type")
	if mime == "" || mime == "application/octet-stream" {
		mime = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mime, "image/") {
		return uploadedImage{}, fmt.Errorf("unsupported content type: %s", mime)
	}
	return uploadedImage{mime: mime, data: data}, nil
}

func callVisionModel(cfg ModelConfig, mime string, img []byte, seed PartInfo) (PartInfo, string, error) {
	return callImageJSONModel(cfg, mime, img, buildVisionPrompt(seed))
}

func callMaterialModel(cfg ModelConfig, mime string, img []byte, seed PartInfo) (PartInfo, string, error) {
	content, err := callImageRawModel(cfg, mime, img, buildMaterialPrompt(seed), true)
	if err != nil {
		return PartInfo{}, "", err
	}
	part, err := parsePartJSON(content)
	if err == nil {
		return part, content, nil
	}
	if fallback := parseMaterialText(content); hasMaterialFields(fallback) {
		log.Printf("material model returned text, used fallback parser json_err=%v snippet=%q", err, compactSnippet(content, 600))
		return fallback, content, nil
	}
	snippet := compactSnippet(content, 600)
	log.Printf("material model returned unparseable content snippet=%q", snippet)
	return PartInfo{}, content, fmt.Errorf("model did not return valid material JSON: %w; response starts with: %s", err, snippet)
}

func callImageJSONModel(cfg ModelConfig, mime string, img []byte, prompt string) (PartInfo, string, error) {
	content, err := callImageRawModel(cfg, mime, img, prompt, true)
	if err != nil {
		return PartInfo{}, "", err
	}
	part, err := parsePartJSON(content)
	if err != nil {
		snippet := compactSnippet(content, 600)
		log.Printf("model returned non-json content snippet=%q", snippet)
		return PartInfo{}, content, fmt.Errorf("model did not return valid JSON: %w; response starts with: %s", err, snippet)
	}
	return part, content, nil
}

func callImageTextModel(cfg ModelConfig, mime string, img []byte, prompt string) (string, error) {
	return callImageRawModel(cfg, mime, img, prompt, false)
}

func callImageRawModel(cfg ModelConfig, mime string, img []byte, prompt string, jsonMode bool) (string, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return "", errors.New("model config requires base_url, api_key and model")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	url := base
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img)
	reqBody := map[string]any{
		"model":       cfg.Model,
		"temperature": cfg.Temperature,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
				},
			},
		},
	}
	if jsonMode {
		reqBody["response_format"] = map[string]string{"type": "json_object"}
	}
	body, _ := json.Marshal(reqBody)
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	start := time.Now()
	log.Printf("model image request start url=%s model=%s bytes=%d timeout=%ds", url, cfg.Model, len(img), cfg.TimeoutSec)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("model image request transport error model=%s elapsed=%s err=%v", cfg.Model, time.Since(start), err)
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("model image response status=%d model=%s elapsed=%s bytes=%d", resp.StatusCode, cfg.Model, time.Since(start), len(respBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("model API returned %s: %s", resp.Status, string(respBody))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("model response has no choices")
	}
	return out.Choices[0].Message.Content, nil
}

func callTextModel(cfg ModelConfig, prompt string) (string, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return "", errors.New("model config requires base_url, api_key and model")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	url := base
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}
	reqBody := map[string]any{
		"model":       cfg.Model,
		"temperature": cfg.Temperature,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	body, _ := json.Marshal(reqBody)
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	start := time.Now()
	log.Printf("model text request start url=%s model=%s timeout=%ds", url, cfg.Model, cfg.TimeoutSec)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("model text request transport error model=%s elapsed=%s err=%v", cfg.Model, time.Since(start), err)
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("model text response status=%d model=%s elapsed=%s bytes=%d", resp.StatusCode, cfg.Model, time.Since(start), len(respBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("model API returned %s: %s", resp.Status, string(respBody))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("model response has no choices")
	}
	return out.Choices[0].Message.Content, nil
}

func fetchModelList(cfg ModelConfig) ([]string, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, errors.New("base_url and api_key are required to fetch models")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	url := base
	if strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimSuffix(url, "/chat/completions")
	}
	if !strings.HasSuffix(url, "/models") {
		url += "/models"
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	start := time.Now()
	log.Printf("model list request start url=%s timeout=%ds", url, cfg.TimeoutSec)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("model list request transport error elapsed=%s err=%v", time.Since(start), err)
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("model list response status=%d elapsed=%s bytes=%d", resp.StatusCode, time.Since(start), len(respBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model list API returned %s: %s", resp.Status, string(respBody))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("parse model list response: %w", err)
	}
	seen := map[string]bool{}
	models := make([]string, 0, len(out.Data))
	for _, item := range out.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	sort.Strings(models)
	if len(models) == 0 {
		return nil, errors.New("model list response has no model ids")
	}
	return models, nil
}

func buildVisionPrompt(seed PartInfo) string {
	return fmt.Sprintf(`Read the chip pinout image and return a raw JSON object only. The first character of your response must be "{". Do not include markdown, explanation, or prose.
Use this schema:
{
  "part_name": string,
  "mpn": string,
  "package_name": string,
  "pins": [{"number": number, "name": string, "side": "left|right|top|bottom|hidden"}],
  "exposed_pad": {"number": number, "name": string}
}
Rules:
- Preserve physical pin numbers exactly.
- If there is an exposed thermal pad, add it as exposed_pad. Use name EP_GND if it is ground/thermal pad.
- Normalize obvious active-low slashes later is allowed, but still keep names readable.
- If uncertain, still return JSON with your best pin table and do not explain uncertainty.
- Known user fields: part_name=%q mpn=%q package=%q pcb_decal=%q lcsc=%q.`,
		seed.PartName, seed.MPN, seed.PackageName, seed.PCBDecalName, seed.LCSCNumber)
}

func buildMaterialPrompt(seed PartInfo) string {
	return fmt.Sprintf(`Read the component material/order information image and return a raw JSON object only. The first character of your response must be "{". Do not include markdown, explanation, or prose.
Use this schema:
{
  "part_name": string,
  "mpn": string,
  "lcsc_number": string,
  "package_name": string,
  "pcb_decal_name": string,
  "description": string,
  "value": string,
  "pins": []
}
Field mapping hints for Chinese screenshots:
- 商品型号 / 型号 / Part Number / MPN -> mpn
- 商品编号 / LCSC / 立创编号 -> lcsc_number
- 商品封装 / 封装 / Package -> package_name
- part_name and value should usually be the generic IC name without suffix, for example TPS7A8300 from TPS7A8300RGRR.
- pcb_decal_name may be inferred from package_name using a PADS-safe name if visible.
Known user fields: part_name=%q mpn=%q package=%q pcb_decal=%q lcsc=%q.`,
		seed.PartName, seed.MPN, seed.PackageName, seed.PCBDecalName, seed.LCSCNumber)
}

func parsePartJSON(content string) (PartInfo, error) {
	j := extractJSONObject(content)
	var raw struct {
		PartName     string `json:"part_name"`
		MPN          string `json:"mpn"`
		LCSCNumber   string `json:"lcsc_number"`
		PackageName  string `json:"package_name"`
		PCBDecalName string `json:"pcb_decal_name"`
		Description  string `json:"description"`
		Value        string `json:"value"`
		Pins         []Pin  `json:"pins"`
		ExposedPad   *Pin   `json:"exposed_pad"`
	}
	if err := json.Unmarshal([]byte(j), &raw); err != nil {
		return PartInfo{}, err
	}
	part := PartInfo{
		PartName:     raw.PartName,
		MPN:          raw.MPN,
		LCSCNumber:   raw.LCSCNumber,
		PackageName:  raw.PackageName,
		PCBDecalName: raw.PCBDecalName,
		Description:  raw.Description,
		Value:        firstNonEmpty(raw.Value, raw.PartName),
		Pins:         raw.Pins,
	}
	if raw.ExposedPad != nil && raw.ExposedPad.Number > 0 {
		p := *raw.ExposedPad
		p.Hidden = true
		if p.Side == "" {
			p.Side = "hidden"
		}
		part.Pins = append(part.Pins, p)
	}
	return part, nil
}

func parseMaterialText(content string) PartInfo {
	lines := strings.Split(content, "\n")
	values := map[string]string{}
	for _, line := range lines {
		line = strings.TrimSpace(strings.Trim(line, "|"))
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "-*0123456789. ")
		var key, value string
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			key, value = parts[0], parts[1]
		} else if parts := strings.SplitN(line, "：", 2); len(parts) == 2 {
			key, value = parts[0], parts[1]
		} else {
			continue
		}
		key = normalizeMaterialKey(key)
		value = cleanMaterialValue(value)
		if key != "" && value != "" {
			values[key] = value
		}
	}
	part := PartInfo{
		PartName:     firstNonEmpty(values["part_name"], genericPartName(values["mpn"])),
		MPN:          values["mpn"],
		LCSCNumber:   values["lcsc_number"],
		PackageName:  values["package_name"],
		PCBDecalName: values["pcb_decal_name"],
		Description:  values["description"],
		Value:        values["value"],
	}
	if part.Value == "" {
		part.Value = part.PartName
	}
	return part
}

func normalizeMaterialKey(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	k = strings.ReplaceAll(k, " ", "")
	k = strings.ReplaceAll(k, "_", "")
	k = strings.ReplaceAll(k, "-", "")
	switch {
	case strings.Contains(k, "partname") || strings.Contains(k, "generic") || strings.Contains(k, "品名"):
		return "part_name"
	case strings.Contains(k, "mpn") || strings.Contains(k, "partnumber") || strings.Contains(k, "型号") || strings.Contains(k, "商品型号"):
		return "mpn"
	case strings.Contains(k, "lcsc") || strings.Contains(k, "商品编号") || strings.Contains(k, "立创"):
		return "lcsc_number"
	case strings.Contains(k, "package") || strings.Contains(k, "封装"):
		return "package_name"
	case strings.Contains(k, "pcbdecal") || strings.Contains(k, "decal"):
		return "pcb_decal_name"
	case strings.Contains(k, "description") || strings.Contains(k, "desc") || strings.Contains(k, "描述"):
		return "description"
	case k == "value" || strings.Contains(k, "值"):
		return "value"
	default:
		return ""
	}
}

func cleanMaterialValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`*| ")
	value = strings.Trim(value, `"'`)
	return value
}

func genericPartName(mpn string) string {
	mpn = strings.TrimSpace(mpn)
	if mpn == "" {
		return ""
	}
	re := regexp.MustCompile(`^([A-Za-z]+[0-9]+[A-Za-z0-9]*?)($|[A-Z]{2,}[#/].*|[A-Z]{2,}$)`)
	if m := re.FindStringSubmatch(mpn); len(m) > 1 {
		return m[1]
	}
	return mpn
}

func hasMaterialFields(part PartInfo) bool {
	return part.PartName != "" || part.MPN != "" || part.LCSCNumber != "" || part.PackageName != "" || part.PCBDecalName != "" || part.Description != "" || part.Value != ""
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
				lines = lines[1:]
			}
			if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			s = strings.Join(lines, "\n")
		} else {
			s = strings.TrimPrefix(s, "```json")
			s = strings.TrimPrefix(s, "```")
			s = strings.TrimSuffix(s, "```")
		}
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func compactSnippet(s string, limit int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > limit {
		return s[:limit] + "..."
	}
	return s
}

func loadConfig() (ModelConfig, error) {
	var cfg ModelConfig
	data, err := os.ReadFile(configFile)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func mergeSeed(part *PartInfo, seed PartInfo) {
	if part.PartName == "" {
		part.PartName = seed.PartName
	}
	if part.MPN == "" {
		part.MPN = seed.MPN
	}
	if part.LCSCNumber == "" {
		part.LCSCNumber = seed.LCSCNumber
	}
	if part.PackageName == "" {
		part.PackageName = seed.PackageName
	}
	if part.PCBDecalName == "" {
		part.PCBDecalName = seed.PCBDecalName
	}
	if part.Description == "" {
		part.Description = seed.Description
	}
	if part.Value == "" {
		if seed.Value != "" {
			part.Value = seed.Value
		} else {
			part.Value = part.PartName
		}
	}
}

func normalizePart(part *PartInfo) {
	part.PartName = safeIdent(firstNonEmpty(part.PartName, part.Value, part.MPN))
	part.Value = safeIdent(firstNonEmpty(part.Value, part.PartName))
	if part.PCBDecalName == "" && part.PackageName != "" {
		part.PCBDecalName = safeDecalName(part.PackageName)
	}
	if part.Description == "" {
		part.Description = strings.TrimSpace(strings.Join([]string{part.MPN, part.PackageName}, " "))
	}
	for i := range part.Pins {
		part.Pins[i].Name = normalizePinName(part.Pins[i].Name)
		part.Pins[i].Side = normalizeSide(part.Pins[i].Side)
		if part.Pins[i].Side == "hidden" {
			part.Pins[i].Hidden = true
		}
	}
	sort.SliceStable(part.Pins, func(i, j int) bool {
		return part.Pins[i].Number < part.Pins[j].Number
	})
}

func validatePart(part PartInfo) error {
	if part.PartName == "" {
		return errors.New("part_name is required")
	}
	if part.PCBDecalName == "" {
		return errors.New("pcb_decal_name is required")
	}
	if len(part.Pins) == 0 {
		return errors.New("pins are required")
	}
	seen := map[int]bool{}
	for _, p := range part.Pins {
		if p.Number <= 0 {
			return fmt.Errorf("invalid pin number %d", p.Number)
		}
		if p.Name == "" {
			return fmt.Errorf("pin %d has empty name", p.Number)
		}
		if seen[p.Number] {
			return fmt.Errorf("duplicate pin number %d", p.Number)
		}
		seen[p.Number] = true
	}
	return nil
}

func generateLogicDecal(part PartInfo) string {
	pins := drawablePins(part.Pins)
	pinCount := len(part.Pins)
	left, right, top := splitPinsForSymbol(pins)
	rows := max(len(left), len(right))
	if rows < 1 {
		rows = 1
	}
	height := (rows + 1) * 200
	width := 1400
	refX := width/2 + 10
	refY := height + 308
	var b strings.Builder
	fmt.Fprintf(&b, "*PADS-LIBRARY-SCH-DECALS-V9*\n\n")
	fmt.Fprintf(&b, "%-16s 32000 32000 97 10 97 10 4 1 0 %d 0\n", part.PartName, pinCount)
	fmt.Fprintf(&b, "TIMESTAMP %s\n", padsTimestamp())
	fmt.Fprintf(&b, "\"Default Font\"\n\"Default Font\"\n")
	fmt.Fprintf(&b, "%-5d %-5d 0 4 97 10 \"Default Font\"\nREF-DES\n", refX, refY)
	fmt.Fprintf(&b, "%-5d -40   0 6 97 10 \"Default Font\"\nPART-TYPE\n", width/2)
	fmt.Fprintf(&b, "%-5d -140  0 6 97 10 \"Default Font\"\n*\n", width/2)
	fmt.Fprintf(&b, "%-5d -240  0 6 97 10 \"Default Font\"\n*\n", width/2)
	fmt.Fprintf(&b, "CLOSED 5 30 0 -1\n")
	fmt.Fprintf(&b, "0     0\n%d  0\n%d  %d\n0     %d\n0     0\n", width, width, height, height)

	for i := range left {
		y := height - 200 - i*200
		writePinGraphic(&b, -200, y, 0)
	}
	for i := range right {
		y := 200 + i*200
		writePinGraphic(&b, width+200, y, 2)
	}
	for i := range top {
		x := 300 + i*200
		writePinGraphic(&b, x, height+200, 4)
	}
	hidden := pinCount - len(pins)
	for i := 0; i < hidden; i++ {
		x := width/2 + i*80
		writePinGraphic(&b, x, height+400, 4)
	}
	fmt.Fprintf(&b, "\n*END*\n")
	return b.String()
}

func drawablePins(pins []Pin) []Pin {
	var out []Pin
	for _, p := range pins {
		if !p.Hidden && p.Side != "hidden" {
			out = append(out, p)
		}
	}
	return out
}

func splitPinsForSymbol(pins []Pin) (left, right, top []Pin) {
	for _, p := range pins {
		switch p.Side {
		case "left":
			left = append(left, p)
		case "right":
			right = append(right, p)
		case "top", "bottom":
			top = append(top, p)
		default:
			if len(left) <= len(right) {
				left = append(left, p)
			} else {
				right = append(right, p)
			}
		}
	}
	if len(left) == 0 && len(right) == 0 && len(top) == 0 {
		for i, p := range pins {
			if i < (len(pins)+1)/2 {
				left = append(left, p)
			} else {
				right = append(right, p)
			}
		}
	}
	return
}

func writePinGraphic(b *strings.Builder, x, y, orient int) {
	if orient == 0 {
		fmt.Fprintf(b, "T%-5d %-5d 0 0 140   20    0 2 230   0     0 16 PIN\n", x, y)
	} else if orient == 2 {
		fmt.Fprintf(b, "T%-5d %-5d 0 2 140   20    0 2 230   0     0 16 PIN\n", x, y)
	} else {
		fmt.Fprintf(b, "T%-5d %-5d 90 4 140   20    0 2 230   0     0 16 PIN\n", x, y)
	}
	fmt.Fprintf(b, "P-520  0     0 2 -80   0     0 2 0\n")
}

func generatePartType(part PartInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*PADS-LIBRARY-PART-TYPES-V9*\n\n")
	fmt.Fprintf(&b, "%s %s I TTL 4 1 0 0 0\n", part.PartName, part.PCBDecalName)
	fmt.Fprintf(&b, "TIMESTAMP %s\n", padsTimestamp())
	fmt.Fprintf(&b, "\"Description\" %s\n", part.Description)
	fmt.Fprintf(&b, "\"Geometry.Height\" \n")
	fmt.Fprintf(&b, "\"Part Number\" %s\n", part.LCSCNumber)
	fmt.Fprintf(&b, "\"Value\" %s\n", part.Value)
	fmt.Fprintf(&b, "GATE 1 %d 0\n", len(part.Pins))
	fmt.Fprintf(&b, "%s\n", part.PartName)
	for _, p := range part.Pins {
		fmt.Fprintf(&b, "%d 0 L %s\n", p.Number, p.Name)
	}
	fmt.Fprintf(&b, "\n*END*\n")
	return b.String()
}

func normalizePinName(name string) string {
	original := strings.TrimSpace(name)
	activeLow := strings.HasPrefix(original, "/")
	name = strings.TrimPrefix(original, "/")
	name = strings.ReplaceAll(name, "−", "-")
	name = strings.ReplaceAll(name, "–", "-")
	name = strings.ReplaceAll(name, "+", "P")
	name = strings.ReplaceAll(name, "-", "N")
	name = strings.ReplaceAll(name, ".", "V")
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "(", "_")
	name = strings.ReplaceAll(name, ")", "_")
	name = strings.ReplaceAll(name, "__", "_")
	name = strings.Trim(name, "_")
	name = strings.ToUpper(name)
	if activeLow && !strings.HasSuffix(name, "_N") {
		name += "_N"
	}
	if name == "THERMAL_PAD" || name == "PAD" || name == "EP" {
		name = "EP_GND"
	}
	re := regexp.MustCompile(`[^A-Z0-9_]+`)
	name = re.ReplaceAllString(name, "_")
	return strings.Trim(name, "_")
}

func safeIdent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)
	re := regexp.MustCompile(`[^A-Z0-9_]+`)
	s = re.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s != "" && s[0] >= '0' && s[0] <= '9' {
		s = "U_" + s
	}
	return s
}

func safeDecalName(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	re := regexp.MustCompile(`[^A-Z0-9_.-]+`)
	s = re.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func normalizeSide(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "left", "right", "top", "bottom", "hidden":
		return s
	default:
		return ""
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func padsTimestamp() string {
	return time.Now().Format("2006.01.02.15.04.05")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func parseFloat(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

var _ = atoi
