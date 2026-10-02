package services

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ResolveResult contém a URL resolvida e os cookies de sessão obtidos durante a resolução.
type ResolveResult struct {
	URL     string
	Cookies string // Cookies de sessão obtidos (ex: do WorkUpload PoW)
}

// resolverRule define uma regra de resolução para um domínio específico.
type resolverRule struct {
	// hostPattern faz match no hostname da URL (inclui subdomínios).
	hostPattern *regexp.Regexp
	// resolve tenta extrair o URL de download direto e cookies de sessão.
	resolve func(r *LinkResolver, originalURL string, cookies string, userAgent string) ResolveResult
}

var resolverRules = []resolverRule{
	{
		// WorkUpload: qualquer subdomínio *.workupload.com ou workupload.com
		hostPattern: regexp.MustCompile(`(?i)(.*\.)?workupload\.com`),
		resolve:     resolveWorkUpload,
	},
	{
		// 1Fichier: 1fichier.com/?FILEID
		hostPattern: regexp.MustCompile(`(?i)(www\.)?1fichier\.com`),
		resolve:     resolve1Fichier,
	},
	{
		// Mediafire: mediafire.com/file/...
		hostPattern: regexp.MustCompile(`(?i)(www\.)?mediafire\.com`),
		resolve:     resolveMediafire,
	},
	{
		// PixelDrain: pixeldrain.com/u/...
		hostPattern: regexp.MustCompile(`(?i)(www\.)?pixeldrain\.com`),
		resolve:     resolvePixelDrain,
	},
}

// puzzleResponse é a resposta da API /puzzle do WorkUpload.
type puzzleResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Puzzle string   `json:"puzzle"`
		Range  int      `json:"range"`
		Find   []string `json:"find"`
	} `json:"data"`
}

// downloadServerResponse é a resposta da API /api/file/getDownloadServer/ do WorkUpload.
type downloadServerResponse struct {
	Success bool `json:"success"`
	Data    struct {
		URL string `json:"url"`
	} `json:"data"`
}

// resolveWorkUpload implementa o fluxo completo de anti-bot do WorkUpload:
//
// 1. GET /puzzle               → dados do PoW SHA-256
// 2. Resolver SHA-256 PoW     → encontrar i onde sha256(puzzle+i) ∈ find[]
// 3. POST /captcha            → submeter soluções, receber cookie de sessão
// 4. GET /api/file/getDownloadServer/FILE_ID → URL real do CDN (ex: f95.workupload.com/download/ID)
// 5. Retornar URL do CDN + cookies de sessão para uso no download
func resolveWorkUpload(r *LinkResolver, originalURL string, cookies string, userAgent string) ResolveResult {
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		log.Printf("[RESOLVER][WorkUpload] URL inválida: %v", err)
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}

	baseHost := "https://workupload.com"

	// Extrair o file ID de qualquer formato de URL do WorkUpload:
	//   workupload.com/file/ID
	//   workupload.com/start/ID
	//   f58.workupload.com/download/ID
	fileIDPattern := regexp.MustCompile(`(?i)/(?:file|start|f|download)/([a-zA-Z0-9]+)`)
	matches := fileIDPattern.FindStringSubmatch(parsedURL.Path)
	if len(matches) < 2 {
		log.Printf("[RESOLVER][WorkUpload] Não foi possível extrair file ID de %s", originalURL)
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}
	fileID := matches[1]
	pageURL := fmt.Sprintf("%s/file/%s", baseHost, fileID)

	log.Printf("[RESOLVER][WorkUpload] FileID=%s", fileID)

	// Cookie jar compartilhado para toda a sessão
	jar := &cookieJar{cookies: make(map[string][]*http.Cookie)}
	transportClone := GetSharedTransport().Clone()
	transportClone.DisableKeepAlives = true
	sessionClient := GetSharedHTTPClient()
	sessionClient.Jar = jar
	sessionClient.Transport = transportClone
	sessionClient.CheckRedirect = SafeCheckRedirect(10)

	ua := userAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	}

	// Pré-carregar cookies do usuário no jar
	if cookies != "" {
		jar.setRaw("workupload.com", cookies)
		jar.setRaw(".workupload.com", cookies)
	}

	// Passo 1: GET /puzzle → dados do PoW
	puzzleEndpoint := fmt.Sprintf("%s/puzzle", baseHost)
	log.Printf("[RESOLVER][WorkUpload] Passo 1: buscando puzzle em %s", puzzleEndpoint)
	req1, _ := http.NewRequest("GET", puzzleEndpoint, nil)
	setWorkUploadHeaders(req1, ua, pageURL)
	req1.Header.Set("X-Requested-With", "XMLHttpRequest")
	req1.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")

	resp1, err := sessionClient.Do(req1)
	if err != nil {
		log.Printf("[RESOLVER][WorkUpload] Erro buscando puzzle: %v", err)
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		log.Printf("[RESOLVER][WorkUpload] Puzzle retornou status %d", resp1.StatusCode)
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}

	var puzzle puzzleResponse
	if err := json.NewDecoder(resp1.Body).Decode(&puzzle); err != nil {
		log.Printf("[RESOLVER][WorkUpload] Erro decodificando puzzle: %v", err)
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}
	if !puzzle.Success || len(puzzle.Data.Find) == 0 {
		log.Printf("[RESOLVER][WorkUpload] Puzzle inválido: success=%v", puzzle.Success)
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}

	log.Printf("[RESOLVER][WorkUpload] Passo 2: resolvendo PoW SHA-256 (range=%d, targets=%d)...",
		puzzle.Data.Range, len(puzzle.Data.Find))

	// Passo 2: Resolver o PoW SHA-256
	// sha256(puzzle + i) deve estar em find[]
	findSet := make(map[string]bool, len(puzzle.Data.Find))
	for _, h := range puzzle.Data.Find {
		findSet[strings.ToLower(h)] = true
	}

	var solutions []string
	for i := 0; i < puzzle.Data.Range; i++ {
		input := fmt.Sprintf("%s%d", puzzle.Data.Puzzle, i)
		hash := sha256.Sum256([]byte(input))
		hashHex := fmt.Sprintf("%x", hash)
		if findSet[hashHex] {
			solutions = append(solutions, fmt.Sprintf("%d", i))
			log.Printf("[RESOLVER][WorkUpload] Solução: i=%d", i)
			if len(solutions) == len(puzzle.Data.Find) {
				break
			}
		}
	}

	if len(solutions) == 0 {
		log.Printf("[RESOLVER][WorkUpload] Não foi possível resolver o puzzle PoW")
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}

	captchaValue := strings.Join(solutions, " ") + " "
	log.Printf("[RESOLVER][WorkUpload] Passo 3: submetendo captcha %q", captchaValue)

	// Passo 3: POST /captcha → cookie de sessão verificado
	captchaEndpoint := fmt.Sprintf("%s/captcha", baseHost)
	formData := url.Values{"captcha": {captchaValue}}
	req2, _ := http.NewRequest("POST", captchaEndpoint, strings.NewReader(formData.Encode()))
	setWorkUploadHeaders(req2, ua, pageURL)
	req2.Header.Set("X-Requested-With", "XMLHttpRequest")
	req2.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req2.Close = true // Evita EOF por reuso de conexão fechada pelo servidor

	resp2, err := sessionClient.Do(req2)
	if err != nil {
		log.Printf("[RESOLVER][WorkUpload] Captcha POST err (pode ser EOF normal): %v", err)
	} else {
		io.Copy(io.Discard, resp2.Body)
		resp2.Body.Close()
		log.Printf("[RESOLVER][WorkUpload] Captcha submetido: status=%d", resp2.StatusCode)
	}

	// Passo 4: GET /api/file/getDownloadServer/FILE_ID → URL real do CDN
	apiURL := fmt.Sprintf("%s/api/file/getDownloadServer/%s", baseHost, fileID)
	log.Printf("[RESOLVER][WorkUpload] Passo 4: buscando URL do CDN em %s", apiURL)
	req3, _ := http.NewRequest("GET", apiURL, nil)
	setWorkUploadHeaders(req3, ua, fmt.Sprintf("%s/start/%s", baseHost, fileID))
	req3.Header.Set("X-Requested-With", "XMLHttpRequest")
	req3.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req3.Close = true // Evita EOF por reuso de conexão fechada pelo servidor

	resp3, err := sessionClient.Do(req3)
	if err != nil {
		log.Printf("[RESOLVER][WorkUpload] Erro buscando download server: %v", err)
		sessionCookies := jar.getString("workupload.com")
		return ResolveResult{URL: originalURL, Cookies: sessionCookies}
	}
	defer resp3.Body.Close()

	var dlResp downloadServerResponse
	
	// Ler o corpo primeiro para podermos debugar se falhar
	respBytes, readErr := io.ReadAll(resp3.Body)
	if readErr != nil {
		log.Printf("[RESOLVER][WorkUpload] Erro lendo download server resp: %v", readErr)
	}

	if err := json.Unmarshal(respBytes, &dlResp); err != nil {
		log.Printf("[RESOLVER][WorkUpload] Erro decodificando download server resp: %v. Body: %s", err, string(respBytes))
		sessionCookies := jar.getString("workupload.com")
		return ResolveResult{URL: originalURL, Cookies: sessionCookies}
	}

	if !dlResp.Success || dlResp.Data.URL == "" {
		log.Printf("[RESOLVER][WorkUpload] Download server retornou sucesso=false ou URL vazia")
		sessionCookies := jar.getString("workupload.com")
		return ResolveResult{URL: originalURL, Cookies: sessionCookies}
	}

	cdnURL := dlResp.Data.URL
	sessionCookies := jar.getString("workupload.com")

	log.Printf("[RESOLVER][WorkUpload] ✓ URL CDN resolvida: %s (cookies: %s...)", cdnURL,
		func() string {
			if len(sessionCookies) > 30 {
				return sessionCookies[:30]
			}
			return sessionCookies
		}())

	return ResolveResult{URL: cdnURL, Cookies: sessionCookies}
}
func resolve1Fichier(_ *LinkResolver, originalURL string, cookies string, userAgent string) ResolveResult {
	return ResolveResult{URL: originalURL, Cookies: cookies}
}

// resolveMediafire extrai o link de download direto do Mediafire.
func resolveMediafire(r *LinkResolver, originalURL string, cookies string, userAgent string) ResolveResult {
	if err := isInternalURL(originalURL); err != nil {
		log.Printf("[RESOLVER][Mediafire] Security error: %v", err)
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}
	req, err := http.NewRequest("GET", originalURL, nil)
	if err != nil {
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}
	r.setHeaders(req, cookies, userAgent, originalURL)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	body := string(bodyBytes)

	dlPattern := regexp.MustCompile(`(?i)(?:id="downloadButton"|aria-label="Download file")[^>]*href="([^"]+)"`)
	m := dlPattern.FindStringSubmatch(body)
	if len(m) >= 2 {
		log.Printf("[RESOLVER][Mediafire] Resolved %s → %s", originalURL, m[1])
		return ResolveResult{URL: m[1], Cookies: cookies}
	}

	directPattern := regexp.MustCompile(`https://download\d+\.mediafire\.com/[^"'\s]+`)
	match := directPattern.FindString(body)
	if match != "" {
		log.Printf("[RESOLVER][Mediafire] Resolved (direct) %s → %s", originalURL, match)
		return ResolveResult{URL: match, Cookies: cookies}
	}

	log.Printf("[RESOLVER][Mediafire] Não foi possível extrair link de %s", originalURL)
	return ResolveResult{URL: originalURL, Cookies: cookies}
}

// resolvePixelDrain converte URL da página para URL de API direta.
func resolvePixelDrain(_ *LinkResolver, originalURL string, cookies string, userAgent string) ResolveResult {
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return ResolveResult{URL: originalURL, Cookies: cookies}
	}

	pathPattern := regexp.MustCompile(`(?i)/u/([a-zA-Z0-9]+)`)
	m := pathPattern.FindStringSubmatch(parsedURL.Path)
	if len(m) >= 2 {
		fileID := m[1]
		directURL := fmt.Sprintf("%s://%s/api/file/%s?download", parsedURL.Scheme, parsedURL.Host, fileID)
		log.Printf("[RESOLVER][PixelDrain] Resolved %s → %s", originalURL, directURL)
		return ResolveResult{URL: directURL, Cookies: cookies}
	}

	log.Printf("[RESOLVER][PixelDrain] Não foi possível extrair file ID de %s", originalURL)
	return ResolveResult{URL: originalURL, Cookies: cookies}
}

// setWorkUploadHeaders configura os headers padrão para requests do WorkUpload.
func setWorkUploadHeaders(req *http.Request, ua, referer string) {
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// req.Header.Set("Accept-Encoding", "gzip, deflate, br") // Comentado para o Go descomprimir GZIP automaticamente
	req.Header.Set("Connection", "keep-alive")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func propagateHeaders(req *http.Request, via []*http.Request) {
	if len(via) > 0 {
		for key, vals := range via[0].Header {
			if key == "Cookie" || key == "User-Agent" || key == "Referer" {
				req.Header[key] = vals
			}
		}
	}
}

// cookieJar é um jar de cookies simples para gerenciar sessões HTTP.
type cookieJar struct {
	mu      sync.RWMutex
	cookies map[string][]*http.Cookie
}

func (j *cookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()

	host := j.normalizeHost(u.Hostname())
	
	// Para evitar cookies duplicados obsoletos, vamos sobrescrever cookies com o mesmo nome
	existing := j.cookies[host]
	var updated []*http.Cookie
	
	for _, oldCookie := range existing {
		overwritten := false
		for _, newCookie := range cookies {
			if oldCookie.Name == newCookie.Name {
				overwritten = true
				break
			}
		}
		if !overwritten {
			updated = append(updated, oldCookie)
		}
	}
	
	updated = append(updated, cookies...)
	j.cookies[host] = updated
}

func (j *cookieJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.RLock()
	defer j.mu.RUnlock()

	host := j.normalizeHost(u.Hostname())
	var result []*http.Cookie
	result = append(result, j.cookies[host]...)
	result = append(result, j.cookies["."+host]...)
	parts := strings.SplitN(host, ".", 2)
	if len(parts) == 2 {
		result = append(result, j.cookies[parts[1]]...)
		result = append(result, j.cookies["."+parts[1]]...)
	}
	return result
}

func (j *cookieJar) setRaw(host, rawCookies string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	host = j.normalizeHost(host)
	for _, part := range strings.Split(rawCookies, ";") {
		part = strings.TrimSpace(part)
		if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
			j.cookies[host] = append(j.cookies[host], &http.Cookie{
				Name:  strings.TrimSpace(kv[0]),
				Value: strings.TrimSpace(kv[1]),
			})
		}
	}
}

func (j *cookieJar) getString(host string) string {
	host = j.normalizeHost(host)
	var parts []string
	seen := make(map[string]bool)
	for _, c := range j.Cookies(&url.URL{Host: host}) {
		if !seen[c.Name] {
			parts = append(parts, c.Name+"="+c.Value)
			seen[c.Name] = true
		}
	}
	return strings.Join(parts, "; ")
}

func (j *cookieJar) normalizeHost(host string) string {
	return strings.ToLower(strings.TrimPrefix(host, "www."))
}

// LinkResolver resolve URLs de páginas de file hosting para links de download direto.
type LinkResolver struct {
	httpClient *http.Client
}

// NewLinkResolver cria um novo LinkResolver com timeout configurado.
func NewLinkResolver() *LinkResolver {
	return &LinkResolver{
		httpClient: func() *http.Client {
			c := GetSharedHTTPClient()
			c.CheckRedirect = SafeCheckRedirect(15)
			return c
		}(),
	}
}

// isHTMLResponse verifica se a resposta HTTP é uma página HTML.
func isHTMLResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
}

// isDirectDownload verifica se a resposta é um download direto (não HTML).
func isDirectDownload(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	cd := resp.Header.Get("Content-Disposition")

	if strings.Contains(cd, "attachment") {
		return true
	}

	htmlTypes := []string{"text/html", "application/xhtml", "text/plain", "text/xml", "application/xml"}
	for _, htmlType := range htmlTypes {
		if strings.Contains(ct, htmlType) {
			return false
		}
	}

	if strings.Contains(ct, "application/octet-stream") ||
		strings.Contains(ct, "application/zip") ||
		strings.Contains(ct, "video/") ||
		strings.Contains(ct, "audio/") ||
		strings.Contains(ct, "image/") {
		return true
	}

	return resp.ContentLength > 1024*1024
}

// ResolveWithSession tenta resolver uma URL para o link de download direto,
// retornando também os cookies de sessão obtidos durante o processo.
func (r *LinkResolver) ResolveWithSession(rawURL string, cookies string, userAgent string) ResolveResult {
	log.Printf("[RESOLVER] Tentando resolver URL: %s", rawURL)

	if cached, ok := getCachedResolveResult(rawURL); ok {
		log.Printf("[RESOLVER] Usando cache para URL: %s", rawURL)
		return cached
	}

	parsedURL, err := ValidateURL(rawURL)
	if err != nil {
		log.Printf("[RESOLVER] URL rejeitada: %v", err)
		return ResolveResult{URL: rawURL, Cookies: cookies}
	}

	// Verificar se existe uma regra para este domínio
	var matchedRule *resolverRule
	for i := range resolverRules {
		if resolverRules[i].hostPattern.MatchString(parsedURL.Hostname()) {
			matchedRule = &resolverRules[i]
			break
		}
	}

	var res ResolveResult
	if matchedRule == nil {
		log.Printf("[RESOLVER] Nenhuma regra específica para %s. Verificando diretamente.", parsedURL.Hostname())
		finalURL, err := r.probeURL(rawURL, cookies, userAgent)
		if err != nil {
			log.Printf("[RESOLVER] Probe falhou: %v. Usando URL original.", err)
			return ResolveResult{URL: rawURL, Cookies: cookies}
		}
		res = ResolveResult{URL: finalURL, Cookies: cookies}
	} else {
		res = matchedRule.resolve(r, rawURL, cookies, userAgent)
	}

	cacheResolveResult(rawURL, res)
	return res
}

// Resolve é mantido por compatibilidade — usa ResolveWithSession internamente.
func (r *LinkResolver) Resolve(rawURL string, cookies string, userAgent string) (string, error) {
	result := r.ResolveWithSession(rawURL, cookies, userAgent)
	return result.URL, nil
}

// probeURL faz um HEAD request para verificar se a URL é um download direto.
func (r *LinkResolver) probeURL(rawURL string, cookies string, userAgent string) (string, error) {
	if err := isInternalURL(rawURL); err != nil {
		return "", fmt.Errorf("security error: %w", err)
	}
	req, err := http.NewRequest("HEAD", rawURL, nil)
	if err != nil {
		return "", err
	}
	r.setHeaders(req, cookies, userAgent, rawURL)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	log.Printf("[RESOLVER] URL probe: %s → status=%d, content-type=%s, final=%s",
		rawURL, resp.StatusCode, resp.Header.Get("Content-Type"), finalURL)

	return finalURL, nil
}

// setHeaders configura headers padrão para as requisições genéricas do resolver.
func (r *LinkResolver) setHeaders(req *http.Request, cookies, userAgent, referer string) {
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	} else {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	}
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	// req.Header.Set("Accept-Encoding", "gzip, deflate, br") // Comentado para o Go descomprimir GZIP automaticamente
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

type cacheEntry struct {
	result ResolveResult
	exp    time.Time
}

var resolveCache sync.Map

// getCachedResolveResult checks the cache for a cached resolution.
func getCachedResolveResult(url string) (ResolveResult, bool) {
	if val, ok := resolveCache.Load(url); ok {
		entry := val.(cacheEntry)
		if time.Now().Before(entry.exp) {
			return entry.result, true
		}
		resolveCache.Delete(url)
	}
	return ResolveResult{}, false
}

func cacheResolveResult(url string, result ResolveResult) {
	resolveCache.Store(url, cacheEntry{
		result: result,
		exp:    time.Now().Add(10 * time.Minute),
	})
}
