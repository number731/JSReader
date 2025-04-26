package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorPurple  = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
	colorOrange  = "\033[38;5;208m"
	colorTeal    = "\033[38;5;6m" 
	colorPink    = "\033[38;5;13m" 
	colorMagenta = "\033[35m"  
)

var (
	threads    int
	inputFile  string
	jsFile     string
	pipeMode   bool
	outputFile string
)

type Result struct {
	Type    string
	URL     string
	Source  string
	Details string
}

type SafePrinter struct {
	mu       sync.Mutex
	fileMu   sync.Mutex
	outputFH *os.File
}

func (p *SafePrinter) PrintResult(result Result) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var color, prefix string
	switch result.Type {
	case "S3 Bucket":
		color = colorRed
		prefix = "S3 Bucket"
	case "Firebase URL":
		color = colorYellow
		prefix = "Firebase DB"
	case "Firebase Storage":
		color = colorYellow
		prefix = "Firebase Storage"
	case "Firebase API":
		color = colorYellow
		prefix = "Firebase API"
	case "API Endpoint":
		color = colorGreen
		prefix = "API"
	case "GraphQL":
		color = colorCyan
		prefix = "GraphQL"
	case "Auth Endpoint":
		color = colorPurple
		prefix = "Auth"
	case "URL in variable":
		color = colorBlue
		prefix = "JS Variable"
	case "Telegram Token":
		color = colorOrange
		prefix = "Telegram Bot"
	case "API Subdomain":
		color = colorTeal
		prefix = "API Subdomain"
	case "API Version":
		color = colorPink
		prefix = "API Version"
	case "API Component":
		color = colorMagenta
		prefix = "API Component"
	default:
		color = colorWhite
		prefix = result.Type
	}

	fmt.Printf("%s[%s]%s %s%s%s\n",
		color, prefix, colorReset,
		colorWhite, result.URL, colorReset)

	if result.Details != "" {
		fmt.Printf("   %sDetails:%s %s\n",
			colorWhite, colorReset, result.Details)
	}

	if result.Source != "" {
		fmt.Printf("   %sSource:%s %s\n\n",
			colorWhite, colorReset, result.Source)
	} else {
		fmt.Println()
	}

	if p.outputFH != nil {
		p.fileMu.Lock()
		defer p.fileMu.Unlock()

		entry := fmt.Sprintf("[%s] %s\n", prefix, result.URL)
		if result.Details != "" {
			entry += fmt.Sprintf("Details: %s\n", result.Details)
		}
		if result.Source != "" {
			entry += fmt.Sprintf("Source: %s\n", result.Source)
		}
		entry += "\n"

		_, err := p.outputFH.WriteString(entry)
		if err != nil {
			fmt.Printf("%s[ERROR]%s Failed to write to output file: %v\n", colorRed, colorReset, err)
		}
	}
}

func (p *SafePrinter) PrintStatus(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Printf("%s[STATUS]%s %s\n", colorBlue, colorReset, msg)
}

func (p *SafePrinter) PrintError(source, msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Printf("%s[ERROR]%s %s - %s\n", colorRed, colorReset, source, msg)
}

func (p *SafePrinter) CloseOutput() {
	if p.outputFH != nil {
		p.outputFH.Close()
	}
}

var printer = &SafePrinter{}

func main() {
	flag.IntVar(&threads, "t", 1, "Number of threads to use")
	flag.StringVar(&inputFile, "i", "", "Path to file with list of JS URLs (one per line)")
	flag.StringVar(&jsFile, "f", "", "Path to single JS file to analyze")
	flag.BoolVar(&pipeMode, "p", false, "Enable pipe mode (read from stdin)")
	flag.StringVar(&outputFile, "o", "", "Output file to save results (.txt)")
	flag.Parse()

	if outputFile != "" {
		fh, err := os.Create(outputFile)
		if err != nil {
			printer.PrintError("Output", fmt.Sprintf("Failed to create output file: %v", err))
			os.Exit(1)
		}
		printer.outputFH = fh
		defer printer.CloseOutput()

		_, err = fh.WriteString("=== JS Parser Results ===\n\n")
		if err != nil {
			printer.PrintError("Output", fmt.Sprintf("Failed to write to output file: %v", err))
		}
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		pipeMode = true
	}

	var jsFiles []string

	switch {
	case pipeMode:
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			jsFiles = append(jsFiles, strings.TrimSpace(scanner.Text()))
		}
		if err := scanner.Err(); err != nil {
			printer.PrintError("stdin", fmt.Sprintf("Error reading from stdin: %v", err))
			os.Exit(1)
		}
	case jsFile != "":
		jsFiles = append(jsFiles, jsFile)
	case inputFile != "":
		file, err := os.Open(inputFile)
		if err != nil {
			printer.PrintError(inputFile, fmt.Sprintf("Error opening input file: %v", err))
			os.Exit(1)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			jsFiles = append(jsFiles, strings.TrimSpace(scanner.Text()))
		}

		if err := scanner.Err(); err != nil {
			printer.PrintError(inputFile, fmt.Sprintf("Error reading input file: %v", err))
			os.Exit(1)
		}
	default:
		printer.PrintError("Args", "You must specify input source (-f, -i, or pipe)")
		flag.Usage()
		os.Exit(1)
	}

	if len(jsFiles) == 0 {
		printer.PrintError("Input", "No JS files to analyze")
		os.Exit(1)
	}

	if !pipeMode {
		printer.PrintStatus(fmt.Sprintf("Found %d JS files to analyze", len(jsFiles)))
		printer.PrintStatus(fmt.Sprintf("Using %d threads", threads))
		if outputFile != "" {
			printer.PrintStatus(fmt.Sprintf("Saving results to: %s", outputFile))
		}
	}

	workChan := make(chan string, len(jsFiles))
	resultsChan := make(chan Result, 100)
	var wg sync.WaitGroup

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range workChan {
				analyzeJSFile(file, resultsChan)
			}
		}()
	}

	go func() {
		for result := range resultsChan {
			printer.PrintResult(result)
		}
	}()

	for _, file := range jsFiles {
		workChan <- file
	}
	close(workChan)

	wg.Wait()
	close(resultsChan)
}

func analyzeJSFile(jsURL string, resultsChan chan<- Result) {
	var content []byte
	var err error

	if strings.HasPrefix(jsURL, "http") {
		if !pipeMode {
			printer.PrintStatus(fmt.Sprintf("Fetching remote file: %s", jsURL))
		}
		content, err = fetchRemoteJS(jsURL)
	} else {
		if !pipeMode {
			printer.PrintStatus(fmt.Sprintf("Reading local file: %s", jsURL))
		}
		content, err = os.ReadFile(jsURL)
	}

	if err != nil {
		printer.PrintError(jsURL, err.Error())
		return
	}

	if !pipeMode {
		printer.PrintStatus(fmt.Sprintf("Analyzing %s (%d bytes)", jsURL, len(content)))
	}

	jsContent := string(content)

	patterns := map[string]*regexp.Regexp{
		"S3 Bucket":        regexp.MustCompile(`https?://[a-zA-Z0-9.-]*\.?s3[.-][a-z0-9-]*\.amazonaws\.com[^\s"']*`),
		"Firebase URL":     regexp.MustCompile(`https?://[a-zA-Z0-9-]+\.firebaseio\.com[^\s"']*`),
		"Firebase Storage": regexp.MustCompile(`https?://firebasestorage\.googleapis\.com[^\s"']*`),
		"Firebase API":     regexp.MustCompile(`https?://[a-zA-Z0-9-]+\.firebaseapp\.com[^\s"']*`),
		"API Endpoint":     regexp.MustCompile(`https?://[a-zA-Z0-9.-]+/(v[0-9]+/|api/)[a-zA-Z0-9./_-]*`),
		"GraphQL":          regexp.MustCompile(`https?://[a-zA-Z0-9.-]+/(graphql|gql)[^\s"']*`),
		"Auth Endpoint":    regexp.MustCompile(`https?://[a-zA-Z0-9.-]+/(auth|oauth|token|login|register|user|admin)[^\s"']*`),
		"Telegram Token":   regexp.MustCompile(`[0-9]{8,10}:[a-zA-Z0-9_-]{35}`),
		"API Version":      regexp.MustCompile(`["'](v[0-9]+(\.[0-9]+)?)["']`),
		"API Subdomain":    regexp.MustCompile(`https?://(api|api-[a-zA-Z0-9]+)\.([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}[^\s"']*`),
		"API Component":    regexp.MustCompile(`["'](/(api|rest|v[0-9]+)/[a-zA-Z0-9/_-]+)["']`),
	}

	excludePatterns := []*regexp.Regexp{
		regexp.MustCompile(`w3\.org/`),
		regexp.MustCompile(`schema\.org/`),
		regexp.MustCompile(`\.min\.js`),
		regexp.MustCompile(`://localhost`),
		regexp.MustCompile(`://127\.0\.0\.1`),
	}

	found := make(map[string]bool)
	var mu sync.Mutex

	reportFinding := func(name, match, details string) {
		for _, excludeRe := range excludePatterns {
			if excludeRe.MatchString(strings.ToLower(match)) {
				return
			}
		}

		mu.Lock()
		defer mu.Unlock()
		if !found[match] {
			resultsChan <- Result{
				Type:    name,
				URL:     match,
				Source:  jsURL,
				Details: details,
			}
			found[match] = true
		}
	}

	for name, re := range patterns {
		matches := re.FindAllString(jsContent, -1)
		for _, match := range matches {
			details := ""
			switch name {
			case "S3 Bucket":
				details = "Potential public S3 bucket - check permissions"
			case "Firebase URL", "Firebase Storage", "Firebase API":
				details = "Firebase service - check security rules"
			case "Auth Endpoint":
				details = "Authentication endpoint - check for vulnerabilities"
			case "API Endpoint":
				details = "API endpoint - investigate available methods"
			case "Telegram Token":
				details = "Telegram Bot API token - check if it's exposed"
			case "API Version":
				details = "API version identifier - may indicate available API versions"
			case "API Subdomain":
				details = "Dedicated API subdomain - investigate available endpoints"
			case "API Component":
				details = "API path component - may indicate service structure"
			}
			reportFinding(name, match, details)
		}
	}

	apiVersionRe := regexp.MustCompile(`https?://[^/]+/v([0-9]+(\.[0-9]+)?)/`)
	apiVersionMatches := apiVersionRe.FindAllStringSubmatch(jsContent, -1)
	for _, match := range apiVersionMatches {
		if len(match) > 1 {
			versionStr := "v" + match[1]
			details := "API version from URL path - investigate other available versions"
			reportFinding("API Version", versionStr, details)
		}
	}

	telegramContextRe := regexp.MustCompile(`(bot|token|api|key)[\s]*[=:][\s]*["']([0-9]{8,10}:[a-zA-Z0-9_-]{35})["']`)
	telegramContextMatches := telegramContextRe.FindAllStringSubmatch(jsContent, -1)
	for _, match := range telegramContextMatches {
		if len(match) > 2 {
			details := "Telegram Bot API token in a variable context - high confidence match"
			reportFinding("Telegram Token", match[2], details)
		}
	}

	variablePatterns := []*regexp.Regexp{
		regexp.MustCompile(`const\s+[a-zA-Z0-9_]+\s*=\s*["'](https?://[^"'\s]+)["']`),
		regexp.MustCompile(`let\s+[a-zA-Z0-9_]+\s*=\s*["'](https?://[^"'\s]+)["']`),
		regexp.MustCompile(`var\s+[a-zA-Z0-9_]+\s*=\s*["'](https?://[^"'\s]+)["']`),
		regexp.MustCompile(`[a-zA-Z0-9_]+\s*:\s*["'](https?://[^"'\s]+)["']`),
		regexp.MustCompile(`(url|endpoint|api|baseUrl|apiUrl|baseURL|apiURL)\s*[=:]\s*["'](https?://[^"'\s]+)["']`),
		regexp.MustCompile(`(url|endpoint|api|baseUrl|apiUrl|baseURL|apiURL)\s*[=:]\s*["'](\/[^"'\s]+)["']`),
	}

	for _, re := range variablePatterns {
		matches := re.FindAllStringSubmatch(jsContent, -1)
		for _, match := range matches {
			if len(match) > 1 {
				reportFinding("URL in variable", match[1], "Found in JavaScript variable - may contain sensitive API URLs")
			}
		}
	}

	apiObjectPatterns := []*regexp.Regexp{
		regexp.MustCompile(`endpoints\s*:\s*\{\s*[^}]*["']([^"']+)["']\s*:\s*["']([^"']+)["']`),
		regexp.MustCompile(`api\s*:\s*\{\s*[^}]*["']([^"']+)["']\s*:\s*["']([^"']+)["']`),
		regexp.MustCompile(`routes\s*:\s*\{\s*[^}]*["']([^"']+)["']\s*:\s*["']([^"']+)["']`),
	}

	for _, re := range apiObjectPatterns {
		matches := re.FindAllStringSubmatch(jsContent, -1)
		for _, match := range matches {
			if len(match) > 2 {
				endpoint := match[2]
				if strings.HasPrefix(endpoint, "http") {
					reportFinding("API Component", endpoint, "API endpoint found in object definition - "+match[1])
				} else if strings.HasPrefix(endpoint, "/") {
					reportFinding("API Component", endpoint, "API path found in object definition - "+match[1])
				}
			}
		}
	}

	apiVersionCommentRe := regexp.MustCompile(`\/\/.*\b(v[0-9]+(\.[0-9]+)?)\b.*api`)
	apiVersionCommentMatches := apiVersionCommentRe.FindAllStringSubmatch(jsContent, -1)
	for _, match := range apiVersionCommentMatches {
		if len(match) > 1 {
			reportFinding("API Version", match[1], "API version mentioned in code comment")
		}
	}

	apiCallPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(fetch|axios\.get|axios\.post|ajax|request)\s*\(\s*["'](https?://[^"'\s]+)["']`),
		regexp.MustCompile(`\.(get|post|put|delete|patch)\s*\(\s*["'](https?://[^"'\s]+)["']`),
		regexp.MustCompile(`\.(get|post|put|delete|patch)\s*\(\s*["'](\/[^"'\s]+)["']`),
	}

	for _, re := range apiCallPatterns {
		matches := re.FindAllStringSubmatch(jsContent, -1)
		for _, match := range matches {
			if len(match) > 2 {
				endpoint := match[2]
				if strings.Contains(endpoint, "/api/") ||
					strings.Contains(endpoint, "/v1/") ||
					strings.Contains(endpoint, "/v2/") ||
					regexp.MustCompile(`/v[0-9]+/`).MatchString(endpoint) {
					reportFinding("API Endpoint", endpoint, "API endpoint found in "+match[1]+" call")
				} else if strings.HasPrefix(endpoint, "http") {
					reportFinding("URL in variable", endpoint, "URL found in "+match[1]+" call")
				}
			}
		}
	}
}

func fetchRemoteJS(jsURL string) ([]byte, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("GET", jsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching JS file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading JS file: %w", err)
	}

	return content, nil
}
